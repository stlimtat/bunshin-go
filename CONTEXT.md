# bunshin-go

A Go library for building production LLM pipelines and agentic workflows. Clones LangChain/LangGraph/LangSmith concepts with Go's type safety, native concurrency, and single-binary deploys.

## Language

### Execution model

**Runnable**:
The core unit of work. Accepts input and produces output. Every chain step, LLM call, tool, and prompt renderer is a Runnable.
_Avoid_: Task, handler, step (when referring to the unit itself)

**TypedRunnable**:
The type-safe inner variant of Runnable parameterised as `TypedRunnable[In, Out]`. All domain logic is expressed here; wrap with `AsRunnable` or `Chain.AsRunnable()` at composition seams.
_Avoid_: Generic runnable, typed handler

**Chain**:
A `Chain[S]` executes steps sequentially, threading `State[S]` from one step to the next. Implements `TypedRunnable[State[S], State[S]]`.
_Avoid_: Pipeline, sequence

**Graph**:
A `Graph[S]` executes a directed graph of nodes, where routers decide the next node. Supports cycles for agent loops. Implements `TypedRunnable[State[S], State[S]]`.
_Avoid_: DAG (it can have cycles at runtime), workflow (too generic)

**Node**:
One vertex in a Graph. Holds a `TypedRunnable[State[S], State[S]]` and an optional Router.
_Avoid_: Step (reserved for Chain), vertex

**Router**:
A function `Router[S]` that inspects `State[S]` and returns the next node ID. Returning `END` terminates the graph.
_Avoid_: Edge, transition, selector

### State

**State**:
`State[S]{ Data S; Meta map[string]any }` — the envelope passed between all Runnables. `Data` holds the typed workflow payload; `Meta` carries cross-cutting concerns (trace IDs, cost budgets). Every inter-step value must be wrapped in State; raw types do not flow between steps.
_Avoid_: Context (overloaded with `context.Context`), payload (incomplete — misses Meta)

**Meta**:
The `map[string]any` bag inside State for cross-cutting data that should flow transparently through a pipeline without polluting the domain struct. Keys are namespaced: `"bunshin.*"`.
_Avoid_: Metadata map, extra fields, side-channel

### LLM

**LLMProvider**:
The interface every model adapter implements. Provides `Complete`, `StreamComplete`, `CanTransferContext`, and `NativeMessages`.
_Avoid_: Model, adapter, client (LLMProvider is the canonical term)

**ProviderID**:
A string identifier for a specific registered `LLMProvider` instance (e.g., `"openai-high-budget"`, `"openai-low-budget"`). Not a vendor name. Multiple instances of the same vendor are registered under distinct `ProviderID`s. See `VendorID` for the vendor type.
_Avoid_: Provider name (ambiguous — could mean vendor or instance)

**VendorID**:
A string identifying the LLM vendor (`"openai"`, `"anthropic"`, `"google"`, `"ollama"`). Carried as a tag on registered providers. Distinct from `ProviderID` which identifies the instance.
_Avoid_: ProviderID (that is the instance name, not the vendor)

**ModelTier**:
A capability classification: `fast`, `smart`, or `reasoning`. Used as the primary selector when resolving a provider from the `ProviderRegistry`. Narrowed by tags.
_Avoid_: Model size, tier level

**ProviderRegistry**:
**Core capability and key differentiator of bunshin-go.** Holds named `LLMProvider` instances tagged with `VendorID`, `ModelTier`, and arbitrary key-value tags (e.g., `"budget": "high"`, `"region": "us-east"`). Workflow nodes select providers by tier + tags — never by hardcoded instance name. Enables multiple instances of the same vendor with different API keys, token budgets, or rate limits, switchable at registration time without code changes. Per-tenant billing isolation is achieved by registering tenant-specific provider instances with matching tags.
_Avoid_: Provider map, model registry, adapter registry

