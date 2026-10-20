# MemAura Go SDK

[English](README.md)

MemAura API Key 数据面的官方 Go SDK。它提供稳定的 `Memory` 门面，支持异步写入、普通检索、显式混合检索和写入任务状态查询。

## 环境要求

- Go 1.25 或 Go 1.26
- MemAura API Key（`qbk_...`）

## 安装

```bash
go get github.com/Omnimemory/go-sdk@latest
```

## 快速开始

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
			Content: "产品评审安排在下周三下午三点。",
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
		Query: "产品评审是什么时候？",
		Limit: 3,
	})
	if err != nil {
		panic(err)
	}
	_ = result
}
```

## V4 Card/Episode API

`NewMemoryV4` 用于创建面向当前 `/api/v3` 数据面的新增客户端；原有 `NewMemory`
继续服务已有的 `/api/v2` 接入，行为不变。

```go
memory, err := memaura.NewMemoryV4(memaura.Config{
	APIKey: os.Getenv("MEMAURA_API_KEY"), DeviceNo: "DEVICE-001",
})
if err != nil { log.Fatal(err) }

ack, err := memory.Ingest(ctx, memaura.V4IngestInput{Turns: []memaura.V4Turn{
	{Role: memaura.RoleUser, Content: "我偏好周三下午进行产品评审。"},
	{Role: memaura.RoleAssistant, Content: "好的，已记下。"},
}})
if err != nil { log.Fatal(err) }
job, err := memory.Jobs().Wait(ctx, ack.JobID, memaura.WithWaitTimeout(5*time.Minute))
if err != nil { log.Fatal(err) }
if job.Status == "failed" { log.Fatalf("写入失败: %v", job.Error) }

result, err := memory.Retrieve(ctx, memaura.V4RetrievalInput{
	Query: "我偏好什么时候进行产品评审？", TopK: 5,
})
if err != nil { log.Fatal(err) }
```

V4 写入不会自动重试。本地等待超时不会取消后台任务，请继续查询同一个任务 ID。检索支持
TopK、向量/文本融合权重、硬标签、角色和作用域、显式时间范围，以及实体和参与角色过滤。

## 旧版 V2 API

```go
memory, err := memaura.NewMemory(memaura.Config{
	APIKey:     "qbk_...", // 必填
	Endpoint:   "https://api.omnimemory.cn/api/v2", // 可选
	DeviceNo:   "service-a", // 可选的默认设备命名空间
	HTTPClient: nil, // 可选的自定义 *http.Client
})

ack, err := memory.Add(ctx, memaura.AddInput{...})
regular, err := memory.Search(ctx, memaura.SearchInput{Query: "..."})
hybrid, err := memory.SearchHybrid(ctx, memaura.SearchInput{Query: "..."})
status, err := memory.Jobs().Get(ctx, ack.JobID)
finished, err := memory.Jobs().Wait(ctx, ack.JobID)
```

- `Add` 为异步操作。为安全重试，请使用 `CommitID`。
- `Search` 始终使用 `/memory/retrieval`；`SearchHybrid` 始终使用 `/memory/retrieval/hybrid`。
- 配置的 `DeviceNo` 或单次调用传入的 `DeviceNo` 会作为 `X-Device-No` 用于写入和检索。
- `Jobs().Wait` 默认每秒轮询一次，默认总超时为一分钟；可通过 `WithPollInterval` 和 `WithWaitTimeout` 调整。`accumulated` 是成功终态。

完整说明见 [API 参考](docs/API.zh-CN.md) 和[基础示例](examples/basic)。

## 错误处理

- `*memaura.ValidationError`：本地参数不合法，尚未发出请求。
- `*memaura.APIError`：Router 返回错误信封，可检查 `StatusCode`、`Code`、`ErrorCode` 和 `Detail`。
- `*memaura.TimeoutError`：等待任务到达截止时间。它可通过 `errors.Is(err, context.DeadlineExceeded)` 识别，并在可用时包含 `LastJob`。
- 网络、传输和调用方 `context` 的错误会保留原始原因。

## 安全边界

SDK 面向可信的服务端 Go 应用。不要将 `qbk_...` API Key 打包到浏览器、移动端、桌面端或其他不可信分发环境中。

## 契约与开发

仓库中的 OpenAPI 快照有意只保留四个 API Key 数据面操作。内部客户端由 `ogen v1.20.3` 生成；生成文件已提交，禁止手改。

```bash
# 需要本地 Go 1.25+ 和 jq。
make snapshot OPENAPI_SOURCE=/path/to/router/docs/openapi.v2.json
make generate
make check-generated
go test -race ./...
```

格式为 `vX.Y.Z` 的 Git tag 即 Go Module 发布版本。创建 tag 前，CI 会验证两个受支持的 Go 版本。

## 许可

[MIT](LICENSE)
