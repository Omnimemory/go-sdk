package memaura

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	ogen "github.com/Omnimemory/go-sdk/internal/generated/ogen"
)

// Memory is a configured API-key client for MemAura memory operations.
type Memory struct {
	client   *ogen.Client
	deviceNo string
}

var newGeneratedClient = ogen.NewClient

// NewMemory creates a Memory client.
func NewMemory(config Config) (*Memory, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, &ValidationError{Field: "api key", Message: "must not be empty"}
	}

	endpoint, err := normalizeEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}

	options := make([]ogen.ClientOption, 0, 1)
	if config.HTTPClient != nil {
		options = append(options, ogen.WithClient(config.HTTPClient))
	}
	client, err := newGeneratedClient(endpoint, apiKeySource{apiKey: config.APIKey}, options...)
	if err != nil {
		return nil, fmt.Errorf("create generated client: %w", err)
	}
	return &Memory{client: client, deviceNo: config.DeviceNo}, nil
}

// Add queues a memory-ingestion request. CommitID makes caller-managed retries idempotent.
func (m *Memory) Add(ctx context.Context, input AddInput) (JobAck, error) {
	if len(input.Turns) == 0 {
		return JobAck{}, &ValidationError{Field: "turns", Message: "must contain at least one message"}
	}

	turns := make([]ogen.MemoryIngestMemoryIngestPostReqTurnsItem, 0, len(input.Turns))
	for index, turn := range input.Turns {
		wireTurn, err := toWireTurn(turn, index)
		if err != nil {
			return JobAck{}, err
		}
		turns = append(turns, wireTurn)
	}

	request := &ogen.MemoryIngestMemoryIngestPostReq{Turns: turns}
	setOptionalString(&request.SessionID, input.SessionID)
	setOptionalString(&request.GroupID, input.GroupID)
	setOptionalString(&request.GroupName, input.GroupName)
	setOptionalString(&request.CommitID, input.CommitID)

	params := ogen.MemoryIngestMemoryIngestPostParams{}
	setDeviceNo(&params.XDeviceNo, resolveDeviceNo(input.DeviceNo, m.deviceNo))
	response, err := m.client.MemoryIngestMemoryIngestPost(ctx, request, params)
	if err != nil {
		return JobAck{}, fmt.Errorf("ingest memory: %w", err)
	}
	return mapAddResponse(response)
}

// Search performs regular memory retrieval.
func (m *Memory) Search(ctx context.Context, input SearchInput) (SearchResult, error) {
	return m.search(ctx, input, false)
}

// SearchHybrid performs explicit hybrid memory retrieval.
func (m *Memory) SearchHybrid(ctx context.Context, input SearchInput) (SearchResult, error) {
	return m.search(ctx, input, true)
}

func (m *Memory) search(ctx context.Context, input SearchInput, hybrid bool) (SearchResult, error) {
	if strings.TrimSpace(input.Query) == "" {
		return SearchResult{}, &ValidationError{Field: "query", Message: "must not be empty"}
	}
	if input.Limit < 0 {
		return SearchResult{}, &ValidationError{Field: "limit", Message: "must not be negative"}
	}

	deviceNo := resolveDeviceNo(input.DeviceNo, m.deviceNo)
	if hybrid {
		request := &ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostReq{Query: input.Query}
		setOptionalInt(&request.TopK, input.Limit)
		setOptionalString(&request.GroupID, input.GroupID)
		params := ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostParams{}
		setDeviceNo(&params.XDeviceNo, deviceNo)
		response, err := m.client.MemoryHybridRetrievalMemoryRetrievalHybridPost(ctx, request, params)
		if err != nil {
			return SearchResult{}, fmt.Errorf("hybrid retrieval: %w", err)
		}
		return mapHybridSearchResponse(response, input.Query)
	}

	request := &ogen.MemoryRetrievalMemoryRetrievalPostReq{Query: input.Query}
	setOptionalInt(&request.TopK, input.Limit)
	setOptionalString(&request.GroupID, input.GroupID)
	params := ogen.MemoryRetrievalMemoryRetrievalPostParams{}
	setDeviceNo(&params.XDeviceNo, deviceNo)
	response, err := m.client.MemoryRetrievalMemoryRetrievalPost(ctx, request, params)
	if err != nil {
		return SearchResult{}, fmt.Errorf("memory retrieval: %w", err)
	}
	return mapSearchResponse(response, input.Query)
}

func normalizeEndpoint(endpoint string) (string, error) {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", &ValidationError{Field: "endpoint", Message: "must be an absolute HTTP URL"}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

type apiKeySource struct {
	apiKey string
}

func (source apiKeySource) ApiKeyAuth(context.Context, ogen.OperationName) (ogen.ApiKeyAuth, error) {
	return ogen.ApiKeyAuth{APIKey: source.apiKey}, nil
}

func toWireTurn(turn Turn, index int) (ogen.MemoryIngestMemoryIngestPostReqTurnsItem, error) {
	if strings.TrimSpace(turn.Content) == "" {
		return ogen.MemoryIngestMemoryIngestPostReqTurnsItem{}, &ValidationError{Field: fmt.Sprintf("turns[%d].content", index), Message: "must not be empty"}
	}
	wireTurn := ogen.MemoryIngestMemoryIngestPostReqTurnsItem{Content: turn.Content}
	if turn.Role != "" {
		switch turn.Role {
		case RoleUser, RoleAssistant, RoleSystem:
			wireTurn.Role.SetTo(ogen.MemoryIngestMemoryIngestPostReqTurnsItemRole(turn.Role))
		default:
			return ogen.MemoryIngestMemoryIngestPostReqTurnsItem{}, &ValidationError{Field: fmt.Sprintf("turns[%d].role", index), Message: "must be user, assistant, or system"}
		}
	}
	setOptionalString(&wireTurn.TurnID, turn.TurnID)
	setOptionalString(&wireTurn.Timestamp, turn.Timestamp)
	setOptionalString(&wireTurn.SenderName, turn.SenderName)
	if turn.ReferList != nil {
		wireTurn.ReferList.SetTo(turn.ReferList)
	}
	return wireTurn, nil
}

func setOptionalString(target *ogen.OptNilString, value string) {
	if value != "" {
		target.SetTo(value)
	}
}

func setOptionalInt(target *ogen.OptNilInt, value int) {
	if value != 0 {
		target.SetTo(value)
	}
}

func setDeviceNo(target *ogen.OptString, value string) {
	if value != "" {
		target.SetTo(value)
	}
}

func resolveDeviceNo(operationValue, defaultValue string) string {
	if operationValue != "" {
		return operationValue
	}
	return defaultValue
}
