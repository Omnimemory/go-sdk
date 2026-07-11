# 基础示例

[English](README.md)

在仓库根目录使用服务端 API Key 运行：

```bash
MEMAURA_API_KEY=qbk_... go run ./examples/basic
```

程序会写入一条消息、等待写入任务进入终态，再进行混合检索。这是服务端示例；不要在浏览器或移动端客户端暴露 API Key。
