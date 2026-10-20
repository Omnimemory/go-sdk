package memaura

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultV4Endpoint = "https://api.omnimemory.cn/api/v3"

// V4Turn is one message submitted to the V4 Card/Episode pipeline.
type V4Turn struct {
	Content    string   `json:"content"`
	Role       Role     `json:"role,omitempty"`
	TurnID     string   `json:"turn_id,omitempty"`
	Timestamp  string   `json:"timestamp,omitempty"`
	SenderName string   `json:"sender_name,omitempty"`
	ReferList  []string `json:"refer_list,omitempty"`
}

// V4IngestInput describes a V4 ingestion request.
type V4IngestInput struct {
	Turns              []V4Turn
	Language           string
	Timezone           string
	DeviceNo           string
	WaitFor            string
	WaitTimeoutSeconds float64
}

// V4Job is the public state of a V4 ingestion task.
type V4Job struct {
	JobID        string            `json:"job_id"`
	Status       string            `json:"status,omitempty"`
	WaitTimedOut bool              `json:"wait_timed_out,omitempty"`
	Error        map[string]string `json:"error,omitempty"`
}

// V4Episode is a compact source summary returned with a Card.
type V4Episode struct {
	EpisodeID string `json:"episode_id"`
	Summary   string `json:"summary"`
}

// V4Card is the compact public Card returned by default V4 retrieval.
type V4Card struct {
	CardID   string      `json:"card_id"`
	Role     string      `json:"role"`
	Content  string      `json:"content"`
	Episodes []V4Episode `json:"episodes"`
}

// V4RetrievalInput describes weighted V4 Card retrieval.
type V4RetrievalInput struct {
	Query                string
	TopK                 int
	VectorWeight         *float64
	TagKeys              []string
	Role                 string
	ScopeType            string
	StartTime            string
	EndTime              string
	EntityID             string
	EntityMatchMode      string
	ParticipantRoles     []string
	DeviceNo             string
	IncludeQueryAnalysis bool
	IncludeDetails       bool
}

// V4RetrievalResult contains normalized Cards and optional full response data.
type V4RetrievalResult struct {
	Cards         []V4Card
	QueryAnalysis map[string]any
	Details       map[string]any
}

// MemoryV4 is an additive V4 client; the legacy Memory client remains unchanged.
type MemoryV4 struct {
	apiKey     string
	endpoint   string
	deviceNo   string
	httpClient *http.Client
}

// NewMemoryV4 creates a client for /api/v3 memory V4 routes.
func NewMemoryV4(config Config) (*MemoryV4, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, &ValidationError{Field: "api key", Message: "must not be empty"}
	}
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = defaultV4Endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, &ValidationError{Field: "endpoint", Message: "must be an absolute HTTP URL"}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &MemoryV4{
		apiKey: config.APIKey, endpoint: parsed.String(), deviceNo: config.DeviceNo, httpClient: client,
	}, nil
}

// Ingest submits the turns exactly once. Transport failures are never retried automatically.
func (m *MemoryV4) Ingest(ctx context.Context, input V4IngestInput) (V4Job, error) {
	if len(input.Turns) == 0 {
		return V4Job{}, &ValidationError{Field: "turns", Message: "must contain at least one message"}
	}
	for index, turn := range input.Turns {
		if strings.TrimSpace(turn.Content) == "" {
			return V4Job{}, &ValidationError{Field: fmt.Sprintf("turns[%d].content", index), Message: "must not be empty"}
		}
	}
	language := input.Language
	if language == "" {
		language = "auto"
	}
	if language != "auto" && language != "zh" && language != "en" {
		return V4Job{}, &ValidationError{Field: "language", Message: "must be auto, zh, or en"}
	}
	waitFor := input.WaitFor
	if waitFor == "" {
		waitFor = "staged"
	}
	if waitFor != "staged" && waitFor != "searchable" {
		return V4Job{}, &ValidationError{Field: "wait for", Message: "must be staged or searchable"}
	}
	waitTimeout := input.WaitTimeoutSeconds
	if waitTimeout == 0 {
		waitTimeout = 120
	}
	if waitTimeout <= 0 || waitTimeout > 300 {
		return V4Job{}, &ValidationError{Field: "wait timeout seconds", Message: "must be between 0 and 300"}
	}
	body := map[string]any{
		"turns": input.Turns, "language": language, "wait_for": waitFor,
		"wait_timeout_seconds": waitTimeout,
	}
	if input.Timezone != "" {
		body["timezone"] = input.Timezone
	}
	var job V4Job
	if err := m.request(ctx, http.MethodPost, "/memory/ingest_4", body, input.DeviceNo, &job); err != nil {
		return V4Job{}, fmt.Errorf("V4 ingest: %w", err)
	}
	if job.JobID == "" {
		return V4Job{}, fmt.Errorf("invalid V4 job response: missing job_id")
	}
	return job, nil
}

