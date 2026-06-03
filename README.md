# bunshin-go

[![CI](https://github.com/stlimtat/bunshin-go/actions/workflows/ci.yml/badge.svg)](https://github.com/stlimtat/bunshin-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/stlimtat/bunshin-go.svg)](https://pkg.go.dev/github.com/stlimtat/bunshin-go)
[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://go.dev/doc/install)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

![A gopher with a lattice that synthesizes the concepts of "chaining" LangChain and "cloning" Bunshin using modern, technical shapes and subtle, safe references to ninja tools](/docs/static/img/bunshin-logo-01.png)

A Go port of LangChain / LangGraph / LangSmith — production-grade LLM pipeline primitives with native concurrency, type safety, and single-binary deploys.

| Problem | Python LangChain | bunshin-go |
|---------|-----------------|------------|
| Concurrency | GIL limits parallelism | goroutines, zero overhead |
| Type safety | Runtime errors, brittle | Generics, compile-time checks |
| Deploy | Virtual envs, deps | Single static binary |
| Latency | 50–200 ms startup | Sub-1 ms startup |
| Context windows | 2M tokens = 2 GB RAM | Reference/cursor, O(1) RAM |

**Docs:** [architecture](docs/content/architecture/index.md) · [quickstart](docs/content/quickstart/index.md) · [concepts](docs/content/concepts/index.md) · [how-tos](docs/content/howto/)

---

## Install

```bash
go get github.com/stlimtat/bunshin-go
```

```go
provider := llm.NewFakeProvider("openai", "Hello from bunshin-go!")
result, _ := provider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "What is Go?")},
})
fmt.Println(result.Content)
```

---

## CLI

```bash
go build -o bunshin ./cmd/bunshin
```

| Subcommand | What it does |
|------------|--------------|
| `bunshin llm` | Single provider LLM call |
| `bunshin chain` | Two-step entity extraction chain |
| `bunshin agent` | Agent loop with arithmetic tools |
| `bunshin mcp-sandbox` | MCP tool discovery + sandboxed code execution |
| `bunshin serve` | Start the HTTP workflow server |
| `bunshin health` | Self-check (used by Docker healthcheck) |
| `bunshin version` | Print version |
| `bunshin docs` | Generate LLM-ready CLI markdown docs |

```bash
bunshin llm --message "What is Go?"
bunshin agent --question "What is 6*7?"
bunshin serve --addr :9090
```

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BUNSHIN_PROVIDER` | `fake` | `fake\|openai\|anthropic\|google\|ollama` |
| `BUNSHIN_MODEL` | _(provider default)_ | Model ID |
| `BUNSHIN_API_KEY` | | API key |
| `BUNSHIN_LOG_LEVEL` | `info` | `debug\|info\|warn\|error` |
| `BUNSHIN_ADDR` | `:8080` | HTTP listen address |

```bash
BUNSHIN_PROVIDER=openai BUNSHIN_API_KEY=sk-... bunshin llm --message "Hello"
```

---

## Docker

```bash
docker compose -f deployments/docker-compose.yml up

curl -X POST http://localhost:8080/workflows/echo \
  -H "Content-Type: application/json" \
  -d '{"input": {"message": "hello"}}'

curl -N "http://localhost:8080/workflows/echo/stream?input=%7B%22message%22%3A%22hello%22%7D"
```

---

## Testing

```bash
go test -race -count=1 ./...
go test ./pkg/testing/fault/... -v   # chaos / fault injection
go test ./pkg/eval/... -v            # eval harness
```

---

## Project layout

| Directory | Purpose |
|-----------|---------|
| `pkg/` | Public library packages |
| `internal/` | Private shared utilities |
| `cmd/bunshin/` | Unified CLI (cobra + viper) |
| `api/` | OpenAPI 3.1 specification |
| `deployments/` | Dockerfile, docker-compose |
| `docs/` | Documentation site |

---

## Roadmap

- [ ] OpenAI + Anthropic provider adapters
- [ ] Redis and S3 MessageStore backends
- [ ] gRPC transport
- [ ] LangSmith telemetry backend
- [ ] E2B and Docker sandbox backends
- [ ] Real MCP client (stdio + HTTP/SSE transport)

---

## License

MIT — see [LICENSE](LICENSE).
