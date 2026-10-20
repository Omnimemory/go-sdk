package memaura

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ogen "github.com/Omnimemory/go-sdk/internal/generated/ogen"
	"github.com/go-faster/jx"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestErrorTypes(t *testing.T) {
	if got := (&ValidationError{Message: "bad"}).Error(); got != "bad" {
		t.Fatal(got)
	}
	if got := (&ValidationError{Field: "query", Message: "bad"}).Error(); got != "query: bad" {
		t.Fatal(got)
	}
	if got := (&APIError{Message: "bad"}).Error(); got != "memaura API error: bad" {
		t.Fatal(got)
	}
	if got := (&APIError{ErrorCode: "invalid", Message: "bad"}).Error(); !strings.Contains(got, "invalid") {
		t.Fatal(got)
	}
	timeout := &TimeoutError{JobID: "job-1"}
	if !strings.Contains(timeout.Error(), "job-1") || !errors.Is(timeout, context.DeadlineExceeded) {
		t.Fatal(timeout)
	}
}

func TestMemoryInternalValidationAndTransportErrors(t *testing.T) {
	if _, err := NewMemory(Config{APIKey: "key", Endpoint: "relative"}); err == nil {
		t.Fatal("relative endpoint accepted")
	}
	if _, err := NewMemory(Config{APIKey: "key"}); err != nil {
		t.Fatalf("default endpoint: %v", err)
	}

	transportError := errors.New("transport failed")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportError
	})}
	memory, err := NewMemory(Config{APIKey: "key", Endpoint: "https://example.test/", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = memory.Add(ctx, AddInput{}); err == nil {
		t.Fatal("empty turns accepted")
	}
	if _, err = memory.Add(ctx, AddInput{Turns: []Turn{{Content: " "}}}); err == nil {
		t.Fatal("blank turn accepted")
	}
	if _, err = memory.Add(ctx, AddInput{Turns: []Turn{{Content: "x", Role: Role("tool")}}}); err == nil {
		t.Fatal("invalid role accepted")
	}
	if _, err = memory.Add(ctx, AddInput{Turns: []Turn{{Content: "x", Role: RoleUser, ReferList: []string{}}}}); !errors.Is(err, transportError) {
		t.Fatalf("Add transport error = %v", err)
	}
	if _, err = memory.Search(ctx, SearchInput{Query: " "}); err == nil {
		t.Fatal("blank query accepted")
	}
	if _, err = memory.Search(ctx, SearchInput{Query: "q", Limit: -1}); err == nil {
		t.Fatal("negative limit accepted")
	}
	if _, err = memory.Search(ctx, SearchInput{Query: "q"}); !errors.Is(err, transportError) {
		t.Fatalf("Search transport error = %v", err)
	}
	if _, err = memory.SearchHybrid(ctx, SearchInput{Query: "q"}); !errors.Is(err, transportError) {
		t.Fatalf("SearchHybrid transport error = %v", err)
	}

	turn, err := toWireTurn(Turn{Content: "x"}, 0)
	if err != nil || turn.Content != "x" {
		t.Fatalf("toWireTurn = %#v, %v", turn, err)
	}
	var optionalString ogen.OptNilString
	setOptionalString(&optionalString, "")
	setOptionalString(&optionalString, "x")
	var optionalInt ogen.OptNilInt
	setOptionalInt(&optionalInt, 0)
	setOptionalInt(&optionalInt, 1)
	var device ogen.OptString
	setDeviceNo(&device, "")
	setDeviceNo(&device, "d")
	if resolveDeviceNo("operation", "default") != "operation" || resolveDeviceNo("", "default") != "default" {
		t.Fatal("resolveDeviceNo")
	}
	auth, err := (apiKeySource{apiKey: "key"}).ApiKeyAuth(ctx, "operation")
	if err != nil || auth.APIKey != "key" {
		t.Fatalf("ApiKeyAuth = %#v, %v", auth, err)
	}
}

func TestNewMemoryWrapsGeneratedClientErrors(t *testing.T) {
	original := newGeneratedClient
	t.Cleanup(func() { newGeneratedClient = original })
	sentinel := errors.New("generated client failed")
	newGeneratedClient = func(string, ogen.SecuritySource, ...ogen.ClientOption) (*ogen.Client, error) {
		return nil, sentinel
	}
	_, err := NewMemory(Config{APIKey: "key", Endpoint: "https://example.test"})
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "create generated client") {
		t.Fatalf("NewMemory() error = %v", err)
	}
}

