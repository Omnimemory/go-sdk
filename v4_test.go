package memaura

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMemoryV4UsesExplicitRoutes(t *testing.T) {
	requests := make([]map[string]any, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		requests = append(requests, map[string]any{"path": r.URL.Path, "body": body, "device": r.Header.Get("X-Device-No")})
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/memory/ingest_4":
			_, _ = w.Write([]byte(`{"success":true,"data":{"job_id":"job-1","status":"queued"}}`))
		case "/api/v3/memory/ingest_4/jobs/job-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"succeeded"}}`))
		case "/api/v3/memory/retrieval_4":
			_, _ = w.Write([]byte(`{"results":[{"card_id":"card-1","role":"user","content":"Prefers tea","episodes":[{"episode_id":"episode-1","summary":"Said they prefer tea"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"missing"}}`))
		}
	}))
	defer server.Close()

	memory, err := NewMemoryV4(Config{APIKey: "qbk_test", DeviceNo: "device-1", Endpoint: server.URL + "/api/v3"})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := memory.Ingest(context.Background(), V4IngestInput{Turns: []V4Turn{{Role: RoleUser, Content: "I prefer tea"}}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := memory.Jobs().Get(context.Background(), ack.JobID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := memory.Retrieve(context.Background(), V4RetrievalInput{Query: "What do I prefer?", TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "succeeded" || result.Cards[0].CardID != "card-1" || result.Cards[0].Episodes[0].EpisodeID != "episode-1" {
		t.Fatalf("unexpected V4 result: job=%+v result=%+v", job, result)
	}
	if requests[0]["path"] != "/api/v3/memory/ingest_4" || requests[2]["path"] != "/api/v3/memory/retrieval_4" {
		t.Fatalf("unexpected paths: %+v", requests)
	}
	if requests[0]["device"] != "device-1" {
		t.Fatalf("missing device header: %+v", requests[0])
	}
	ingest := requests[0]["body"].(map[string]any)
	if ingest["wait_for"] != "staged" || ingest["session_id"] != nil {
		t.Fatalf("unexpected ingest body: %+v", ingest)
	}
}

func TestNewMemoryV4ValidationAndDefaults(t *testing.T) {
	for _, config := range []Config{
		{},
		{APIKey: "key", Endpoint: "relative"},
		{APIKey: "key", Endpoint: "http://[::1"},
		{APIKey: "key", Endpoint: "ftp://example.com"},
	} {
		if _, err := NewMemoryV4(config); err == nil {
			t.Fatalf("expected validation error for %+v", config)
		}
	}
	memory, err := NewMemoryV4(Config{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if memory.endpoint != defaultV4Endpoint || memory.httpClient.Timeout != 30*time.Second {
		t.Fatalf("unexpected defaults: %+v", memory)
	}
}

func TestMemoryV4InputValidation(t *testing.T) {
	memory := &MemoryV4{}
	ingestCases := []V4IngestInput{
		{},
		{Turns: []V4Turn{{Content: " "}}},
		{Turns: []V4Turn{{Content: "x"}}, Language: "fr"},
		{Turns: []V4Turn{{Content: "x"}}, WaitFor: "done"},
		{Turns: []V4Turn{{Content: "x"}}, WaitTimeoutSeconds: -1},
		{Turns: []V4Turn{{Content: "x"}}, WaitTimeoutSeconds: 301},
	}
	for _, input := range ingestCases {
		if _, err := memory.Ingest(context.Background(), input); err == nil {
			t.Fatalf("expected ingest validation error for %+v", input)
		}
	}
	negative, tooLarge := -0.1, 1.1
	retrievalCases := []V4RetrievalInput{
		{}, {Query: "x", TopK: -1}, {Query: "x", TopK: 101},
		{Query: "x", VectorWeight: &negative}, {Query: "x", VectorWeight: &tooLarge},
		{Query: "x", ScopeType: "workspace"},
		{Query: "x", EntityMatchMode: "other"},
	}
	for _, input := range retrievalCases {
		if _, err := memory.Retrieve(context.Background(), input); err == nil {
			t.Fatalf("expected retrieval validation error for %+v", input)
		}
	}
}

func TestMemoryV4OptionalFieldsAndMalformedResults(t *testing.T) {
	requests := make([]map[string]any, 0)
	responses := []string{
		`{"success":true,"data":{"job_id":"j","status":"queued"}}`,
		`{"results":[{"card_id":"c","role":"user","content":"body","episodes":[{},"bad"]}],"query_analysis":{"route":"both"}}`,
		`{"results":[{}]}`,
		`{}`,
		`{"results":["bad"]}`,
		`{"results":[{"episodes":"bad"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		_, _ = w.Write([]byte(responses[0]))
		responses = responses[1:]
	}))
	defer server.Close()
	memory, _ := NewMemoryV4(Config{APIKey: "key", Endpoint: server.URL})
	job, err := memory.Ingest(context.Background(), V4IngestInput{
		Turns: []V4Turn{{Content: "x"}}, Language: "zh", Timezone: "Asia/Shanghai",
		DeviceNo: "operation-device", WaitFor: "searchable", WaitTimeoutSeconds: 3,
	})
	if err != nil || job.JobID != "j" {
		t.Fatalf("unexpected ingest: %+v %v", job, err)
	}
	weight := 0.0
	result, err := memory.Retrieve(context.Background(), V4RetrievalInput{
		Query: "q", TopK: 1, VectorWeight: &weight, TagKeys: []string{"preference"},
		Role: "user", ScopeType: "device", DeviceNo: "operation-device",
		StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-02-01T00:00:00Z",
		EntityID: "entity-1", EntityMatchMode: "involved", ParticipantRoles: []string{"owner"},
		IncludeQueryAnalysis: true, IncludeDetails: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cards[0].Episodes) != 2 || result.Cards[0].Episodes[1].EpisodeID != "" || result.QueryAnalysis["route"] != "both" || result.Details == nil {
		t.Fatalf("unexpected detailed result: %+v", result)
	}
	compact, err := memory.Retrieve(context.Background(), V4RetrievalInput{Query: "q", TagKeys: []string{}})
	if err != nil || compact.Cards[0].Episodes == nil || compact.Details != nil {
		t.Fatalf("unexpected compact result: %+v %v", compact, err)
	}
	if requests[0]["timezone"] != "Asia/Shanghai" || requests[1]["role"] != "user" || requests[1]["scope_type"] != "device" || requests[1]["entity_match_mode"] != "involved" {
		t.Fatalf("missing optional request fields: %+v", requests)
	}
	if _, err := memory.Retrieve(context.Background(), V4RetrievalInput{Query: "q"}); err == nil || !strings.Contains(err.Error(), "missing results") {
		t.Fatalf("expected missing results error, got %v", err)
	}
	if _, err := memory.Retrieve(context.Background(), V4RetrievalInput{Query: "q"}); err == nil || !strings.Contains(err.Error(), "Card must be an object") {
		t.Fatalf("expected Card error, got %v", err)
	}
	if _, err := memory.Retrieve(context.Background(), V4RetrievalInput{Query: "q"}); err == nil || !strings.Contains(err.Error(), "episodes must be an array") {
		t.Fatalf("expected episodes error, got %v", err)
	}
}

func TestMemoryV4IngestAndRetrievalRequestErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad"}`))
	}))
	defer server.Close()
	memory, _ := NewMemoryV4(Config{APIKey: "key", Endpoint: server.URL})
	if _, err := memory.Ingest(context.Background(), V4IngestInput{Turns: []V4Turn{{Content: "x"}}}); err == nil || !strings.Contains(err.Error(), "V4 ingest") {
		t.Fatalf("expected wrapped ingest error, got %v", err)
	}
	if _, err := memory.Retrieve(context.Background(), V4RetrievalInput{Query: "q"}); err == nil || !strings.Contains(err.Error(), "V4 retrieval") {
		t.Fatalf("expected wrapped retrieval error, got %v", err)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer empty.Close()
	memory, _ = NewMemoryV4(Config{APIKey: "key", Endpoint: empty.URL})
	if _, err := memory.Ingest(context.Background(), V4IngestInput{Turns: []V4Turn{{Content: "x"}}}); err == nil || !strings.Contains(err.Error(), "missing job_id") {
		t.Fatalf("expected missing job error, got %v", err)
	}
}

func TestV4JobGetAndWaitStates(t *testing.T) {
	statuses := []string{"queued", "succeeded", "failed", "mystery"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := statuses[0]
		statuses = statuses[1:]
		_, _ = w.Write([]byte(`{"success":true,"data":{"status":"` + status + `"}}`))
	}))
	defer server.Close()
	memory, _ := NewMemoryV4(Config{APIKey: "key", Endpoint: server.URL})
	jobs := memory.Jobs()
	if _, err := jobs.Get(context.Background(), " "); err == nil {
		t.Fatal("expected empty job id error")
	}
	job, err := jobs.Wait(context.Background(), "job/a", nil, WithPollInterval(time.Millisecond), WithWaitTimeout(time.Second))
	if err != nil || job.Status != "succeeded" || job.JobID != "job/a" {
		t.Fatalf("unexpected waited job: %+v %v", job, err)
	}
	job, err = jobs.Wait(context.Background(), "j")
	if err != nil || job.Status != "failed" {
		t.Fatalf("unexpected failed job: %+v %v", job, err)
	}
	if _, err := jobs.Wait(context.Background(), "j"); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("expected unknown status, got %v", err)
	}
}