**ContentPart**:
One element of a multi-modal message. Has a `Type` and one active field group: `Text`, `Media`, `ToolCall`, or `ToolResult`.
_Avoid_: Message part, content block

**MediaRef**:
The transport descriptor inside a `ContentPart` for image, audio, video, and document content. Exactly one of `URL` (remote reference) or `Data` (inline bytes) is set.
_Avoid_: Media payload, binary part, inline data

### Memory

**MessageStore**:
Holds conversation history as a cursor/reference rather than materialising all messages. Returns sliding windows to keep `State[S]` small even for 2M-token contexts.
_Avoid_: History store, conversation buffer

**Window**:
The most-recent N tokens of messages returned by `MessageStore.Window`. Not a fixed count of messages.
_Avoid_: Context window (that's the LLM's limit), sliding window (acceptable but verbose)

### Workflow (declarative)

**WorkflowSpec**:
A declarative YAML definition of a graph workflow, compiled at load time to a `Graph[core.State[map[string]any]]`. Has a `name`, an ordered list of `nodes`, and optional `router:` per node. Linear pipelines omit `router:` and `next:` — the compiler auto-wires each node to the next in list order; the last node routes to `END`. Cyclic graphs (agent loops) declare `router:` explicitly. Single execution model — there is no `Chain` vs `Graph` distinction in YAML.
_Avoid_: WorkflowDefinition, Pipeline, FlowSpec

**WorkflowNode (YAML)**:
One vertex in a `WorkflowSpec`. Has an `id`, a `runnable:` (tagged union of `type: llm | tool | custom`), and optional `router:` (for cycles or branching). `input_key` / `output_key` fields move data between `State.Data` (`map[string]any`) and the node's I/O. The `custom` type is the escape hatch — references a Go-registered `Runnable` by name in `RunnableRegistry`.
_Avoid_: Step (reserved for `Chain`), block, action

**workflow.Registries**:
Compile-time dependency struct passed to `workflow.Compile`. Fields: `LLM *llm.ProviderRegistry`, `Tools *tools.ToolRegistry`, `Custom *workflow.RunnableRegistry`, `Prompts prompt.TemplateStore`. Workflows reference registry entries by name/tier; the compiler resolves at compile time, surfacing missing refs as compile errors, not runtime panics.
_Avoid_: Container, Dependencies, ResolverContext

**workflow.Store**:
Interface for persisting `WorkflowSpec` artifacts with `draft → active` versioning (same lifecycle as `Fragment`). Methods: `Create`, `Get` (active), `GetVersion`, `List`, `ListVersions`, `Activate`, `Delete` (soft). Version string is `"sha256:" + hex16(canonical_yaml)` — identical content yields identical version across all backends, making `Create` idempotent and migrations safe. Backends in `pkg/workflow/store/{memory,postgres,blob,git}`; each is a sub-package so callers import only the deps they need.
_Avoid_: WorkflowRepository, SpecStore

**RunnableRegistry**:
A name → `core.Runnable` map used by YAML `custom` nodes to invoke Go-defined Runnables. Lives in `pkg/workflow`; populated at process start by application code; read-only at runtime.
_Avoid_: NodeRegistry, CustomRegistry

### Routers (EIP catalog)

**RouterRegistry**:
A name → `Router[map[string]any]` factory map. YAML `router: { type: X, config: ... }` looks up `X` here. Application code registers EIP routers at startup. Bunshin ships a v1 catalog modelled on Apache Camel's Enterprise Integration Patterns.
_Avoid_: EIPRegistry, RouterCatalog

**Content-Based Router**:
`router.type: content_based`. Reads a key from `State.Meta` and matches the value against a `cases` map. Most common router for agent loops — the LLM node writes `bunshin.next_action` into Meta; the router branches.
_Avoid_: switch (too generic), conditional

**Message Filter**:
`router.type: filter`. Evaluates a predicate (`condition: { key, op, value }`); if false, routes to `END`, skipping all downstream nodes. Used for cache-hit early-exit, guard clauses.
_Avoid_: skip, guard