func TestJobInternalBranches(t *testing.T) {
	memory, err := NewMemory(Config{APIKey: "key", Endpoint: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("job transport")
	})}})
	if err != nil {
		t.Fatal(err)
	}
	jobs := memory.Jobs()
	if _, err := jobs.Get(context.Background(), " "); err == nil {
		t.Fatal("blank job accepted")
	}
	if _, err := jobs.Get(context.Background(), "job"); err == nil {
		t.Fatal("transport error missing")
	}
	if _, err := jobs.Wait(context.Background(), " "); err == nil {
		t.Fatal("blank wait job accepted")
	}
	if _, err := jobs.Wait(context.Background(), "job", nil, WithPollInterval(0)); err == nil {
		t.Fatal("zero interval accepted")
	}
	if _, err := jobs.Wait(context.Background(), "job", WithWaitTimeout(0)); err == nil {
		t.Fatal("zero timeout accepted")
	}
	if _, err := jobs.Wait(context.Background(), "job", WithPollInterval(time.Millisecond)); err == nil {
		t.Fatal("wait transport error missing")
	}

	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	bounded, boundedCancel := boundedContext(parent, time.Minute)
	boundedCancel()
	if bounded != parent {
		t.Fatal("shorter parent deadline not retained")
	}
	bounded, boundedCancel = boundedContext(context.Background(), time.Millisecond)
	boundedCancel()
	if bounded == context.Background() {
		t.Fatal("timeout context not created")
	}
	sentinel := errors.New("sentinel")
	if got := waitError("job", nil, context.Background(), sentinel); !errors.Is(got, sentinel) {
		t.Fatal(got)
	}
	if _, ok := waitError("job", nil, context.Background(), context.DeadlineExceeded).(*TimeoutError); !ok {
		t.Fatal("deadline was not mapped")
	}
	for _, status := range []string{" accumulated ", "canceled", "cancelled", "completed", "done", "error", "failed", "success", "succeeded"} {
		if !isTerminalStatus(status) {
			t.Fatalf("%q not terminal", status)
		}
	}
	if isTerminalStatus("queued") {
		t.Fatal("queued is terminal")
	}
}