// Retrieve performs weighted V4 retrieval and parses compact Card/Episode results.
func (m *MemoryV4) Retrieve(ctx context.Context, input V4RetrievalInput) (V4RetrievalResult, error) {
	if strings.TrimSpace(input.Query) == "" {
		return V4RetrievalResult{}, &ValidationError{Field: "query", Message: "must not be empty"}
	}
	if input.TopK < 0 || input.TopK > 100 {
		return V4RetrievalResult{}, &ValidationError{Field: "top k", Message: "must be between 1 and 100 when set"}
	}
	weight := 0.8
	if input.VectorWeight != nil {
		weight = *input.VectorWeight
	}
	if weight < 0 || weight > 1 {
		return V4RetrievalResult{}, &ValidationError{Field: "vector weight", Message: "must be between 0 and 1"}
	}
	body := map[string]any{
		"query": input.Query, "vector_weight": weight, "tag_keys": nonNilStrings(input.TagKeys),
		"include_query_analysis": input.IncludeQueryAnalysis, "include_details": input.IncludeDetails,
	}
	if input.TopK > 0 {
		body["top_k"] = input.TopK
	}
	if input.Role != "" {
		body["role"] = input.Role
	}
	if input.ScopeType != "" {
		if input.ScopeType != "account" && input.ScopeType != "api_key" && input.ScopeType != "device" {
			return V4RetrievalResult{}, &ValidationError{Field: "scope type", Message: "must be account, api_key, or device"}
		}
		body["scope_type"] = input.ScopeType
	}
	if input.StartTime != "" {
		body["start_time"] = input.StartTime
	}
	if input.EndTime != "" {
		body["end_time"] = input.EndTime
	}
	if input.EntityID != "" {
		body["entity_id"] = input.EntityID
	}
	if input.EntityMatchMode != "" {
		if input.EntityMatchMode != "primary" && input.EntityMatchMode != "involved" && input.EntityMatchMode != "source" {
			return V4RetrievalResult{}, &ValidationError{Field: "entity match mode", Message: "must be primary, involved, or source"}
		}
		body["entity_match_mode"] = input.EntityMatchMode
	}
	if input.ParticipantRoles != nil {
		body["participant_roles"] = input.ParticipantRoles
	}
	var payload map[string]any
	if err := m.request(ctx, http.MethodPost, "/memory/retrieval_4", body, input.DeviceNo, &payload); err != nil {
		return V4RetrievalResult{}, fmt.Errorf("V4 retrieval: %w", err)
	}
	rawResults, ok := payload["results"].([]any)
	if !ok {
		return V4RetrievalResult{}, fmt.Errorf("invalid V4 retrieval response: missing results")
	}
	cards := make([]V4Card, 0, len(rawResults))
	for _, raw := range rawResults {
		value, ok := raw.(map[string]any)
		if !ok {
			return V4RetrievalResult{}, fmt.Errorf("invalid V4 retrieval response: Card must be an object")
		}
		card := V4Card{
			CardID: stringValue(value["card_id"]), Role: stringValue(value["role"]),
			Content: stringValue(value["content"]), Episodes: []V4Episode{},
		}
		if rawEpisodes, exists := value["episodes"]; exists && rawEpisodes != nil {
			episodes, ok := rawEpisodes.([]any)
			if !ok {
				return V4RetrievalResult{}, fmt.Errorf("invalid V4 retrieval response: episodes must be an array")
			}
			for _, rawEpisode := range episodes {
				episode, _ := rawEpisode.(map[string]any)
				card.Episodes = append(card.Episodes, V4Episode{
					EpisodeID: stringValue(episode["episode_id"]), Summary: stringValue(episode["summary"]),
				})
			}
		}
		cards = append(cards, card)
	}
	result := V4RetrievalResult{Cards: cards}
	if analysis, ok := payload["query_analysis"].(map[string]any); ok {
		result.QueryAnalysis = analysis
	}
	if input.IncludeDetails {
		result.Details = payload
	}
	return result, nil
}

