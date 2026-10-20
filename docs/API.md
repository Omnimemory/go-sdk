# API Reference

[简体中文](API.zh-CN.md)

## V4 API

`NewMemoryV4(Config)` creates the additive V4 client. Its default endpoint is `https://api.omnimemory.cn/api/v3`; the legacy `NewMemory` client remains unchanged.

### Ingest

`Ingest(ctx, V4IngestInput)` posts turns to `/memory/ingest_4` and returns a `V4Job`. The input supports language (`auto`, `zh`, or `en`), timezone, per-operation device number, wait target (`staged` or `searchable`), and a Router wait timeout of at most 300 seconds.

V4 ingestion is sent exactly once. The SDK never retries a write automatically; after an ambiguous timeout or transport failure, use the retained job ID or an application-level idempotency strategy instead of blindly resubmitting.

### Retrieval

`Retrieve(ctx, V4RetrievalInput)` posts to `/memory/retrieval_4` and returns normalized `V4Card` values with compact `V4Episode` sources. Filters include `TopK`, `VectorWeight`, `TagKeys`, `Role`, `ScopeType`, `StartTime`, `EndTime`, `EntityID`, `EntityMatchMode`, and `ParticipantRoles`. `DeviceNo`, `IncludeQueryAnalysis`, and `IncludeDetails` control request scope and optional response data.

`ScopeType` accepts `account`, `api_key`, or `device`; `EntityMatchMode` accepts `primary`, `involved`, or `source`.

### Jobs

`memory.Jobs().Get(ctx, jobID)` reads `/memory/ingest_4/jobs/{jobID}`. `Wait` polls only that job until `succeeded` or `failed`; pending states are `queued`, `staged`, `processing`, `buffered`, and `indexing`. It supports `WithPollInterval` and `WithWaitTimeout`, defaulting to one second and five minutes. A timeout returns `*V4TimeoutError`, which unwraps to `context.DeadlineExceeded`.

## Legacy V2 API

### Client

`NewMemory(Config)` validates the API key and endpoint immediately. The default endpoint is `https://api.omnimemory.cn/api/v2`.

`HTTPClient` is optional. Prefer request-scoped deadlines through `context.Context`; the SDK passes that context to every generated HTTP operation.

### Ingest

```go
ack, err := memory.Add(ctx, memaura.AddInput{
	Turns: []memaura.Turn{{Content: "...", Role: memaura.RoleUser}},
	SessionID: "optional-ordering-session",
	GroupID: "optional-retrieval-group",
	GroupName: "optional-display-name",
	CommitID: "idempotency-key",
	DeviceNo: "optional-operation-device",
})
```

`Add` returns after the Router accepts the job. `JobAck.Status` is normally `queued`; it is not proof that memory materialization has completed. Use `Jobs().Get` or `Jobs().Wait` for task progress.

`CommitID` prevents duplicate enqueueing for retried writes in the same Router idempotency scope. Do not let the SDK retry ingestion automatically because callers must control the idempotency key.

### Retrieval

```go
input := memaura.SearchInput{
	Query: "What is the review schedule?",
	Limit: 3,       // zero omits top_k
	GroupID: "team-a",
	DeviceNo: "optional-operation-device",
}

regular, err := memory.Search(ctx, input)
hybrid, err := memory.SearchHybrid(ctx, input)
```

`Search` and `SearchHybrid` are separate operations. Setting a device number never changes the selected retrieval strategy. `SearchResult.EvidenceDetails` is ordered evidence suitable for downstream context assembly.

### Jobs

```go
status, err := memory.Jobs().Get(ctx, ack.JobID)

finished, err := memory.Jobs().Wait(ctx, ack.JobID,
	memaura.WithPollInterval(500*time.Millisecond),
	memaura.WithWaitTimeout(30*time.Second),
)
```

The status endpoint scopes jobs by the API-key tenant and job ID. It does not require a device number. `Wait` returns terminal jobs, including successful `accumulated` and terminal failed/cancelled values. It returns `*TimeoutError` when the selected deadline expires.

## Errors

Use `errors.As` for API failures and `errors.Is` for deadline checks:

```go
var apiErr *memaura.APIError
if errors.As(err, &apiErr) {
	log.Printf("router error: status=%d code=%d error=%s", apiErr.StatusCode, apiErr.Code, apiErr.ErrorCode)
}

if errors.Is(err, context.DeadlineExceeded) {
	// A caller deadline or Wait timeout expired.
}
```
