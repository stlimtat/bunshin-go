# bunshin-go

[![CI](https://github.com/stlimtat/bunshin-go/actions/workflows/ci.yml/badge.svg)](https://github.com/stlimtat/bunshin-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/stlimtat/bunshin-go.svg)](https://pkg.go.dev/github.com/stlimtat/bunshin-go)
[![Go Version](https://img.shields.io/badge/go-1.23+-blue.svg)](https://go.dev/doc/install)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A Go port of LangChain / LangGraph / LangSmith — production-grade LLM pipeline primitives with native concurrency, type safety, and single-binary deploys.

**Why Go instead of Python?**

| Problem | Python LangChain | bunshin-go |
|---------|-----------------|------------|
| Concurrency | GIL limits parallelism | goroutines, zero overhead |
| Type safety | Runtime errors, brittle | Generics, compile-time checks |
| Deploy | Virtual envs, deps | Single static binary |
| Latency | 50–200 ms startup | Sub-1 ms startup |
| Context windows | 2M tokens = 2 GB RAM | Reference/cursor, O(1) RAM |

---

## Quick Start

```bash
go get github.com/stlimtat/bunshin-go
```

```go
// Single LLM call
provider := llm.NewFakeProvider("openai", "Hello from bunshin-go!")
result, _ := provider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "What is Go?")},
})
fmt.Println(result.Content)
```

---

## Examples

| Example | What it shows |
|---------|---------------|
| [`hello-llm`](examples/hello-llm/) | Single provider call |
| [`hello-chain`](examples/hello-chain/) | Two-step chain: fast → slow model |
| [`hello-agent`](examples/hello-agent/) | Agent loop with tool use |
| [`hello-mcp-sandbox`](examples/hello-mcp-sandbox/) | MCP tool discovery + sandboxed code execution |

```bash
go run ./examples/hello-agent
go run ./examples/hello-mcp-sandbox
```

---

## Architecture

```
bunshin-go/
├── pkg/
│   ├── core/          # Runnable interface — the composable unit of work
│   ├── llm/           # LLMProvider interface + canonical message types
│   ├── memory/        # MessageStore with 2M-token sliding window
│   ├── middleware/     # Chain, WithLogging, WithRetry, WithTimeout, …
│   ├── prompt/        # Fragment → PromptTemplate → PromptBackend
│   ├── telemetry/     # TelemetryBackend (LangSmith + OTEL)
│   ├── checkpoint/    # Checkpointer + Journal for resume/replay recovery
│   ├── eval/          # EvalRunner + built-in evaluators
│   ├── chain/         # Sequential step composition
│   ├── graph/         # DAG executor with conditional routing
│   ├── transport/     # HTTPTransport (SSE), StreamTransport
│   ├── tools/         # Tool interface + ToolRegistry
│   ├── mcp/           # MCP client + server (Model Context Protocol)
│   ├── sandbox/       # Pluggable code execution (E2B, Docker, WASM)
│   └── testing/fault/ # FaultInjector for chaos testing
├── internal/
│   └── credentials/   # Credential injection (context + Provider interface)
├── examples/          # Runnable hello-world programs
├── api/               # OpenAPI 3.1 specification
└── deployments/       # Dockerfile + docker-compose
```

### Core abstraction: `Runnable`

Every component — LLM call, chain step, graph node, tool — implements one interface:

```go
type Runnable interface {
    Name() string
    Invoke(ctx context.Context, input any) (any, error)
    Stream(ctx context.Context, input any) (<-chan StreamChunk, error)
}
```

Wrap typed functions with `AsRunnable[In, Out]` for compile-time type safety at definition time, composition safety at seams.

### Middleware

Five interception levels, applied with `middleware.Chain`:

```go
agent := middleware.Chain(graph,
    middleware.WithLogging(logger),     // L2: structured logs
    middleware.WithRetry(cfg),          // L2: exponential backoff
    middleware.WithPanicRecovery(),     // L2: convert panics → errors
    middleware.WithTimeout(30*time.Second),
)
```

### Credential injection

Per-request credentials flow through `context.Context`, decoupled from client construction:

```go
ctx = credentials.WithCredential(ctx, "openai", credentials.APIKeyCredential(os.Getenv("OPENAI_API_KEY")))
result, err := mcpClient.CallTool(ctx, "search", input)
```

Or inject at client construction via a `credentials.Provider` backed by AWS Secrets Manager, Vault, or GCP Secret Manager.

---

## REST API

The `HTTPTransport` exposes two endpoints. Full spec: [`api/openapi.yaml`](api/openapi.yaml).

```
POST /workflows/{id}          — synchronous execution
GET  /workflows/{id}/stream   — SSE streaming (LLM tokens + step events)
```

**SSE event types:** `step_start`, `llm_token`, `step_end`, `error`, `done`.

---

## Testing

```bash
# Unit + integration tests with race detector
go test -race -count=1 ./...

# Fault injection / chaos tests
go test ./pkg/testing/fault/... -v

# Eval harness
go test ./pkg/eval/... -v
```

---

## Docker

```bash
# Start workflow server + Redis
docker compose -f deployments/docker-compose.yml up

# Call the echo workflow
curl -X POST http://localhost:8080/workflows/echo \
  -H "Content-Type: application/json" \
  -d '{"input": {"message": "hello"}}'

# Stream output
curl -N http://localhost:8080/workflows/echo/stream?input=%7B%22message%22%3A%22hello%22%7D
```

---

## Project layout

Follows [golang-standards/project-layout](https://github.com/golang-standards/project-layout):

| Directory | Purpose |
|-----------|---------|
| `pkg/` | Public library packages |
| `internal/` | Private shared utilities (credentials, backoff) |
| `examples/` | Runnable demo programs |
| `api/` | OpenAPI 3.1 specification |
| `deployments/` | Dockerfile, docker-compose |
| `.github/workflows/` | CI pipeline |

---

## Roadmap

- [ ] OpenAI + Anthropic provider adapters
- [ ] Redis and S3 MessageStore backends
- [ ] gRPC transport
- [ ] LangSmith telemetry backend
- [ ] `cmd/bunshin` CLI (serve, eval run, checkpoint list)
- [ ] E2B and Docker sandbox backends
- [ ] Real MCP client (stdio + HTTP/SSE transport)

---

## License

MIT — see [LICENSE](LICENSE).
