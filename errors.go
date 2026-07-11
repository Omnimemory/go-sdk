package memaura

import (
	"context"
	"encoding/json"
	"fmt"
)

// ValidationError reports invalid client-side input before a request is sent.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// APIError reports a non-success MemAura API envelope.
type APIError struct {
	StatusCode int
	Code       int
	ErrorCode  string
	Message    string
	Detail     json.RawMessage
}

func (e *APIError) Error() string {
	if e.ErrorCode != "" {
		return fmt.Sprintf("memaura API error (%s): %s", e.ErrorCode, e.Message)
	}
	return fmt.Sprintf("memaura API error: %s", e.Message)
}

// TimeoutError reports that waiting for an ingest job reached its deadline.
type TimeoutError struct {
	JobID   string
	LastJob *JobStatus
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("timed out waiting for ingest job %s", e.JobID)
}

// Unwrap allows errors.Is(err, context.DeadlineExceeded).
func (e *TimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}