**Routing Slip**:
`router.type: routing_slip`. Reads an ordered list of node IDs from `State.Meta[slip_key]` and routes through them in order. Enables "LLM emits a plan, executor runs it" pattern.
_Avoid_: plan_router, sequence

**Recipient List**:
`router.type: recipient_list`. Fans out to multiple named nodes; each receives the same input. Outputs collected via `Aggregator` (paired). Enables parallel LLM calls (ensemble, multi-perspective summarisation).
_Avoid_: multicast, fanout

**Splitter**:
`router.type: splitter`. Reads a slice from `State.Data[input_key]`, runs the next node once per item with `State.Data[item_key]` set to each item. Used for chunk-and-process flows (long documents).
_Avoid_: forEach, iterator

**Aggregator**:
`router.type: aggregator`. Collects N outputs from a Recipient List or Splitter into a single slice at `State.Data[output_key]`. Always paired with one of those upstream.
_Avoid_: gather, reducer

**Custom Router**:
`router.type: custom`. Looks up a named entry in the application-provided portion of `RouterRegistry`. The escape hatch for routing logic that can't be expressed declaratively.
_Avoid_: user_router, function_router

### Multi-agent

**SubagentNode**:
A typed adapter `SubagentNode[Outer, Inner]` that runs a `Graph[Inner]` as a node inside a `Graph[Outer]`. `extract` maps outer state to the subagent's input; `merge` writes the subagent's output back into the outer state. Meta flows across both boundaries automatically.
_Avoid_: Nested graph, inner graph, child agent

**Orchestrator**:
A `Graph[OrchestratorState]` whose Router decides which subagent to invoke. Contains no LLM calls itself — it delegates to specialist subagents.
_Avoid_: Supervisor (acceptable but implies hierarchy; orchestrator is more precise), manager agent

**Agent**:
A declarative subagent defined by an `AgentSpec` and compiled to a `CompiledAgent`. Carries its own system prompt (a `Fragment` referenced by slug), tool allowlist, model selector, and iteration cap. Runs an isolated tool-calling loop in a fresh context and returns a result — the caller never sees the agent's internal turns. The systematic, declarative replacement for hand-written `SubagentNode`.
_Avoid_: Subagent (use Agent; "subagent" describes the role, not the type), Assistant, Bot

**AgentSpec**:
A declarative YAML definition of an `Agent`. Fields: `name`, `description`, `system_prompt` (a `FragmentRef`), `tools` (allowlist into `ToolRegistry`), `agents` / `skills` (allowlists resolved against `agent.Store` / `skill.Store`), `model` (`{tier, tags}` into `ProviderRegistry`), `max_iterations` (default 8), optional `input_schema` / `output_schema`. Same `draft → active` lifecycle, content-hash versioning, and tenant-scoped `agent.Store` as `WorkflowSpec`. Resolved eagerly and topologically at compile time — missing refs and reference cycles are compile errors.
_Avoid_: AgentDefinition, Persona, AgentConfig

**CompiledAgent**:
The result of `agent.Compile(spec, registries)`. A `Graph[AgentState]` (`llm → [content-based router] → tools → llm … → END`) that implements **both** `core.Runnable` (top-level invoke + YAML graph node) and `tools.Tool` (agent-as-tool delegation). One compile, three invocation surfaces. Enforces a runtime delegation-depth cap via `Meta["bunshin.agent_depth"]`; exceeding `max_iterations` returns the last message with `Meta["bunshin.agent_truncated"] = true` rather than erroring.
_Avoid_: AgentRunner, AgentInstance

**AgentState**:
The isolated state a `CompiledAgent` runs on. Holds the task string, structured args, and the agent's own message list. Created fresh per invocation; `Meta` (trace IDs, cost budget, tenant, depth) flows in from the caller, but messages never flow back out — only the final result does.
_Avoid_: SubState, LoopState

### Skills

