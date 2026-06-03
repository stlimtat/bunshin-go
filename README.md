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

## CLI

All demo programs and the server are unified under the `bunshin` CLI.

### Build

```bash
go build -o bunshin ./cmd/bunshin
```

### Subcommands

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

### Examples

```bash
# Fake provider (no key needed)
bunshin llm --message "What is Go?"

# Two-step chain

# Agent with arithmetic
bunshin agent --question "3 + 4"
bunshin agent --question "What is 6*7?"

# MCP sandbox demo
bunshin mcp-sandbox

# Start server on :8080
bunshin serve
bunshin serve --addr :9090

# Generate CLI docs to ./cli-docs/
bunshin docs --output-dir ./cli-docs
```

### Environment Variables

All flags can be set via environment variables with the `BUNSHIN_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `BUNSHIN_PROVIDER` | `fake` | LLM provider: `fake\|openai\|anthropic\|google\|ollama` |
| `BUNSHIN_MODEL` | _(provider default)_ | Model ID |
| `BUNSHIN_API_KEY` | | API key for the chosen provider |
| `BUNSHIN_LOG_LEVEL` | `info` | Log level: `debug\|info\|warn\|error` |
| `BUNSHIN_ADDR` | `:8080` | HTTP listen address (serve subcommand) |

```bash
BUNSHIN_PROVIDER=openai BUNSHIN_API_KEY=sk-... bunshin llm --message "Hello"
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
│   ├── checkpoint/    # Checkpointer + Journal for resume/replay recovery
│   ├── eval/          # EvalRunner + built-in evaluators
│   ├── chain/         # Sequential step composition
│   ├── graph/         # DAG executor with conditional routing
│   ├── transport/     # HTTPTransport (SSE), health/live/ready/pprof endpoints
│   ├── tools/         # Tool interface + ToolRegistry
│   ├── mcp/           # MCP client + server (Model Context Protocol)
│   ├── sandbox/       # Pluggable code execution (E2B, Docker, WASM)
│   └── testing/fault/ # FaultInjector for chaos testing
├── internal/
│   ├── credentials/   # Credential injection (context + Provider interface)
│   └── telemetry/     # zerolog init, slog bridge, TelemetryBackend, OTELBackend
├── cmd/bunshin/       # Unified CLI (cobra + viper)
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
    middleware.WithLogging(logger),     // L2: structured logs (zerolog)
    middleware.WithRetry(cfg),          // L2: exponential backoff
    middleware.WithPanicRecovery(),     // L2: convert panics → errors
    middleware.WithTimeout(30*time.Second),
)
```

### Logging

All structured logs use [zerolog](https://github.com/rs/zerolog). Initialize once at startup:

```go
logger := telemetry.NewLogger("info") // also bridges slog → zerolog for third-party libs
```

### Credential injection

Per-request credentials flow through `context.Context`, decoupled from client construction:

```go
ctx = credentials.WithCredential(ctx, "openai", credentials.APIKeyCredential(os.Getenv("OPENAI_API_KEY")))
result, err := mcpClient.CallTool(ctx, "search", input)
```

---

## REST API

The `HTTPTransport` exposes:

```
POST /workflows/{id}          — synchronous execution
GET  /workflows/{id}/stream   — SSE streaming (LLM tokens + step events)
GET  /health                  — healthcheck (Docker/LB probe)
GET  /live                    — liveness probe (Kubernetes)
GET  /ready                   — readiness probe (Kubernetes)
GET  /debug/pprof/*           — Go pprof profiling
```

**SSE event types:** `step_start`, `llm_token`, `step_end`, `error`, `done`.

Full spec: [`api/openapi.yaml`](api/openapi.yaml).

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
# Start workflow server
docker compose -f deployments/docker-compose.yml up

# Call the echo workflow
curl -X POST http://localhost:8080/workflows/echo \
  -H "Content-Type: application/json" \
  -d '{"input": {"message": "hello"}}'

# Stream output
curl -N "http://localhost:8080/workflows/echo/stream?input=%7B%22message%22%3A%22hello%22%7D"

# Health check
curl http://localhost:8080/health
```

The Docker image uses `bunshin health` as the container healthcheck — no extra tooling required in the distroless image.

---

## Project layout

Follows [golang-standards/project-layout](https://github.com/golang-standards/project-layout):

| Directory | Purpose |
|-----------|---------|
| `pkg/` | Public library packages |
| `internal/` | Private shared utilities (credentials, telemetry/logging) |
| `cmd/bunshin/` | Unified CLI (cobra + viper) |
| `api/` | OpenAPI 3.1 specification |
| `deployments/` | Dockerfile, docker-compose |
| `.github/workflows/` | CI pipeline |

---

## Roadmap

- [ ] OpenAI + Anthropic provider adapters
- [ ] Redis and S3 MessageStore backends
- [ ] gRPC transport
- [ ] LangSmith telemetry backend (OTEL backend ships via `internal/telemetry.NewOTELBackend`)
- [ ] E2B and Docker sandbox backends
- [ ] Real MCP client (stdio + HTTP/SSE transport)

---

## License

MIT — see [LICENSE](LICENSE).
