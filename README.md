# MemAura Go SDK

[简体中文](README.zh-CN.md)

The official Go SDK for the MemAura API-key memory data plane. It provides a stable `Memory` facade for asynchronous ingestion, regular retrieval, explicit hybrid retrieval, and ingestion-job tracking.

## Requirements

- Go 1.25 or Go 1.26
- A MemAura API key (`qbk_...`)

## Install

```bash
go get github.com/Omnimemory/go-sdk@latest
```

## Quick Start

```go
package main

import (
	"context"
	"os"

	memaura "github.com/Omnimemory/go-sdk"
)

func main() {
	ctx := context.Background()
	memory, err := memaura.NewMemory(memaura.Config{
		APIKey:   os.Getenv("MEMAURA_API_KEY"),
		DeviceNo: "backend-worker-1",
	})
	if err != nil {
		panic(err)
	}

	ack, err := memory.Add(ctx, memaura.AddInput{
		CommitID: "conversation-42:1",
		Turns: []memaura.Turn{{
			Content: "The product review is next Wednesday at 3 PM.",
			Role:    memaura.RoleUser,
		}},
	})
	if err != nil {
		panic(err)
	}

	if _, err := memory.Jobs().Wait(ctx, ack.JobID); err != nil {
		panic(err)
	}

	result, err := memory.SearchHybrid(ctx, memaura.SearchInput{
		Query: "When is the product review?",
		Limit: 3,
	})
	if err != nil {
		panic(err)
	}
	_ = result
}
```

## V4 Card/Episode API

`NewMemoryV4` creates an additive client for the current `/api/v3` data plane.
`NewMemory` remains unchanged for existing `/api/v2` integrations.

```go
memory, err := memaura.NewMemoryV4(memaura.Config{
	APIKey: os.Getenv("MEMAURA_API_KEY"), DeviceNo: "DEVICE-001",
})
if err != nil { log.Fatal(err) }

ack, err := memory.Ingest(ctx, memaura.V4IngestInput{Turns: []memaura.V4Turn{
	{Role: memaura.RoleUser, Content: "I prefer product reviews on Wednesday afternoons."},
	{Role: memaura.RoleAssistant, Content: "Noted."},
}})
if err != nil { log.Fatal(err) }
job, err := memory.Jobs().Wait(ctx, ack.JobID, memaura.WithWaitTimeout(5*time.Minute))
if err != nil { log.Fatal(err) }
if job.Status == "failed" { log.Fatalf("ingest failed: %v", job.Error) }

result, err := memory.Retrieve(ctx, memaura.V4RetrievalInput{
	Query: "When do I prefer product reviews?", TopK: 5,
})
if err != nil { log.Fatal(err) }
```

### Client and ingestion

`NewMemoryV4(Config)` uses `Config.APIKey` and the optional `Endpoint`,
`DeviceNo`, and `HTTPClient`. Its default endpoint is
`https://api.omnimemory.cn/api/v3`; the default HTTP client has a 30-second
timeout.

`Ingest(ctx, V4IngestInput)` calls `POST /memory/ingest_4` and returns `V4Job`.

| `V4IngestInput` field | Meaning |
| --- | --- |
| `Turns []V4Turn` | Required non-empty messages. |
| `Language string` | `auto` (default), `zh`, or `en`. |
| `Timezone string` | Optional timezone. |
| `DeviceNo string` | Per-operation device override. |
| `WaitFor string` | `staged` (default) or `searchable`. |
| `WaitTimeoutSeconds float64` | Router wait, default 120 and maximum 300 seconds. |

Each `V4Turn` has `Content` plus optional `Role`, `TurnID`, `Timestamp`,
`SenderName`, and `ReferList`.

### Retrieval

`Retrieve(ctx, V4RetrievalInput)` calls `POST /memory/retrieval_4`.

| `V4RetrievalInput` field | Meaning |
| --- | --- |
| `Query string` | Required non-empty query. |
| `TopK int` | Zero omits the field; otherwise 1–100. |
| `VectorWeight *float64` | Defaults to 0.8; accepted range is 0–1. |
| `TagKeys []string` | Hard tag filters. |
| `Role string` | Card role filter. |
| `ScopeType string` | `account`, `api_key`, or `device`. |
| `StartTime`, `EndTime string` | Explicit time range. |
| `EntityID string` | Entity filter. |
| `EntityMatchMode string` | `primary`, `involved`, or `source`. |
| `ParticipantRoles []string` | Participant-role filters. |
| `DeviceNo string` | Per-operation device override. |
| `IncludeQueryAnalysis bool` | Include query analysis. |
| `IncludeDetails bool` | Preserve the complete response in `Details`. |

`V4RetrievalResult` contains `Cards`, optional `QueryAnalysis`, and optional
`Details`. Each `V4Card` has `CardID`, `Role`, `Content`, and `Episodes`; each
`V4Episode` has `EpisodeID` and `Summary`.

### V4 jobs