func TestV4JobWaitTimeoutAndRequestErrors(t *testing.T) {
	pending := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"status":"buffered"}}`))
	}))
	defer pending.Close()
	memory, _ := NewMemoryV4(Config{APIKey: "key", Endpoint: pending.URL})
	_, err := memory.Jobs().Wait(context.Background(), "j", WithPollInterval(time.Second), WithWaitTimeout(100*time.Millisecond))
	var timeout *V4TimeoutError
	if !errors.As(err, &timeout) || timeout.LastJob == nil || timeout.Error() == "" || !errors.Is(timeout, context.DeadlineExceeded) {
		t.Fatalf("expected timeout with last job, got %#v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = memory.Jobs().Wait(canceled, "j")
	if !errors.As(err, &timeout) || timeout.LastJob != nil {
		t.Fatalf("expected timeout without last job, got %#v", err)
	}

	broken := &MemoryV4{endpoint: "http://example.test", httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}}
	if _, err := broken.Jobs().Wait(context.Background(), "j"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestMemoryV4RequestFailureMatrix(t *testing.T) {
	memory := &MemoryV4{apiKey: "key", endpoint: "http://example.test", deviceNo: "default", httpClient: &http.Client{}}
	if err := memory.request(context.Background(), http.MethodPost, "/x", func() {}, "", &map[string]any{}); err == nil || !strings.Contains(err.Error(), "encode request") {
		t.Fatalf("expected encode error, got %v", err)
	}
	invalid := &MemoryV4{endpoint: ":"}
	if err := invalid.request(context.Background(), http.MethodGet, "/x", nil, "", &map[string]any{}); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("expected request creation error, got %v", err)
	}

	tests := []struct {
		response *http.Response
		errText  string
		target   any
	}{
		{&http.Response{StatusCode: 200, Body: failingBody{}}, "read response", &map[string]any{}},
		{response(200, `not-json`), "decode response", &map[string]any{}},
		{response(200, `{"success":true,"data":null}`), "empty response", &map[string]any{}},
		{response(200, `{"value":"text"}`), "decode response data", &struct{ Value int }{}},
	}
	for _, test := range tests {
		memory.httpClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("X-Device-No") != "operation" {
				t.Fatalf("missing operation device header")
			}
			test.response.Request = request
			return test.response, nil
		})}
		if err := memory.request(context.Background(), http.MethodGet, "/x", nil, "operation", test.target); err == nil || !strings.Contains(err.Error(), test.errText) {
			t.Fatalf("expected %q, got %v", test.errText, err)
		}
	}
}

func TestV4APIErrorMappingsAndHelpers(t *testing.T) {
	cases := []struct {
		payload map[string]any
		message string
		code    string
	}{
		{map[string]any{"message": "envelope"}, "envelope", ""},
		{map[string]any{"error": map[string]any{"message": "compact", "code": "bad"}}, "compact", "bad"},
		{map[string]any{"data": map[string]any{"error": "legacy"}}, "Router request failed", "legacy"},
		{map[string]any{"error": map[string]any{}}, "Router request failed", ""},
	}
	for _, item := range cases {
		err := v4APIError(400, item.payload, []byte(`{}`)).(*APIError)
		if !strings.Contains(err.Message, item.message) || err.ErrorCode != item.code || len(err.Detail) == 0 {
			t.Fatalf("unexpected API error: %+v", err)
		}
	}
	if got := nonNilStrings(nil); got == nil || len(got) != 0 {
		t.Fatalf("expected non-nil empty slice: %#v", got)
	}
	values := []string{"x"}
	if nonNilStrings(values)[0] != "x" || stringValue("x") != "x" || stringValue(3) != "" {
		t.Fatal("unexpected helper result")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingBody) Close() error             { return nil }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
