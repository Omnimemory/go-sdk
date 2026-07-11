package memaura

import (
	"fmt"
	"net/http"

	ogen "github.com/Omnimemory/go-sdk/internal/generated/ogen"
)

func mapAddResponse(response ogen.MemoryIngestMemoryIngestPostRes) (JobAck, error) {
	switch value := response.(type) {
	case *ogen.EnvelopeIngestJobAck:
		if err := successEnvelopeError(http.StatusAccepted, value.Success, value.Message, value.Code); err != nil {
			return JobAck{}, err
		}
		data, ok := value.Data.Get()
		if !ok {
			return JobAck{}, emptyDataError(http.StatusAccepted, value.Code, optionalString(value.Message))
		}
		status := optionalString(data.Status)
		if status == "" {
			status = "queued"
		}
		return JobAck{
			JobID:     data.JobID,
			SessionID: optionalNilString(data.SessionID),
			Status:    status,
			StatusURL: data.StatusURL,
		}, nil
	case *ogen.MemoryIngestMemoryIngestPostBadRequest:
		return JobAck{}, errorEnvelope(http.StatusBadRequest, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryIngestMemoryIngestPostUnauthorized:
		return JobAck{}, errorEnvelope(http.StatusUnauthorized, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryIngestMemoryIngestPostNotFound:
		return JobAck{}, errorEnvelope(http.StatusNotFound, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryIngestMemoryIngestPostUnprocessableEntity:
		return JobAck{}, errorEnvelope(http.StatusUnprocessableEntity, ogen.ErrorEnvelope(*value))
	default:
		return JobAck{}, fmt.Errorf("unexpected ingest response type %T", response)
	}
}

func mapSearchResponse(response ogen.MemoryRetrievalMemoryRetrievalPostRes, fallbackQuery string) (SearchResult, error) {
	switch value := response.(type) {
	case *ogen.EnvelopeMemoryRetrievalOut:
		return mapSearchEnvelope(value, fallbackQuery)
	case *ogen.MemoryRetrievalMemoryRetrievalPostBadRequest:
		return SearchResult{}, errorEnvelope(http.StatusBadRequest, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryRetrievalMemoryRetrievalPostUnauthorized:
		return SearchResult{}, errorEnvelope(http.StatusUnauthorized, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryRetrievalMemoryRetrievalPostNotFound:
		return SearchResult{}, errorEnvelope(http.StatusNotFound, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryRetrievalMemoryRetrievalPostUnprocessableEntity:
		return SearchResult{}, errorEnvelope(http.StatusUnprocessableEntity, ogen.ErrorEnvelope(*value))
	default:
		return SearchResult{}, fmt.Errorf("unexpected retrieval response type %T", response)
	}
}

func mapHybridSearchResponse(response ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostRes, fallbackQuery string) (SearchResult, error) {
	switch value := response.(type) {
	case *ogen.EnvelopeMemoryRetrievalOut:
		return mapSearchEnvelope(value, fallbackQuery)
	case *ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostBadRequest:
		return SearchResult{}, errorEnvelope(http.StatusBadRequest, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostUnauthorized:
		return SearchResult{}, errorEnvelope(http.StatusUnauthorized, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostNotFound:
		return SearchResult{}, errorEnvelope(http.StatusNotFound, ogen.ErrorEnvelope(*value))
	case *ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostUnprocessableEntity:
		return SearchResult{}, errorEnvelope(http.StatusUnprocessableEntity, ogen.ErrorEnvelope(*value))
	default:
		return SearchResult{}, fmt.Errorf("unexpected hybrid retrieval response type %T", response)
	}
}

func mapJobStatusResponse(response ogen.IngestJobStatusMemoryIngestJobsJobIDGetRes) (JobStatus, error) {
	switch value := response.(type) {
	case *ogen.EnvelopeIngestJobStatus:
		if err := successEnvelopeError(http.StatusOK, value.Success, value.Message, value.Code); err != nil {
			return JobStatus{}, err
		}
		data, ok := value.Data.Get()
		if !ok {
			return JobStatus{}, emptyDataError(http.StatusOK, value.Code, optionalString(value.Message))
		}
		return JobStatus{
			JobID:     optionalNilString(data.JobID),
			SessionID: optionalNilString(data.SessionID),
			Status:    optionalNilString(data.Status),
		}, nil
	case *ogen.IngestJobStatusMemoryIngestJobsJobIDGetBadRequest:
		return JobStatus{}, errorEnvelope(http.StatusBadRequest, ogen.ErrorEnvelope(*value))
	case *ogen.IngestJobStatusMemoryIngestJobsJobIDGetUnauthorized:
		return JobStatus{}, errorEnvelope(http.StatusUnauthorized, ogen.ErrorEnvelope(*value))
	case *ogen.IngestJobStatusMemoryIngestJobsJobIDGetNotFound:
		return JobStatus{}, errorEnvelope(http.StatusNotFound, ogen.ErrorEnvelope(*value))
	case *ogen.IngestJobStatusMemoryIngestJobsJobIDGetUnprocessableEntity:
		return JobStatus{}, errorEnvelope(http.StatusUnprocessableEntity, ogen.ErrorEnvelope(*value))
	default:
		return JobStatus{}, fmt.Errorf("unexpected ingest job response type %T", response)
	}
}

func mapSearchEnvelope(envelope *ogen.EnvelopeMemoryRetrievalOut, fallbackQuery string) (SearchResult, error) {
	if err := successEnvelopeError(http.StatusOK, envelope.Success, envelope.Message, envelope.Code); err != nil {
		return SearchResult{}, err
	}
	data, ok := envelope.Data.Get()
	if !ok {
		return SearchResult{}, emptyDataError(http.StatusOK, envelope.Code, optionalString(envelope.Message))
	}
	query := optionalString(data.Query)
	if query == "" {
		query = fallbackQuery
	}
	result := SearchResult{
		Strategy:         optionalString(data.Strategy),
		Query:            query,
		Evidence:         append([]string(nil), data.Evidence...),
		EvidenceDetails:  make([]EvidenceDetail, 0, len(data.EvidenceDetails)),
		RequestedGroupID: optionalNilStringPointer(data.RequestedGroupID),
	}
	for _, detail := range data.EvidenceDetails {
		result.EvidenceDetails = append(result.EvidenceDetails, EvidenceDetail{
			EventID:     detail.EventID,
			Text:        detail.Text,
			Source:      optionalString(detail.Source),
			Role:        optionalNilStringPointer(detail.Role),
			SenderName:  optionalNilStringPointer(detail.SenderName),
			AtomicFacts: append([]string(nil), detail.AtomicFacts...),
			GroupID:     optionalNilStringPointer(detail.GroupID),
			Timestamp:   optionalNilStringPointer(detail.Timestamp),
		})
	}
	return result, nil
}

func successEnvelopeError(status int, success ogen.OptBool, message ogen.OptString, code int) error {
	if value, ok := success.Get(); ok && !value {
		return emptyDataError(status, code, optionalString(message))
	}
	return nil
}

func emptyDataError(status, code int, message string) *APIError {
	if message == "" {
		message = "Router returned an empty response"
	}
	if code == 0 {
		code = status
	}
	return &APIError{StatusCode: status, Code: code, Message: message}
}

func errorEnvelope(status int, envelope ogen.ErrorEnvelope) *APIError {
	errorCode := ""
	var detail []byte
	if data, ok := envelope.Data.Get(); ok {
		errorCode = optionalNilString(data.Error)
		detail = append([]byte(nil), data.Detail...)
	}
	code := envelope.Code
	if code == 0 {
		code = status
	}
	message := envelope.Message
	if message == "" {
		message = fmt.Sprintf("Router request failed with status %d", status)
	}
	return &APIError{StatusCode: status, Code: code, ErrorCode: errorCode, Message: message, Detail: detail}
}

func optionalString(value ogen.OptString) string {
	result, _ := value.Get()
	return result
}

func optionalNilString(value ogen.OptNilString) string {
	result, _ := value.Get()
	return result
}

func optionalNilStringPointer(value ogen.OptNilString) *string {
	result, ok := value.Get()
	if !ok {
		return nil
	}
	return &result
}