- `memory.Jobs().Get(ctx, jobID)` reads `GET /memory/ingest_4/jobs/{jobID}`.
- `memory.Jobs().Wait(ctx, jobID, options...)` polls only the existing job.
- `WithPollInterval(duration)` and `WithWaitTimeout(duration)` configure the wait; defaults are one second and five minutes.

`Jobs()` returns `*V4JobOperations`; its variadic configuration values implement
the exported `WaitOption` interface.

`V4Job` contains `JobID`, `Status`, `WaitTimedOut`, and `Error`. Pending states
are `queued`, `staged`, `processing`, `buffered`, and `indexing`; terminal
states are `succeeded` and `failed`. Expiry returns `*V4TimeoutError`, which
contains `JobID` and `LastJob` and unwraps to `context.DeadlineExceeded`.

V4 writes are never retried automatically. A local wait timeout does not
cancel the job; continue polling the same job ID. Retrieval supports TopK,
weighted vector/text fusion, hard tags, role and scope selection, explicit time
ranges, and entity/participant filters.

## Legacy V2 API

```go
memory, err := memaura.NewMemory(memaura.Config{
	APIKey:     "qbk_...", // required
	Endpoint:   "https://api.omnimemory.cn/api/v2", // optional
	DeviceNo:   "service-a", // optional default device namespace
	HTTPClient: nil, // optional custom *http.Client
})

ack, err := memory.Add(ctx, memaura.AddInput{...})
regular, err := memory.Search(ctx, memaura.SearchInput{Query: "..."})
hybrid, err := memory.SearchHybrid(ctx, memaura.SearchInput{Query: "..."})
status, err := memory.Jobs().Get(ctx, ack.JobID)
finished, err := memory.Jobs().Wait(ctx, ack.JobID)
```

### V2 public types and methods

- `Config`: `APIKey`, optional `Endpoint`, `DeviceNo`, and `HTTPClient`.
- `Turn`: `Content`, `Role`, `TurnID`, `Timestamp`, `SenderName`, and `ReferList`.
- `AddInput`: `Turns`, `SessionID`, `GroupID`, `GroupName`, `CommitID`, and `DeviceNo`.
- `Add(ctx, AddInput) (JobAck, error)`: returns `JobID`, `SessionID`, `Status`, and `StatusURL`.
- `Search(ctx, SearchInput)` and `SearchHybrid(ctx, SearchInput)`: `SearchInput` contains `Query`, `Limit`, `GroupID`, and `DeviceNo`.
- `Jobs().Get(ctx, jobID)` and `Jobs().Wait(ctx, jobID, options...)`: return `JobStatus` with `JobID`, `SessionID`, and `Status`.

`Jobs()` returns `*JobOperations`; `Wait` uses the same `WaitOption` values.

`RoleUser`, `RoleAssistant`, and `RoleSystem` are the predefined `Role` values.
`SearchResult` contains `Strategy`, `Query`, `Evidence`, ordered
`EvidenceDetails`, and optional `RequestedGroupID`. Each `EvidenceDetail`
contains `EventID`, `Text`, `Source`, optional `Role`, optional `SenderName`,
`AtomicFacts`, optional `GroupID`, and optional `Timestamp`.

- `Add` is asynchronous. Use `CommitID` for idempotent retry behavior.
- `Search` always uses `/memory/retrieval`; `SearchHybrid` always uses `/memory/retrieval/hybrid`.
- A configured `DeviceNo`, or an operation-level `DeviceNo`, is sent as `X-Device-No` for ingestion and retrieval.
- `Jobs().Wait` polls once per second by default and has a one-minute default timeout. Use `WithPollInterval` and `WithWaitTimeout` to change these values. `accumulated` is a successful terminal state.

See the full [API reference](docs/API.md) and the [basic example](examples/basic).

## Error Handling

- `*memaura.ValidationError`: invalid local input; no request was sent.
- `*memaura.APIError`: a Router error envelope; inspect `StatusCode`, `Code`, `ErrorCode`, and `Detail`.
- `*memaura.TimeoutError`: job waiting reached its deadline. Its `Unwrap()` matches `context.DeadlineExceeded` and it includes `LastJob` when available.
- `*memaura.V4TimeoutError`: the equivalent V4 wait timeout with `JobID` and `LastJob`.
- Network, transport, and caller-context errors are returned with their original cause.

## Security

This SDK is for trusted server-side Go applications. Do not ship a `qbk_...` API key in a browser, mobile application, desktop client, or other untrusted distribution.

## Contract and Development

The checked-in OpenAPI snapshot is intentionally limited to four API-key data-plane operations. The internal client is generated with `ogen v1.20.3`; generated files are committed and must not be edited manually.

```bash
# Requires a local Go 1.25+ toolchain and jq.
make snapshot OPENAPI_SOURCE=/path/to/router/docs/openapi.v2.json
make generate
make check-generated
go test -race ./...
make coverage
```

`make coverage` requires 100% statement coverage for the complete hand-written
root package. Generated `internal/generated` packages and runnable examples are
explicitly excluded from the metric; they are still generated, built, and
validated separately by CI.

Git tags in the form `vX.Y.Z` are Go Module release versions. CI verifies both supported Go releases before a tag is created.

## License

[MIT](LICENSE)
