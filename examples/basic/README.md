# Basic Example

[简体中文](README.zh-CN.md)

Run from the repository root with a server-side API key:

```bash
MEMAURA_API_KEY=qbk_... go run ./examples/basic
```

The program writes one turn, waits for its ingestion job to reach a terminal status, then performs hybrid retrieval. It is intentionally a server-side example; never expose the API key in a browser or mobile client.
