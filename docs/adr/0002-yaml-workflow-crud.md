# YAML workflow definitions with multi-backend CRUD

Workflows can be defined declaratively in YAML and CRUDed over HTTP, in addition to the existing path of constructing `Chain[S]` / `Graph[S]` in Go code. The YAML path is intended for production deployments where workflow changes ship without recompiling — e.g. SREs editing prompts and routing logic, GitOps flows, multi-tenant environments where each tenant ships its own workflows.

LangChain (Python) does not ship a first-class YAML workflow format. LCEL is code-only; LangChain Hub stores prompts (not workflows); LangGraph is code-only. The closest analogues are Argo Workflows, GitHub Actions, and Apache Beam YAML. Shipping CRUD for YAML workflows is a deliberate differentiator.

## Architecture

- **State type**: YAML workflows compile to `Graph[core.State[map[string]any]]`. YAML cannot name a Go type; the dynamic-map state is the typed-Go escape hatch. Runtime errors when a node reads a missing key are accepted as the cost of dynamic typing.
- **Schema**: A single `kind: graph` model. Linear pipelines omit `router:` and `next:`; the compiler auto-wires position-based edges and routes the last node to `END`. Cyclic graphs (agent loops) declare routers explicitly. No separate `chain` kind — Chain is just Graph with implicit linear routing.
- **Package layout**: `pkg/workflow` (parser, compiler, interfaces, `RunnableRegistry`) plus `pkg/workflow/store/{memory,postgres,blob,git}` sub-packages so callers import only the backend deps they need (pgx, gocloud.dev/blob, go-git).
- **Compile-time dependencies**: `workflow.Registries{LLM, Tools, Custom, Prompts}` struct passed to `workflow.Compile`. Missing refs surface as compile errors, not runtime panics.
- **Versioning**: `draft → active` lifecycle, roll-forward only. Same model as `Fragment` (per ADR-0000 prompt versioning). Version string is `"sha256:" + hex16(canonical_yaml)` so identical content yields identical version across all backends — `Create` is idempotent, migrations preserve identity, corruption is detectable.

## Node types (v1)

| Type | Purpose |
|---|---|
| `llm` | Render a prompt template, call an LLMProvider, write result to `output_key` |
| `tool` | Invoke a registered `Tool` with `input_key`, write result to `output_key` |
| `custom` | Invoke a Go-registered `Runnable` by name — escape hatch |

`prompt` (render-only) and `subgraph` (nested workflow) are deferred to v2.

## HTTP API

Standard REST surface under `/v1/workflows`:

```
POST   /v1/workflows                          create draft, returns version
GET    /v1/workflows                          list workflow names
GET    /v1/workflows/{name}                   get active spec
GET    /v1/workflows/{name}/versions          list versions
GET    /v1/workflows/{name}/versions/{ver}    get specific version
POST   /v1/workflows/{name}/activate          promote version → active
DELETE /v1/workflows/{name}                   soft delete
POST   /v1/workflows/{name}/invoke            execute (synchronous)
GET    /v1/workflows/{name}/stream            execute (SSE)
```

The existing `POST /v1/workflows/{id}` route is renamed to `POST /v1/workflows/{name}/invoke` to free the bare `POST /v1/workflows` for create.

## Consequences

- The YAML schema is part of the public API. Adding a new node type is backward-compatible; renaming a field or changing semantics is a breaking change.
- The content-hash version format means re-creating a spec with whitespace changes (after YAML canonicalisation) produces the same version — intentional.
- `pkg/workflow/store/git` adds a `go-git` dependency only when imported; the bare `pkg/workflow` import stays light.
- Sub-package store layout means application code must import the chosen backend explicitly; there is no central `NewStore(kind)` factory.
- Multi-backend `Create` idempotency depends on canonical YAML serialisation; the canoniser is part of the public API and must be stable across versions.
- The rename of `POST /v1/workflows/{id}` is a one-shot breaking change; current state has one internal test and no external consumers, so cost is contained.
