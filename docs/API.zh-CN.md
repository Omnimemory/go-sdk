# API 参考

[English](API.md)

## V4 API

`NewMemoryV4(Config)` 创建增量加入的 V4 客户端，默认端点为 `https://api.omnimemory.cn/api/v3`；原有 `NewMemory` 客户端保持不变。

### 写入

`Ingest(ctx, V4IngestInput)` 将 turns 提交到 `/memory/ingest_4` 并返回 `V4Job`。输入支持语言（`auto`、`zh` 或 `en`）、时区、单次操作设备号、等待目标（`staged` 或 `searchable`），以及最长 300 秒的 Router 等待时间。

V4 写入只发送一次，SDK 不会自动重试。遇到结果不明确的超时或传输错误时，应继续使用已保留的任务 ID，或采用应用层幂等策略，不要直接重复提交。

### 检索

`Retrieve(ctx, V4RetrievalInput)` 请求 `/memory/retrieval_4`，返回归一化的 `V4Card`，并包含精简的 `V4Episode` 来源。过滤项包括 `TopK`、`VectorWeight`、`TagKeys`、`Role`、`ScopeType`、`StartTime`、`EndTime`、`EntityID`、`EntityMatchMode` 和 `ParticipantRoles`；`DeviceNo`、`IncludeQueryAnalysis` 与 `IncludeDetails` 用于控制请求范围和可选响应数据。

`ScopeType` 可取 `account`、`api_key` 或 `device`；`EntityMatchMode` 可取 `primary`、`involved` 或 `source`。

### 任务

`memory.Jobs().Get(ctx, jobID)` 查询 `/memory/ingest_4/jobs/{jobID}`。`Wait` 只轮询该任务，直到 `succeeded` 或 `failed`；等待中的状态包括 `queued`、`staged`、`processing`、`buffered` 和 `indexing`。可通过 `WithPollInterval` 与 `WithWaitTimeout` 配置，默认分别为一秒和五分钟。超时时返回 `*V4TimeoutError`，并可通过 `errors.Is` 匹配 `context.DeadlineExceeded`。

## 旧版 V2 API

### 客户端

`NewMemory(Config)` 会立即校验 API Key 与端点。默认端点为 `https://api.omnimemory.cn/api/v2`。

`HTTPClient` 可选。建议通过 `context.Context` 提供单次请求截止时间；SDK 会将该 context 传入每个 OpenAPI 生成的 HTTP 调用。

### 写入记忆

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

`Add` 在 Router 接受任务后返回。`JobAck.Status` 通常是 `queued`，并不代表记忆已物化完成；请通过 `Jobs().Get` 或 `Jobs().Wait` 获取任务进度。

`CommitID` 用于让同一 Router 幂等范围内的写入重试不重复入队。SDK 不会自动重试写入，调用方必须自行控制幂等键。

### 检索

```go
input := memaura.SearchInput{
	Query: "评审日程是什么？",
	Limit: 3,       // 为零时不传 top_k
	GroupID: "team-a",
	DeviceNo: "optional-operation-device",
}

regular, err := memory.Search(ctx, input)
hybrid, err := memory.SearchHybrid(ctx, input)
```

`Search` 与 `SearchHybrid` 是两个明确独立的操作。设置设备号不会改变检索策略。`SearchResult.EvidenceDetails` 为有序证据，可直接用于下游上下文拼装。

### 任务

```go
status, err := memory.Jobs().Get(ctx, ack.JobID)

finished, err := memory.Jobs().Wait(ctx, ack.JobID,
	memaura.WithPollInterval(500*time.Millisecond),
	memaura.WithWaitTimeout(30*time.Second),
)
```

任务状态接口通过 API Key 对应的租户和任务 ID 查询，不要求设备号。`Wait` 会在任务进入终态时返回，包括成功的 `accumulated` 和失败/取消终态；选定截止时间到达时返回 `*TimeoutError`。

## 错误

可用 `errors.As` 检查 API 错误，用 `errors.Is` 检查超时：

```go
var apiErr *memaura.APIError
if errors.As(err, &apiErr) {
	log.Printf("router error: status=%d code=%d error=%s", apiErr.StatusCode, apiErr.Code, apiErr.ErrorCode)
}

if errors.Is(err, context.DeadlineExceeded) {
	// 调用方 deadline 或 Wait 超时已到达。
}
```
