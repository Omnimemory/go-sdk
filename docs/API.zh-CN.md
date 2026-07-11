# API 参考

[English](API.md)

## 客户端

`NewMemory(Config)` 会立即校验 API Key 与端点。默认端点为 `https://api.omnimemory.cn/api/v2`。

`HTTPClient` 可选。建议通过 `context.Context` 提供单次请求截止时间；SDK 会将该 context 传入每个 OpenAPI 生成的 HTTP 调用。

## 写入记忆

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

## 检索

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

## 任务

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
