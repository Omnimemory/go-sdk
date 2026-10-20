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

- `Add` is asynchronous. Use `CommitID` for idempotent retry behavior.
- `Search` always uses `/memory/retrieval`; `SearchHybrid` always uses `/memory/retrieval/hybrid`.
- A configured `DeviceNo`, or an operation-level `DeviceNo`, is sent as `X-Device-No` for ingestion and retrieval.
- `Jobs().Wait` polls once per second by default and has a one-minute default timeout. Use `WithPollInterval` and `WithWaitTimeout` to change these values. `accumulated` is a successful terminal state.

See the full [API reference](docs/API.md) and the [basic example](examples/basic).

## Error Handling

- `*memaura.ValidationError`: invalid local input; no request was sent.
- `*memaura.APIError`: a Router error envelope; inspect `StatusCode`, `Code`, `ErrorCode`, and `Detail`.
- `*memaura.TimeoutError`: job waiting reached its deadline. It unwraps to `context.DeadlineExceeded` and includes `LastJob` when available.
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
```

Git tags in the form `vX.Y.Z` are Go Module release versions. CI verifies both supported Go releases before a tag is created.

## License

[MIT](LICENSE)
