+++
title = 'Architecture'
date = '2026-06-03'
draft = false
weight = 5
+++

# Architecture

## Package layout

```
bunshin-go/
├── pkg/
│   ├── core/          # Runnable interface — the composable unit of work
│   ├── llm/           # LLMProvider interface + canonical message types
│   ├── memory/        # MessageStore with 2M-token sliding window
│   ├── middleware/    # Chain, WithLogging, WithRetry, WithTimeout, …
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

## Core abstraction: `Runnable`

Every component — LLM call, chain step, graph node, tool — implements one interface:

```go
type Runnable interface {
    Name() string
    Invoke(ctx context.Context, input any) (any, error)
    Stream(ctx context.Context, input any) (<-chan StreamChunk, error)
}
```

Wrap typed functions with `AsRunnable[In, Out]` for compile-time type safety at definition time, composition safety at seams.

## Middleware

Five interception levels, applied with `middleware.Chain`:

```go
agent := middleware.Chain(graph,
    middleware.WithLogging(logger),     // L2: structured logs
    middleware.WithRetry(cfg),          // L2: exponential backoff + jitter
    middleware.WithPanicRecovery(),     // L2: convert panics → errors
    middleware.WithTimeout(30*time.Second),
)
```

| Level | Scope | Ships |
|-------|-------|-------|
| L1 | Workflow | auth, rate limit, budget, trace start |
| L2 | Runnable | timing, logging, panic recovery, validation |
| L3 | Prompt | PII scrub, injection guard, window trim |
| L4 | LLM call | semantic cache, retry/backoff, model fallback |
| L5 | Tool | authz, sandbox, result validation |

## Logging

All structured logs use [zerolog](https://github.com/rs/zerolog). Initialize once at startup:

```go
logger := telemetry.NewLogger("info") // also bridges slog → zerolog for third-party libs
```

## Credential injection

Per-request credentials flow through `context.Context`, decoupled from client construction:

```go
ctx = credentials.WithCredential(ctx, "openai", credentials.APIKeyCredential(os.Getenv("OPENAI_API_KEY")))
result, err := mcpClient.CallTool(ctx, "search", input)
```

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

Full spec: [`api/openapi.yaml`](https://github.com/stlimtat/bunshin-go/blob/master/api/openapi.yaml).

## State model

```go
type State[S any] struct {
    Data S
    Meta map[string]any
}
```

Typed struct per workflow plus a metadata bag for cross-cutting concerns (trace IDs, session tokens, cost budgets).

## MessageStore and context windows

`MessageStore` holds a reference/cursor, not raw bytes. The `Window(ctx, maxTokens)` method returns the most recent messages that fit within the token budget — O(1) RAM regardless of total history size.

```go
type MessageStore interface {
    Append(ctx context.Context, msg Message) error
    Window(ctx context.Context, maxTokens int) ([]Message, error)
    WindowFor(ctx context.Context, p LLMProvider, maxTokens int) (*Request, error)
}
```

`WindowFor` bypasses translation when consecutive steps use the same provider — native context transfer with zero serialisation overhead.

## Recovery model

```go
type RecoveryStrategy string
const (
    ResumeFromCheckpoint RecoveryStrategy = "resume"  // pick up from last saved state
    ReplayFromStart      RecoveryStrategy = "replay"  // re-run from beginning
)
```

`ThreadID` is the horizontal-scale coordination key — any node that loads the same `ThreadID` resumes at the correct checkpoint.