func TestJobsWaitCancellationWhileSleeping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"code":200,"data":{"job_id":"job","status":"queued"}}`))
		time.AfterFunc(10*time.Millisecond, cancel)
	}))
	defer server.Close()
	memory, err := NewMemory(Config{APIKey: "key", Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = memory.Jobs().Wait(ctx, "job", WithPollInterval(time.Hour))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func testErrorEnvelope() ogen.ErrorEnvelope {
	return ogen.ErrorEnvelope{
		Message: "bad request",
		Code:    400,
		Data: ogen.NewOptNilErrorData(ogen.ErrorData{
			Error:  ogen.NewOptNilString("invalid"),
			Detail: jx.Raw(`{"field":"query"}`),
		}),
	}
}

func TestResponseMappers(t *testing.T) {
	falseValue := ogen.NewOptBool(false)
	trueValue := ogen.NewOptBool(true)
	message := ogen.NewOptString("failed")

	if _, err := mapAddResponse(&ogen.EnvelopeIngestJobAck{Success: falseValue, Message: message, Code: 400}); err == nil {
		t.Fatal("failed add envelope accepted")
	}
	if _, err := mapAddResponse(&ogen.EnvelopeIngestJobAck{Success: trueValue, Code: 202}); err == nil {
		t.Fatal("empty add envelope accepted")
	}
	ack, err := mapAddResponse(&ogen.EnvelopeIngestJobAck{
		Success: trueValue,
		Code:    202,
		Data:    ogen.NewOptNilIngestJobAck(ogen.IngestJobAck{JobID: "job"}),
	})
	if err != nil || ack.Status != "queued" {
		t.Fatalf("add response = %#v, %v", ack, err)
	}

	base := testErrorEnvelope()
	addErrors := []ogen.MemoryIngestMemoryIngestPostRes{
		(*ogen.MemoryIngestMemoryIngestPostBadRequest)(&base),
		(*ogen.MemoryIngestMemoryIngestPostUnauthorized)(&base),
		(*ogen.MemoryIngestMemoryIngestPostNotFound)(&base),
		(*ogen.MemoryIngestMemoryIngestPostUnprocessableEntity)(&base),
		nil,
	}
	for _, response := range addErrors {
		if _, err := mapAddResponse(response); err == nil {
			t.Fatalf("add response %T accepted", response)
		}
	}

	regularErrors := []ogen.MemoryRetrievalMemoryRetrievalPostRes{
		(*ogen.MemoryRetrievalMemoryRetrievalPostBadRequest)(&base),
		(*ogen.MemoryRetrievalMemoryRetrievalPostUnauthorized)(&base),
		(*ogen.MemoryRetrievalMemoryRetrievalPostNotFound)(&base),
		(*ogen.MemoryRetrievalMemoryRetrievalPostUnprocessableEntity)(&base),
		nil,
	}
	for _, response := range regularErrors {
		if _, err := mapSearchResponse(response, "q"); err == nil {
			t.Fatalf("search response %T accepted", response)
		}
	}

	hybridErrors := []ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostRes{
		(*ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostBadRequest)(&base),
		(*ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostUnauthorized)(&base),
		(*ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostNotFound)(&base),
		(*ogen.MemoryHybridRetrievalMemoryRetrievalHybridPostUnprocessableEntity)(&base),
		nil,
	}
	for _, response := range hybridErrors {
		if _, err := mapHybridSearchResponse(response, "q"); err == nil {
			t.Fatalf("hybrid response %T accepted", response)
		}
	}

	jobErrors := []ogen.IngestJobStatusMemoryIngestJobsJobIDGetRes{
		(*ogen.IngestJobStatusMemoryIngestJobsJobIDGetBadRequest)(&base),
		(*ogen.IngestJobStatusMemoryIngestJobsJobIDGetUnauthorized)(&base),
		(*ogen.IngestJobStatusMemoryIngestJobsJobIDGetNotFound)(&base),
		(*ogen.IngestJobStatusMemoryIngestJobsJobIDGetUnprocessableEntity)(&base),
		nil,
	}
	for _, response := range jobErrors {
		if _, err := mapJobStatusResponse(response); err == nil {
			t.Fatalf("job response %T accepted", response)
		}
	}
	if _, err := mapJobStatusResponse(&ogen.EnvelopeIngestJobStatus{Success: falseValue, Message: message, Code: 400}); err == nil {
		t.Fatal("failed job envelope accepted")
	}
	if _, err := mapJobStatusResponse(&ogen.EnvelopeIngestJobStatus{Success: trueValue}); err == nil {
		t.Fatal("empty job envelope accepted")
	}
	status, err := mapJobStatusResponse(&ogen.EnvelopeIngestJobStatus{
		Success: trueValue,
		Data: ogen.NewOptNilIngestJobStatus(ogen.IngestJobStatus{
			JobID: ogen.NewOptNilString("job"), Status: ogen.NewOptNilString("done"),
		}),
	})
	if err != nil || status.Status != "done" {
		t.Fatalf("job status = %#v, %v", status, err)
	}

	if _, err := mapSearchEnvelope(&ogen.EnvelopeMemoryRetrievalOut{Success: falseValue, Message: message}, "fallback"); err == nil {
		t.Fatal("failed search envelope accepted")
	}
	if _, err := mapSearchEnvelope(&ogen.EnvelopeMemoryRetrievalOut{Success: trueValue}, "fallback"); err == nil {
		t.Fatal("empty search envelope accepted")
	}
	result, err := mapSearchEnvelope(&ogen.EnvelopeMemoryRetrievalOut{
		Success: trueValue,
		Data: ogen.NewOptNilMemoryRetrievalOut(ogen.MemoryRetrievalOut{
			Evidence: []string{"e"},
			EvidenceDetails: []ogen.EvidenceDetailOut{{
				EventID: "e", Text: "fact", Source: ogen.NewOptString("memory"),
				Role: ogen.NewOptNilString("user"), SenderName: ogen.NewOptNilString("Ada"),
				AtomicFacts: []string{"atomic"}, GroupID: ogen.NewOptNilString("g"),
				Timestamp: ogen.NewOptNilString("2026-01-01T00:00:00Z"),
			}},
			RequestedGroupID: ogen.NewOptNilString("g"),
		}),
	}, "fallback")
	if err != nil || result.Query != "fallback" || result.RequestedGroupID == nil || len(result.EvidenceDetails) != 1 {
		t.Fatalf("search result = %#v, %v", result, err)
	}
	if _, err := mapSearchResponse(&ogen.EnvelopeMemoryRetrievalOut{Success: trueValue, Data: ogen.NewOptNilMemoryRetrievalOut(ogen.MemoryRetrievalOut{Query: ogen.NewOptString("q")})}, "fallback"); err != nil {
		t.Fatal(err)
	}
	if _, err := mapHybridSearchResponse(&ogen.EnvelopeMemoryRetrievalOut{Success: trueValue, Data: ogen.NewOptNilMemoryRetrievalOut(ogen.MemoryRetrievalOut{Query: ogen.NewOptString("q")})}, "fallback"); err != nil {
		t.Fatal(err)
	}
}

func TestResponseHelpers(t *testing.T) {
	if successEnvelopeError(200, ogen.NewOptBool(true), ogen.OptString{}, 200) != nil {
		t.Fatal("successful envelope rejected")
	}
	empty := emptyDataError(202, 0, "")
	if empty.Code != 202 || empty.Message == "" {
		t.Fatalf("empty data error = %#v", empty)
	}
	base := ogen.ErrorEnvelope{}
	err := errorEnvelope(401, base)
	if err.Code != 401 || err.Message == "" || err.ErrorCode != "" || err.Detail != nil {
		t.Fatalf("error envelope = %#v", err)
	}
	if optionalString(ogen.NewOptString("x")) != "x" || optionalNilString(ogen.NewOptNilString("x")) != "x" {
		t.Fatal("optional string helpers")
	}
	if optionalNilStringPointer(ogen.OptNilString{}) != nil {
		t.Fatal("unset optional pointer")
	}
	if value := optionalNilStringPointer(ogen.NewOptNilString("x")); value == nil || *value != "x" {
		t.Fatal("set optional pointer")
	}
}