**Skill**:
A named, reusable capability injected into a prompt on demand — instructions plus optional bundled files. Has **no own loop and no own model** (that distinguishes it from an `Agent`); it contributes context to a *consuming* `Agent` or `PromptComposer` and borrows that consumer's sandbox to run any scripts. Defined by a `SkillSpec`.
_Avoid_: Plugin, Capability (too generic), Ability

**SkillSpec**:
A declarative definition of a `Skill`. Fields: `name`, `description`, `body` (a `FragmentRef` — the instructions), `files` (`[]FileRef`, stored as `MediaRef`), and `trigger` (`model` or `condition`). Own `draft → active` lifecycle, content-hash versioning, tenant-scoped `skill.Store` — files version atomically with the spec. The skill *body* is a `Fragment` in `PromptBackend`.
_Avoid_: SkillDefinition, SkillConfig

**Progressive disclosure**:
The skill loading mechanism for `trigger: model` skills. The skill's `name` + `description` are always cheaply in context (advertised as a synthetic `load_skill_<name>` tool); the full body and file manifest inject only when the model calls the load-tool. `trigger: condition` skills bypass this — the `PromptComposer` evaluates a `FragmentRef` condition at render time and injects matching bodies deterministically, no LLM choice.
_Avoid_: Lazy loading, on-demand injection

**FileRef**:
A bundled file in a `SkillSpec`, stored as a `MediaRef` (inline if `< inline_max_bytes`, else MinIO/S3 URL). Reference docs (text/markdown) inject into context on skill load; executable scripts mount into the consuming `Agent`'s sandbox `Session` for its `CodeExecTool` to run. A skill never executes anything itself. With no sandbox available (bare LLM call), scripts are listed but unavailable and the composer sets `Meta["bunshin.skill_scripts_skipped"]`.
_Avoid_: Attachment, Resource, Asset

### Prompt

**Fragment**:
A named, reusable `text/template` partial stored in a `PromptBackend`. Has two identifiers: `ID` (UUID, immutable — survives slug renames; UUIDv4 for PostgresStore, UUIDv5 slug-derived for file-backed stores) and `Slug` (human-readable, tenant-unique, mutable — e.g. `"investigation-report-fragment"`, `"analysis"`). Runtime template resolution uses `Slug`; YAML workflow nodes write `prompt: investigation-report-fragment`. HTTP management operations (`DELETE`, rename, activate) use UUID for stability. Renaming a slug invalidates YAML files referencing the old slug. Each (tenant, slug) has a `draft`→`active` lifecycle. Roll-forward only.
_Avoid_: Partial, snippet, include; do not conflate `ID` (UUID) with `Slug` (human name); do not use UUID in YAML `prompt:` references

**PromptTemplate**:
A `text/template` that composes one or more `Fragment`s into a complete prompt. Rendered at invocation time with a data struct. Output is a `[]llm.Message` ready for an `LLMProvider`.
_Avoid_: Template (too generic), prompt string

**PromptBackend**:
Interface for storing and retrieving `Fragment`s. All methods take explicit `tenantID` — one instance serves all tenants (mirrors `workflow.Store`). Key methods: `Put(ctx, tenantID, *Fragment)`, `Get(ctx, tenantID, slug)` (runtime resolution by slug), `GetByID(ctx, tenantID, uuid)` (management plane), `GetVersion(ctx, tenantID, slug, version)`, `List(ctx, tenantID, tags...)`, `Rename(ctx, tenantID, uuid, newSlug)`, `Watch(ctx, tenantID, slug)`. Backends: `MemoryBackend` (tests), `EmbedStore` (embed.FS, read-only), `FSStore` (os.DirFS, hot-reload, read-only for writes), `PostgresStore` (versioned, promotable, source of truth in production). `Rename` and `Promote` return `ErrNotSupported` on read-only backends.
_Avoid_: TemplateStore (old name), PromptStore, template backend

