# bunshin-go

Go clone of LangChain/LangGraph/LangSmith. See plan at ~/.claude/plans/drifting-dazzling-quilt.md.

## Go version

1.25+

## Commands

```bash
# Tests
go test ./... -race -count=1

# Build
go build ./...

# Lint
golangci-lint run

# Single package test with verbose
go test ./pkg/core/... -v -race
```

## Architecture

- `pkg/core`       — Runnable, TypedRunnable[In,Out], State[S], AsRunnable adapter
- `pkg/llm`        — LLMProvider interface + adapters (OpenAI, Anthropic, Google, Ollama)
- `pkg/memory`     — MessageStore interface + VFS backends (memory, file, Redis, S3)
- `pkg/middleware` — Middleware chain, all 5 interception levels
- `pkg/prompt`     — Fragment, PromptTemplate, pluggable backends
- `pkg/tools`      — Tool interface, ToolRegistry, built-in tools
- `pkg/mcp`        — MCP client + server (Model Context Protocol)
- `pkg/sandbox`    — Sandboxed code execution (E2B, Docker, WASM)
- `pkg/telemetry`  — LangSmith (LLM traces) + OTEL (infrastructure)
- `pkg/chain`      — Sequential chain composition
- `pkg/graph`      — DAG workflow executor, agent loop
- `pkg/checkpoint` — Checkpoint/Journal, resume + replay recovery
- `pkg/eval`       — EvalRunner, evaluators, LangSmith dataset sync
- `pkg/transport`  — gRPC/HTTP2, SSE, streaming transports
- `pkg/testing`    — FaultInjector middleware for HA testing
- `cmd/bunshin`    — CLI (cobra + viper)
- `examples/`      — hello-llm, hello-chain, hello-agent, hello-mcp-sandbox
- `internal/credentials` — credential injection (context + Provider interface)

## Key design decisions

- Two-layer Runnable: `TypedRunnable[In,Out]` (type-safe) wrapped by untyped `Runnable` shell
- State: `State[S]{ Data S; Meta map[string]any }` — typed struct + metadata bag
- MessageStore holds reference/cursor, not raw bytes — handles 2M token context
- Middleware at ALL 5 levels: Workflow, Runnable, Prompt, LLM call, Tool
- Same-provider LLM context transfer: bypass translation when consecutive steps use same provider
- ThreadID = horizontal-scale coordination key for checkpoint/resume across nodes
- LangSmith primary telemetry + OTEL for infrastructure metrics (complementary)

## File layout conventions

- Each `.go` source file contains EITHER interfaces OR structs OR functions (single responsibility)
- Fakes/test doubles go in `fake.go` per package
- Package doc comment goes in `interfaces.go`
- Every exported symbol has a godoc comment
- Table-driven tests, `_test.go` fakes for every interface
- TDD: interface → tests → implement
- Commit after each package (≤400 lines diff per commit for stacked PRs)

## Project layout (golang-standards)

```
pkg/        public library packages
internal/   private shared utilities (credentials, backoff)
examples/   runnable demo programs
api/        OpenAPI 3.1 spec
deployments/ Dockerfile, docker-compose
.github/    CI workflows
```