// Jobs returns V4 ingest-job operations.
func (m *MemoryV4) Jobs() *V4JobOperations {
	return &V4JobOperations{memory: m}
}

// V4JobOperations reads and waits for V4 ingestion jobs.
type V4JobOperations struct {
	memory *MemoryV4
}

// Get reads the authoritative public V4 job state.
func (jobs *V4JobOperations) Get(ctx context.Context, jobID string) (V4Job, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return V4Job{}, &ValidationError{Field: "job id", Message: "must not be empty"}
	}
	var job V4Job
	path := "/memory/ingest_4/jobs/" + url.PathEscape(jobID)
	if err := jobs.memory.request(ctx, http.MethodGet, path, nil, "", &job); err != nil {
		return V4Job{}, fmt.Errorf("get V4 ingest job: %w", err)
	}
	if job.JobID == "" {
		job.JobID = jobID
	}
	return job, nil
}

// Wait polls only the existing job. It never resubmits the ingest request.
func (jobs *V4JobOperations) Wait(ctx context.Context, jobID string, options ...WaitOption) (V4Job, error) {
	config := waitConfig{pollInterval: time.Second, timeout: 5 * time.Minute}
	for _, option := range options {
		if option != nil {
			option.applyWait(&config)
		}
	}
	waitContext, cancel := boundedContext(ctx, config.timeout)
	defer cancel()
	var last *V4Job
	for {
		job, err := jobs.Get(waitContext, jobID)
		if err != nil {
			if waitContext.Err() != nil {
				return V4Job{}, &V4TimeoutError{JobID: jobID, LastJob: last}
			}
			return V4Job{}, err
		}
		last = &job
		switch strings.ToLower(job.Status) {
		case "succeeded", "failed":
			return job, nil
		case "queued", "staged", "processing", "buffered", "indexing":
		default:
			return V4Job{}, fmt.Errorf("unexpected V4 job status %q", job.Status)
		}
		timer := time.NewTimer(config.pollInterval)
		select {
		case <-waitContext.Done():
			timer.Stop()
			return V4Job{}, &V4TimeoutError{JobID: jobID, LastJob: last}
		case <-timer.C:
		}
	}
}

// V4TimeoutError means the local wait ended; callers should keep the job ID.
type V4TimeoutError struct {
	JobID   string
	LastJob *V4Job
}

func (e *V4TimeoutError) Error() string {
	return fmt.Sprintf("timed out waiting for V4 ingest job %s; continue polling the same job", e.JobID)
}

func (e *V4TimeoutError) Unwrap() error { return context.DeadlineExceeded }

func (m *MemoryV4) request(ctx context.Context, method, path string, body any, deviceNo string, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, m.endpoint+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("X-API-Key", m.apiKey)
	request.Header.Set("Content-Type", "application/json")
	resolvedDevice := resolveDeviceNo(deviceNo, m.deviceNo)
	if resolvedDevice != "" {
		request.Header.Set("X-Device-No", resolvedDevice)
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || payload["success"] == false || payload["error"] != nil {
		return v4APIError(response.StatusCode, payload, raw)
	}
	data := raw
	if payload["success"] == true {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields) // raw was validated as a JSON object above.
		data = fields["data"]
		if len(data) == 0 || bytes.Equal(data, []byte("null")) {
			return &APIError{StatusCode: response.StatusCode, Code: response.StatusCode, Message: "Router returned an empty response"}
		}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode response data: %w", err)
	}
	return nil
}

func v4APIError(status int, payload map[string]any, raw []byte) error {
	message := fmt.Sprintf("Router request failed with status %d", status)
	errorCode := ""
	if value, ok := payload["message"].(string); ok && value != "" {
		message = value
	}
	if compact, ok := payload["error"].(map[string]any); ok {
		if value, ok := compact["message"].(string); ok && value != "" {
			message = value
		}
		if value, ok := compact["code"].(string); ok {
			errorCode = value
		}
	}
	if data, ok := payload["data"].(map[string]any); ok && errorCode == "" {
		if value, ok := data["error"].(string); ok {
			errorCode = value
		}
	}
	return &APIError{StatusCode: status, Code: status, ErrorCode: errorCode, Message: message, Detail: append([]byte(nil), raw...)}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}
