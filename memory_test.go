package memaura_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	memaura "github.com/Omnimemory/go-sdk"
)

func TestNewMemoryRejectsBlankAPIKey(t *testing.T) {
	t.Parallel()

	_, err := memaura.NewMemory(memaura.Config{})
	if err == nil {
		t.Fatal("NewMemory accepted a blank API key")
	}
}

func TestMemoryAddSendsStablePayload(t *testing.T) {
	var received map[string]any
	memory := newTestMemory(t, "device-default", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/memory/ingest" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "qbk_test" {
			t.Fatalf("X-API-Key = %q, want qbk_test", got)
		}
		if got := r.Header.Get("X-Device-No"); got != "device-input" {
			t.Fatalf("X-Device-No = %q, want device-input", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(t, w, http.StatusAccepted, map[string]any{
			"success": true,
			"code":    http.StatusAccepted,
			"data": map[string]any{
				"job_id":     "job-123",
				"session_id": "session-1",
				"status":     "queued",
				"status_url": "/memory/ingest/jobs/job-123",
			},
		})
	})

	ack, err := memory.Add(context.Background(), memaura.AddInput{
		Turns: []memaura.Turn{{
			Content:    "Schedule the release review",
			Role:       memaura.RoleAssistant,
			TurnID:     "turn-1",
			Timestamp:  "2026-07-11T09:00:00Z",
			SenderName: "Ari",
			ReferList:  []string{"turn-0"},
		}},
		SessionID: "session-1",
		GroupID:   "group-1",
		GroupName: "Release planning",
		CommitID:  "commit-1",
		DeviceNo:  "device-input",
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if ack.JobID != "job-123" || ack.SessionID != "session-1" || ack.Status != "queued" {
		t.Fatalf("Add() = %#v", ack)
	}
	if got := received["session_id"]; got != "session-1" {
		t.Fatalf("session_id = %#v", got)
	}
	if got := received["group_id"]; got != "group-1" {
		t.Fatalf("group_id = %#v", got)
	}
	if got := received["group_name"]; got != "Release planning" {
		t.Fatalf("group_name = %#v", got)
	}
	if got := received["commit_id"]; got != "commit-1" {
		t.Fatalf("commit_id = %#v", got)
	}
	turns, ok := received["turns"].([]any)
	if !ok || len(turns) != 1 {
		t.Fatalf("turns = %#v", received["turns"])
	}
	turn, ok := turns[0].(map[string]any)
	if !ok || turn["turn_id"] != "turn-1" {
		t.Fatalf("turn = %#v", turns[0])
	}
	referList, ok := turn["refer_list"].([]any)
	if !ok || len(referList) != 1 || referList[0] != "turn-0" {
		t.Fatalf("refer_list = %#v", turn["refer_list"])
	}
	if _, ok := received["client_meta"]; ok {
		t.Fatalf("client_meta should not be sent when X-Device-No is used: %#v", received)
	}
}

func TestMemorySearchUsesRegularRoute(t *testing.T) {
	memory := newTestMemory(t, "device-default", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/memory/retrieval" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Device-No"); got != "device-default" {
			t.Fatalf("X-Device-No = %q", got)
		}
		assertSearchPayload(t, r, "project launch", 2, "group-1")
		writeSearchResponse(t, w, "regular")
	})

	result, err := memory.Search(context.Background(), memaura.SearchInput{
		Query:   "project launch",
		Limit:   2,
		GroupID: "group-1",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.Strategy != "regular" || result.Query != "project launch" || len(result.EvidenceDetails) != 1 {
		t.Fatalf("Search() = %#v", result)
	}
}

func TestMemorySearchHybridUsesHybridRoute(t *testing.T) {
	memory := newTestMemory(t, "device-default", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/memory/retrieval/hybrid" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Device-No"); got != "device-input" {
			t.Fatalf("X-Device-No = %q", got)
		}
		assertSearchPayload(t, r, "project launch", 5, "group-1")
		writeSearchResponse(t, w, "hybrid")
	})

	result, err := memory.SearchHybrid(context.Background(), memaura.SearchInput{
		Query:    "project launch",
		Limit:    5,
		GroupID:  "group-1",
		DeviceNo: "device-input",
	})
	if err != nil {
		t.Fatalf("SearchHybrid() error = %v", err)
	}
	if result.Strategy != "hybrid" || result.RequestedGroupID == nil || *result.RequestedGroupID != "group-1" {
		t.Fatalf("SearchHybrid() = %#v", result)
	}
}

func TestMemoryMapsRouterErrorEnvelope(t *testing.T) {
	memory := newTestMemory(t, "", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Invalid query",
			"code":    http.StatusBadRequest,
			"data":    map[string]any{"error": "invalid_query"},
		})
	})

	_, err := memory.Search(context.Background(), memaura.SearchInput{Query: "bad"})
	var apiErr *memaura.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Search() error = %T %v, want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || apiErr.Code != http.StatusBadRequest || apiErr.ErrorCode != "invalid_query" {
		t.Fatalf("APIError = %#v", apiErr)
	}
}

func newTestMemory(t *testing.T, deviceNo string, handler http.HandlerFunc) *memaura.Memory {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	memory, err := memaura.NewMemory(memaura.Config{
		APIKey:   "qbk_test",
		Endpoint: server.URL,
		DeviceNo: deviceNo,
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	return memory
}

func assertSearchPayload(t *testing.T, r *http.Request, query string, limit int, groupID string) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if body["query"] != query || body["top_k"] != float64(limit) || body["group_id"] != groupID {
		t.Fatalf("search payload = %#v", body)
	}
	if _, ok := body["client_meta"]; ok {
		t.Fatalf("client_meta should not be sent when X-Device-No is used: %#v", body)
	}
}

func writeSearchResponse(t *testing.T, w http.ResponseWriter, strategy string) {
	t.Helper()
	writeJSON(t, w, http.StatusOK, map[string]any{
		"success": true,
		"code":    http.StatusOK,
		"data": map[string]any{
			"strategy":           strategy,
			"query":              "project launch",
			"evidence":           []string{"event-1"},
			"requested_group_id": "group-1",
			"evidence_details": []map[string]any{{
				"event_id":     "event-1",
				"text":         "The project launches next week.",
				"source":       "memory",
				"atomic_facts": []string{"launch next week"},
			}},
		},
	})
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