**PromptCache**:
Two-level runtime cache. Redis layer: shared across the cluster, holds raw template strings keyed `{tenant_id}:prompt:{name}`. In-process layer: compiled `*template.Template` per `(tenant_id, name, version)`, held in an `atomic.Pointer[templateSnapshot]`. Background poll (default 5s) detects Redis version pointer changes and invalidates the in-process layer. Manual refresh via `POST /v1/prompts/refresh` or `bunshin prompt refresh` forces one node to re-fetch from Postgres into Redis; cluster converges within one poll cycle.
_Avoid_: Template cache, prompt registry

### Checkpointing (extended)

**Optimistic locking**:
Checkpoint rows carry a `version int`. A writer reads version N and issues `UPDATE WHERE version = N`, incrementing to N+1. Zero rows affected means another worker already wrote N+1 — conflict detected at write time. Preferred over pessimistic advisory locks because agent loops hold state across long LLM calls; holding a DB lock for seconds creates contention.
_Avoid_: Pessimistic locking, advisory lock (both rejected)

### MCP

**MCPClient**:
Connects to an external MCP server (stdio subprocess or SSE/HTTP), discovers its tools, and registers them in the local `ToolRegistry`. Enables consuming the MCP ecosystem without writing Go adapters per tool.
_Avoid_: MCP adapter, external tool bridge

**MCPServer**:
Exposes bunshin's `ToolRegistry` as an MCP server over stdio or SSE. External MCP clients (Claude Desktop, other agents) discover and call bunshin tools via the MCP protocol.
_Avoid_: Tool server, MCP host

### Telemetry

**RunContext**:
A trace handle written into `context.Context` by `WithLangSmith` middleware. Nodes that perform LLM calls or tool calls read `RunContext` from context and post child runs to LangSmith. Enables per-node trace trees rather than one blob per workflow invocation.
_Avoid_: TraceContext, SpanContext (reserved for OTEL), trace handle

**WithLangSmith**:
Middleware that creates a root `RunContext`, writes it to `context.Context`, invokes the wrapped `Runnable`, then closes the root run with output and error. Child runs are posted by instrumented nodes directly. OTEL span is also started here for infrastructure metrics.
_Avoid_: LangSmith middleware, tracing middleware

### Eval

**EvalRunner**:
`EvalRunner[In, Out any]` in `pkg/eval`. Runs a `TypedRunnable[In, Out]` against a dataset, collecting outputs and passing them to evaluators. Type parameters are fixed at construction — schema drift causes a compile error, not a runtime panic.
_Avoid_: TestRunner, BenchmarkRunner

**Evaluator**:
`Evaluator[Out any]` — a function that scores one `(input In, output Out, expected Out)` triple and returns `EvalScore{ Name string; Score float64; Pass bool }`. Multiple evaluators run per row.
_Avoid_: Scorer, judge, metric

**Dataset**:
Ordered slice of `EvalCase[In, Out]{ Input In; Expected Out; ID string }`. Loaded from LangSmith or local JSONL via `eval.LoadJSONL[In, Out]`. LangSmith is primary; JSONL is offline fallback.
_Avoid_: Test set, benchmark set, eval set

### Vector

**VectorStore**:
Interface in `pkg/vector` for semantic search. Operations: `Upsert(ctx, []Document)`, `Search(ctx, query []float32, topK int, filter map[string]any)`, `Delete(ctx, ids []string)`. Filter maps to SQL `WHERE` on metadata — required for per-thread scoping. Shares a `*pgxpool.Pool` with `pkg/memory` Postgres backends but is a separate interface.
_Avoid_: EmbeddingStore, SemanticStore

**Document**:
The unit stored in a `VectorStore`. Fields: `ID string`, `Content string`, `Vector []float32`, `Metadata map[string]any`. `Vector` is populated by an `Embedder` before upsert. `Metadata` drives filter queries.
_Avoid_: Embedding, chunk, record

**SearchResult**:
Return type from `VectorStore.Search`. Wraps a `Document` with a `Score float32` (cosine similarity).
_Avoid_: Hit, match, result

