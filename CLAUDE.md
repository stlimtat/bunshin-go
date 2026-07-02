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

# Lint (zero issues expected)
golangci-lint run

# Single package test with verbose
go test ./pkg/core/... -v -race
```

## Architecture

- `pkg/core`       — Runnable, TypedRunnable[In,Out], State[S], AsRunnable adapter
- `pkg/llm`        — LLMProvider interface + adapters (OpenAI, Anthropic, Google, Azure, Ollama), ProviderRegistry
- `pkg/memory`     — MessageStore interface + VFS backends (memory, file, Redis, S3)
- `pkg/middleware` — Middleware chain, all 5 interception levels
- `pkg/prompt`     — Fragment, PromptTemplate, pluggable backends, two-level cache
- `pkg/tools`      — Tool interface, ToolRegistry, built-in tools
- `pkg/mcp`        — MCP client + server (Model Context Protocol)
- `pkg/sandbox`    — Sandboxed code execution (E2B, Docker, WASM)
- `pkg/telemetry`  — LangSmith (LLM traces) + OTEL (infrastructure)
- `pkg/chain`      — Sequential chain composition
- `pkg/graph`      — DAG workflow executor, cyclic routing for agent loops
- `pkg/agent`      — AgentSpec → CompiledAgent compiler, subagent delegation
- `pkg/workflow`   — YAML workflow Spec → Runnable compiler, versioned stores
- `pkg/skill`      — Declarative skill specs, versioned stores
- `pkg/checkpoint` — Checkpoint/Journal, resume + replay recovery
- `pkg/eval`       — EvalRunner, evaluators, LangSmith dataset sync
- `pkg/vector`     — VectorStore + Indexer for semantic search
- `pkg/transport`  — gRPC/HTTP2, SSE, streaming transports
- `pkg/testing`    — FaultInjector middleware for HA testing
- `pkg/api`        — HTTP handlers for workflows/prompts/threads
- `cmd/bunshin`    — CLI (cobra + viper)
- `examples/`      — llm, workflow, agent, mcp-sandbox, concurrent/*, job-search
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

## Code style

### Errors

- Wrap with context, `%q` for identifiers, `%w` for the cause:
  `fmt.Errorf("workflow: node %q: %w", ns.ID, err)`
- Prefix errors with the package or type name so a log line locates the source:
  `"blob.Store: write %q: %w"`, `"agent %q: resolve tools: %w"`
- Sentinel errors are package-level `var ErrX = errors.New(...)`; compare with
  `errors.Is`, never `==` (including `io.EOF` from iterators)
- Errors are returned, never carried inside State; the executor decides
  halt/retry/route

### Panics

Only for programmer errors that can never be input-dependent: nil function to
a constructor, duplicate graph node ID. Everything reachable from user input
or config returns an error.

### Constructors and options

- `NewX(...)` returns `*X`, fully usable with zero further setup
- Optional knobs via chainable `WithX` methods returning the receiver
  (`registry.WithLogger(l).WithPingInterval(d)`), not functional options
- Config structs (`OpenAIConfig`) for adapters with several required fields

### Concurrency

- `mu.Lock(); defer mu.Unlock()` — no manual unlock paths
- Read-heavy shared state: `atomic.Pointer[snapshot]` swapped whole, never
  mutated in place (see `prompt.PromptCache`)
- Every spawned goroutine has an owner and a stop signal (`stopCh`,
  `ctx.Done()`, or `sync.Once`-guarded Start)
- Channels: producer closes (`defer close(ch)`); single-result channels are
  buffer=1 so the send never blocks; every send in a loop selects on
  `ctx.Done()`
- Fan-out uses `golang.org/x/sync/errgroup`, not raw WaitGroups

### Interfaces

- Accept interfaces, return structs
- Define the interface where it is consumed, keep it minimal (1–4 methods)
- Optional capabilities are separate interfaces discovered by type assertion
  (`telemetry.Pinger`, `TypedStreamRunnable`) — never fatten the base interface

### General

- `ctx context.Context` is always the first parameter; `tenantID` immediately
  after for multi-tenant methods
- Prefer stdlib over hand-rolled (`strings.Index`, `errors.Is`, `slices`,
  `maps`); prefer no new dependency over a new dependency
- Comments explain WHY (invariants, failure modes, design pressure), not WHAT;
  godoc on exported symbols states behavior AND failure modes
- Deliberate simplifications carry a `ponytail:` comment naming the ceiling
  and the upgrade path
- Close errors: check on writes (`f.Close()` returns flush errors), discard
  explicitly on reads (`defer func() { _ = f.Close() }()`)
- No `init()`; no package-level mutable state except sentinel errors and
  test-swap function vars

## Project layout (golang-standards)

```
pkg/        public library packages
internal/   private shared utilities (credentials, backoff)
examples/   runnable demo programs
api/        OpenAPI 3.1 spec
deployments/ Dockerfile, docker-compose
docs/       Hugo documentation site (published to stlimtat.github.io/bunshin-go)
.github/    CI workflows
```
