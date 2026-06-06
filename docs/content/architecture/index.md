+++
title = 'Architecture'
date = '2026-06-03'
draft = false
toc = true
weight = 5
+++

# Architecture

## Package layout

```
bunshin-go/
├── pkg/
│   ├── core/          # Runnable, TypedRunnable[In,Out], State[S], AsRunnable
│   ├── llm/           # LLMProvider interface, ProviderRegistry, Embedder
│   ├── memory/        # MessageStore — in-memory, file, Redis, Postgres, MinIO
│   ├── vector/        # VectorStore — pgvector, in-memory; MemoryIndexer
│   ├── middleware/    # Chain, WithLogging, WithRetry, WithTimeout, auth, …
│   ├── auth/          # Principal, TenantID, FromContext helpers
│   ├── prompt/        # Fragment, PromptTemplate, TemplateStore, PromptCache
│   ├── checkpoint/    # Checkpoint + Journal, optimistic locking, resume/replay
│   ├── eval/          # EvalRunner[In,Out], Evaluator, Dataset (LangSmith + JSONL)
│   ├── chain/         # Sequential step composition Chain[S]
│   ├── graph/         # Graph[S] executor, SubagentNode, routers
│   ├── transport/     # HTTP/gRPC/SSE primitives — server, router, streaming
│   ├── api/           # Versioned REST handlers (/v1/…) — reference impl
│   ├── probe/         # Health/live/ready/pprof/metrics — Checker interface
│   ├── tools/         # Tool interface, ToolRegistry, CodeExecTool
│   ├── mcp/           # MCPClient + MCPServer (stdio + SSE transports)
│   ├── sandbox/       # Pluggable code execution (E2B, Docker, WASM)
│   ├── telemetry/     # WithLangSmith, RunContext, OTEL spans
│   └── testing/       # FaultInjector (ErrorRate + LatencyP50)
├── internal/
│   └── credentials/   # API key injection at provider construction time
├── cmd/bunshin/       # Unified CLI (cobra + viper)
├── api/               # OpenAPI 3.1 specification
├── ts/                # TypeScript frontends (pnpm workspaces)
│   ├── apps/
│   │   ├── llmwiki/         # Memory navigation UI
│   │   └── graph-navigator/ # Visual graph editor + live trace viewer
│   └── packages/
│       └── api-client/      # Generated OpenAPI TS types + openapi-fetch client
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

## ProviderRegistry — key differentiator

bunshin-go supports multiple instances of the same LLM vendor, each with distinct API keys, token budgets, or rate limits. Nodes select providers by `ModelTier` and tags — never by hardcoded instance names. Swapping infrastructure (key rotation, budget reallocation) requires only re-registration, not code changes.

```go
registry.Register("openai-high-budget", openai.New(openai.WithAPIKey(highKey)),
    llm.Tags{"vendor": "openai", "tier": "smart", "budget": "high"})
registry.Register("openai-low-budget", openai.New(openai.WithAPIKey(lowKey)),
    llm.Tags{"vendor": "openai", "tier": "fast", "budget": "low"})

// Node selects by tier + tag — no hardcoded instance name
llmNode.WithModelTier(llm.Smart, llm.Tag("budget", "high"))
```

Per-tenant billing isolation is achieved by registering tenant-specific provider instances tagged with tenant identifiers.

## Credential injection

API keys are injected at provider construction time. Each `LLMProvider` instance holds its own key. Multiple instances of the same vendor coexist in the `ProviderRegistry` with different keys.

```go
openai.New(openai.WithAPIKey(os.Getenv("OPENAI_API_KEY_HIGH")))
```

## REST API

All application endpoints carry a `/v1` prefix. Probe and metrics endpoints have no version prefix.

```
POST   /v1/workflows/{id}         — synchronous execution
GET    /v1/workflows/{id}/stream  — SSE streaming (LLM tokens + step events)
GET    /v1/threads                — list conversation threads
GET    /v1/threads/{id}/messages  — thread message history
POST   /v1/prompts/{name}/activate — activate a prompt version
POST   /v1/prompts/refresh        — force cache refresh on this node

GET    /healthz                   — liveness probe
GET    /readyz                    — readiness probe (runs Checker chain)
GET    /metrics                   — Prometheus metrics
GET    /debug/pprof/*             — Go pprof profiling
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

## CLI

```
bunshin serve                       start HTTP server
bunshin healthz <addr>              check health/readiness of remote node
bunshin pprof   <addr>              fetch + open pprof from remote node
bunshin version                     print build info

bunshin workflow list|show|create|update|delete|run
bunshin prompt   list|show|create|edit|activate|delete|refresh|run
bunshin eval     list|show|create|update|delete|run
bunshin thread   list|show|delete|export
bunshin memory   list|show|append|delete
bunshin vector   list|search|upsert|delete
bunshin embed    create <text|file>  — embed text, upsert into VectorStore
```

`bunshin prompt run <id>` executes a prompt as an agent loop.
`bunshin workflow run <id>` runs any registered Graph or Chain.
`bunshin embed create` is the primary tool for initialising a VectorStore from a corpus.

## Recovery model

```go
type RecoveryStrategy string
const (
    ResumeFromCheckpoint RecoveryStrategy = "resume"  // pick up from last saved state
    ReplayFromStart      RecoveryStrategy = "replay"  // re-run from beginning
)
```

`ThreadID` is the horizontal-scale coordination key — any node that loads the same `ThreadID` resumes at the correct checkpoint.