### Sandbox

**Sandbox**:
Interface in `pkg/sandbox` for executing untrusted code. Two methods: `Run(ctx, RunRequest)` (ephemeral — open, execute, close atomically) and `Session(ctx)` (persistent — multiple `Run` calls share filesystem and installed packages). Three backends: E2B (cloud VM), Docker (local container), WASM (in-process).
_Avoid_: Executor, runner, code evaluator

**Session**:
A persistent sandbox execution environment returned by `Sandbox.Session`. Multiple `Run` calls share state — installed packages, written files, environment variables. Stored in `State.Meta["bunshin.sandbox_session"]` across tool calls within one graph invocation. Closed by middleware finaliser when invocation ends.
_Avoid_: SandboxContext, execution context (overloaded with context.Context)

**SandboxRegistry**:
Holds named `Sandbox` instances tagged with selection criteria and resource limits (e.g., `"language":"python"`, `"memory_mb":"512"`, `"tenant_tier":"high"`). Tags serve dual purpose: backend selection at call time AND resource configuration at registration time. Same pattern as `ProviderRegistry`. Resource limits are immutable per registry entry — callers cannot exceed their tier's limits.
_Avoid_: Sandbox pool, executor registry

**RunRequest**:
Input to `Sandbox.Run` or `Session.Run`. Fields: `Language`, `Code`, `Timeout`, `Stdin`, `Files map[string][]byte` (inject files), `EnvVars`. No resource limit fields — limits are fixed by the registry entry selected.
_Avoid_: ExecutionRequest, CodeRequest

**RunResult**:
Output from `Sandbox.Run`. Fields: `Stdout`, `Stderr`, `ExitCode`, `Duration`, `Files map[string]*llm.MediaRef`. `Files` uses `MediaRef` so outputs slot directly into `ContentPart` for the next LLM call. Small files returned as inline `io.Reader`; large files uploaded to MinIO/S3 by the backend and returned as a remote URL ref. Threshold configurable per registry entry via `"inline_max_bytes"` tag (default 10 MB).
_Avoid_: ExecutionResult, CodeOutput

### Tools

**CodeExecTool**:
A `Tool` implementation in `pkg/tools` that wraps a `pkg/sandbox.Sandbox`. Thin adapter — 20 lines. `Execute` delegates to `Sandbox.Run`. The sandbox backend (E2B, Docker, WASM) is injected at construction. `Sandbox` and `Tool` are orthogonal interfaces; `CodeExecTool` bridges them.
_Avoid_: SandboxTool, ExecutorTool

### TypeScript

