package memaura

import (
	"context"
	"errors"
	"strings"
	"time"

	ogen "github.com/Omnimemory/go-sdk/internal/generated/ogen"
)

// JobOperations provides status and waiting operations for asynchronous ingestion.
type JobOperations struct {
	memory *Memory
}

// Jobs returns the job operations associated with this Memory client.
func (m *Memory) Jobs() *JobOperations {
	return &JobOperations{memory: m}
}

// Get returns the current status of an ingest job.
func (jobs *JobOperations) Get(ctx context.Context, jobID string) (JobStatus, error) {
	if strings.TrimSpace(jobID) == "" {
		return JobStatus{}, &ValidationError{Field: "job ID", Message: "must not be empty"}
	}
	response, err := jobs.memory.client.IngestJobStatusMemoryIngestJobsJobIDGet(ctx, ogen.IngestJobStatusMemoryIngestJobsJobIDGetParams{JobID: jobID})
	if err != nil {
		return JobStatus{}, err
	}
	return mapJobStatusResponse(response)
}

// WaitOption configures Jobs().Wait.
type WaitOption interface {
	applyWait(*waitConfig)
}

type waitOptionFunc func(*waitConfig)

func (option waitOptionFunc) applyWait(config *waitConfig) {
	option(config)
}

// WithPollInterval sets the delay between job status requests.
func WithPollInterval(interval time.Duration) WaitOption {
	return waitOptionFunc(func(config *waitConfig) {
		config.pollInterval = interval
	})
}

// WithWaitTimeout bounds the total time spent waiting for a job.
func WithWaitTimeout(timeout time.Duration) WaitOption {
	return waitOptionFunc(func(config *waitConfig) {
		config.timeout = timeout
	})
}

type waitConfig struct {
	pollInterval time.Duration
	timeout      time.Duration
}

// Wait polls until the job has a terminal status or the context/deadline ends.
func (jobs *JobOperations) Wait(ctx context.Context, jobID string, options ...WaitOption) (JobStatus, error) {
	if strings.TrimSpace(jobID) == "" {
		return JobStatus{}, &ValidationError{Field: "job ID", Message: "must not be empty"}
	}
	config := waitConfig{pollInterval: time.Second, timeout: time.Minute}
	for _, option := range options {
		if option != nil {
			option.applyWait(&config)
		}
	}
	if config.pollInterval <= 0 {
		return JobStatus{}, &ValidationError{Field: "poll interval", Message: "must be greater than zero"}
	}
	if config.timeout <= 0 {
		return JobStatus{}, &ValidationError{Field: "wait timeout", Message: "must be greater than zero"}
	}

	waitContext, cancel := boundedContext(ctx, config.timeout)
	defer cancel()

	var lastJob *JobStatus
	for {
		status, err := jobs.Get(waitContext, jobID)
		if err != nil {
			return JobStatus{}, waitError(jobID, lastJob, waitContext, err)
		}
		lastJob = &status
		if isTerminalStatus(status.Status) {
			return status, nil
		}

		timer := time.NewTimer(config.pollInterval)
		select {
		case <-waitContext.Done():
			timer.Stop()
			return JobStatus{}, waitError(jobID, lastJob, waitContext, waitContext.Err())
		case <-timer.C:
		}
	}
}

func boundedContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func waitError(jobID string, lastJob *JobStatus, waitContext context.Context, err error) error {
	if errors.Is(waitContext.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &TimeoutError{JobID: jobID, LastJob: lastJob}
	}
	return err
}

func isTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accumulated", "canceled", "cancelled", "completed", "done", "error", "failed", "success", "succeeded":
		return true
	default:
		return false
	}
}
