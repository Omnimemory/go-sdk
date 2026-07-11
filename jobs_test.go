package memaura_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	memaura "github.com/Omnimemory/go-sdk"
)

func TestJobsWaitStopsOnAccumulated(t *testing.T) {
	var requests atomic.Int32
	memory := newTestMemory(t, "device-default", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/memory/ingest/jobs/job-123" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "qbk_test" {
			t.Fatalf("X-API-Key = %q", got)
		}
		status := "queued"
		if requests.Add(1) == 2 {
			status = "accumulated"
		}
		writeJobStatusResponse(t, w, status)
	})

	status, err := memory.Jobs().Wait(context.Background(), "job-123",
		memaura.WithPollInterval(time.Millisecond),
		memaura.WithWaitTimeout(time.Second),
	)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if status.Status != "accumulated" || requests.Load() != 2 {
		t.Fatalf("Wait() = %#v after %d requests", status, requests.Load())
	}
}

func TestJobsWaitReturnsFailedStatus(t *testing.T) {
	memory := newTestMemory(t, "", func(w http.ResponseWriter, r *http.Request) {
		writeJobStatusResponse(t, w, "failed")
	})

	status, err := memory.Jobs().Wait(context.Background(), "job-123", memaura.WithPollInterval(time.Hour))
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if status.Status != "failed" {
		t.Fatalf("Wait() status = %#v", status)
	}
}

func TestJobsWaitRespectsContextCancellation(t *testing.T) {
	firstRequest := make(chan struct{})
	memory := newTestMemory(t, "", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-firstRequest:
		default:
			close(firstRequest)
		}
		writeJobStatusResponse(t, w, "queued")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-firstRequest
		cancel()
	}()

	_, err := memory.Jobs().Wait(ctx, "job-123", memaura.WithPollInterval(time.Hour))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context.Canceled", err)
	}
}

func TestJobsWaitReturnsTimeoutErrorWithLastJob(t *testing.T) {
	memory := newTestMemory(t, "", func(w http.ResponseWriter, r *http.Request) {
		writeJobStatusResponse(t, w, "queued")
	})

	_, err := memory.Jobs().Wait(context.Background(), "job-123",
		memaura.WithPollInterval(time.Millisecond),
		memaura.WithWaitTimeout(100*time.Millisecond),
	)
	var timeoutErr *memaura.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("Wait() error = %T %v, want *TimeoutError", err, err)
	}
	if timeoutErr.LastJob == nil || timeoutErr.LastJob.Status != "queued" {
		t.Fatalf("TimeoutError.LastJob = %#v", timeoutErr.LastJob)
	}
}

func writeJobStatusResponse(t *testing.T, w http.ResponseWriter, status string) {
	t.Helper()
	writeJSON(t, w, http.StatusOK, map[string]any{
		"success": true,
		"code":    http.StatusOK,
		"data": map[string]any{
			"job_id":     "job-123",
			"session_id": "session-1",
			"status":     status,
		},
	})
}