**ts/**:
Root of all TypeScript applications. Uses pnpm workspaces. Structure: `ts/apps/llmwiki`, `ts/apps/graph-navigator`, `ts/packages/api-client` (generated OpenAPI TS types + fetch client).
_Avoid_: frontend directory, web directory

**api-client**:
Shared pnpm workspace package at `ts/packages/api-client`. Generated from the OpenAPI 3.1 spec in `api/` using `openapi-typescript` (types) and `openapi-fetch` (runtime client). Both TS apps import from this package.
_Avoid_: SDK, client library, generated types

### Transport

**pkg/transport**:
HTTP/gRPC/SSE transport primitives — server construction, router, streaming helpers. The library layer. `pkg/api` is the reference implementation built on top of it.
_Avoid_: Server package, HTTP layer

**pkg/api**:
Versioned REST API handlers (`/v1/...`) built on `pkg/transport`. Serves as the reference implementation and example of how to use `pkg/transport`. All endpoints carry a `/v1` prefix except probe and metrics.
_Avoid_: Routes package, HTTP handlers package

**pkg/probe**:
Health, liveness, readiness, pprof, and Prometheus metrics endpoints. Users register custom checks via the `Checker` interface. No API version prefix on these endpoints (`/healthz`, `/readyz`, `/metrics`, `/debug/pprof`).
_Avoid_: healthz package, health package

**Checker**:
`Checker` interface with `Check(ctx context.Context) error`. Registered per-probe type (liveness or readiness) with a name. A failing check causes the probe endpoint to return non-200.
_Avoid_: HealthCheck, Probe function

### CLI

**bunshin CLI**:
Unified CLI entry point at `cmd/bunshin`. Built with cobra + viper. Command groups mirror the package architecture. `bunshin chain` and `bunshin agent` are removed — replaced by `bunshin workflow run` and `bunshin prompt run` respectively.

Top-level commands:
- `serve` — start HTTP server
- `healthz <addr>` — check health/readiness of a remote node
- `pprof <addr>` — fetch and open pprof from a remote node
- `version` — print build info
- `workflow` — CRUD + run (list, show, create, update, delete, run)
- `prompt` — CRUD + lifecycle + run (list, show, create, edit, activate, delete, refresh, run)
- `eval` — CRUD + run (list, show, create, update, delete, run)
- `thread` — conversation thread management (list, show, delete, export)
- `memory` — MessageStore CRUD (list, show, append, delete)
- `vector` — VectorStore CRUD + search (list, search, upsert, delete)
- `embed` — produce embeddings from text or file, upsert into VectorStore (used to initialize datastores)

_Avoid_: `agent` command (use `prompt run`), `chain` command (use `workflow run`)

### Testing

**FaultInjector**:
Middleware in `pkg/testing/fault` that wraps a `Runnable` and injects errors and latency on a configurable schedule. Used for HA testing — validates circuit breakers, retries, and timeout handling. Two fault types: `fault.ErrorRate(probability, err)` and `fault.LatencyP50(median, max)`, each independently composable via `middleware.Chain`. Latency uses a triangular distribution `[0, max]` peaking at `median` so the `median` parameter is the true P50. Partial-stream injection is out of scope.
_Avoid_: ChaosMiddleware, failure injector

### Multi-tenancy

**TenantID**:
A string identifier for an organisation in a multi-tenant deployment. Carried on `auth.Principal`. All stores (`MessageStore`, `VectorStore`, `TemplateStore`, `Checkpoint`) scope queries by `TenantID` at the store layer — not at call sites. Redis cache keys are prefixed `{tenant_id}:`.
_Avoid_: OrgID (acceptable alias; TenantID is canonical), namespace

**Logical isolation**:
Tenant data separated by `tenant_id` column on shared infrastructure (Postgres tables, Redis keyspace). One DB pool, one Redis instance. Store implementations enforce tenant filter — missing it is a bug in the store, not the caller.
_Avoid_: Schema isolation, DB-per-tenant (both rejected)

### Auth

**Principal**:
`Principal{ Subject string; TenantID string; Roles []string; Claims map[string]any }` — the authenticated identity written to `context.Context` by auth middleware. `Subject` is the user ID or API key fingerprint. `TenantID` scopes all store operations to the tenant's data. `Roles` drives role-predicate checks. `Claims` holds raw JWT claims.
_Avoid_: User, caller, identity (Principal is the canonical term)

**WithRBAC**:
Middleware that reads `Principal` from context and calls a `func(Principal) bool` predicate. Returns 403 if the predicate returns false. Role resolution is the caller's responsibility — bunshin does not maintain a role store.
_Avoid_: Authorizer, access control layer

### Checkpointing

**Checkpoint**:
A snapshot of `State[S]` at a point in time, keyed by `ThreadID`. Used for resume-from-failure recovery.
_Avoid_: Snapshot, savepoint

**ThreadID**:
The horizontal-scale coordination key. Any node that loads the same `ThreadID` resumes the same workflow, regardless of which process handles it.
_Avoid_: Session ID (use `MetaSessionID` for session; ThreadID is specifically for checkpoint coordination)

**Journal**:
An append-only log of `JournalEntry` records for a `ThreadID`. Used for replay-from-start recovery when a Checkpoint is unavailable or corrupt.
_Avoid_: Event log, audit log
