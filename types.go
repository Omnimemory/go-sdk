// Package memaura is the official Go SDK for the MemAura memory data plane.
package memaura

import "net/http"

const defaultEndpoint = "https://api.omnimemory.cn/api/v2"

// Config configures a Memory client.
type Config struct {
	// APIKey authenticates data-plane requests. Keep it on trusted servers only.
	APIKey string
	// Endpoint overrides the MemAura API endpoint.
	Endpoint string
	// DeviceNo is sent as X-Device-No unless an operation overrides it.
	DeviceNo string
	// HTTPClient optionally replaces the default HTTP client.
	HTTPClient *http.Client
}

// Role identifies the author of a memory turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// Turn is one message submitted for asynchronous memory ingestion.
type Turn struct {
	Content    string
	Role       Role
	TurnID     string
	Timestamp  string
	SenderName string
	ReferList  []string
}

// AddInput describes a memory-ingestion request.
type AddInput struct {
	Turns     []Turn
	SessionID string
	GroupID   string
	GroupName string
	CommitID  string
	DeviceNo  string
}

// JobAck confirms that an ingestion request has been accepted.
type JobAck struct {
	JobID     string
	SessionID string
	Status    string
	StatusURL string
}

// SearchInput describes a regular or hybrid memory retrieval request.
type SearchInput struct {
	Query    string
	Limit    int
	GroupID  string
	DeviceNo string
}

// EvidenceDetail is one ordered item of retrieval evidence.
type EvidenceDetail struct {
	EventID     string
	Text        string
	Source      string
	Role        *string
	SenderName  *string
	AtomicFacts []string
	GroupID     *string
	Timestamp   *string
}

// SearchResult is the normalized result of a regular or hybrid retrieval.
type SearchResult struct {
	Strategy         string
	Query            string
	Evidence         []string
	EvidenceDetails  []EvidenceDetail
	RequestedGroupID *string
}

// JobStatus is the current state of an asynchronous ingestion task.
type JobStatus struct {
	JobID     string
	SessionID string
	Status    string
}
