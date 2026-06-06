+++
title = 'Roadmap'
date = '2026-06-05'
draft = false
weight = 10
toc = true
+++

# Roadmap

Planned features not yet implemented. Items are grouped by area.

---

## Memory backends

### Postgres MessageStore

Persistent conversation history with full-text search. Schema:

```sql
CREATE TABLE messages (
    thread_id   TEXT        NOT NULL,
    sequence    BIGSERIAL   NOT NULL,
    role        TEXT        NOT NULL,
    parts_json  JSONB       NOT NULL,
    token_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, sequence)
);
CREATE INDEX messages_thread_tokens ON messages (thread_id, sequence DESC, token_count);
```

`Window(ctx, maxTokens)` scans backward from the latest sequence summing `token_count` until the budget is exhausted, then returns the slice. O(window size) not O(total history).

### MinIO / S3 MessageStore

Stores thread history as NDJSON files under `{thread_id}/messages.ndjson`. Uses `gocloud.dev/blob` — works with any S3-compatible object store. Intended for long-term archiving and audit trails; not optimised for low-latency window queries.

---

## LLMWiki — memory navigation frontend

A read-only web UI for browsing and selecting conversation history.

**Primary use case**: context selection. A user or operator browses prior conversations, selects individual messages or entire threads, and injects them as additional context into the next LLM call. Think of it as a conversation memory manager rather than a chat interface.

**Planned capabilities:**

| Feature | Description |
|---------|-------------|
| Thread browser | List all threads with metadata (date, token count, provider) |
| Message viewer | Scroll through a thread with role-colour-coding |
| Context selector | Checkbox on messages → injected into `MessageStore.Window` override |
| Semantic search | Find messages by meaning, not just keyword (requires embedding backend) |
| Export | Download thread as NDJSON or Markdown |

**Frontend**: TypeScript + React (see `ts/llmwiki/`).  
**Backend**: REST endpoints added to `pkg/transport` or a dedicated service.

---

## HTTP authentication middleware

Planned additions to `pkg/middleware` at the L1 (Workflow) level:

| Middleware | Method | Notes |
|-----------|--------|-------|
| `WithAPIKey` | Header `X-API-Key` | Static key validation |
| `WithBearerJWT` | `Authorization: Bearer <token>` | HMAC-SHA256 or RSA validation |
| `WithOIDC` | OIDC discovery URL | Validates against provider (Auth0, Keycloak, Entra) |
| `WithIPAllowlist` | Peer IP | CIDR block check |

All auth middleware operates on `context.Context` and sets a `Principal` value that downstream middleware (e.g., `WithRBAC`) can read.

```go
agent := middleware.Chain(g.AsRunnable(),
    middleware.WithBearerJWT(jwtSecret),
    middleware.WithRBAC(func(principal string) bool {
        return authorizer.CanInvoke(principal, "agent-loop")
    }),
    middleware.WithLogging(logger),
)
```

---

## TypeScript frontends

Two standalone TypeScript applications planned in `ts/`:

### ts/llmwiki

Memory navigation UI (see LLMWiki section above). Stack:
- React 19 + TypeScript
- Vite build
- TanStack Query for data fetching
- shadcn/ui components

### ts/graph-navigator

Visual graph editor and live trace viewer.

**Graph view**: Render the bunshin graph as a node-edge diagram. Nodes are colour-coded by type (LLM, Tool, Router). Edges show routing conditions.

**Trace view**: Watch a workflow run in real-time. SSE stream from `GET /workflows/{id}/stream` drives the UI — nodes light up as they execute, token-by-token output appears in the LLM node panel.

**Troubleshooting**: Click any node in a past run to see its input `State[S]`, output `State[S]`, error (if any), token usage, and latency.

Stack:
- React 19 + TypeScript
- Vite build  
- ReactFlow for graph rendering
- shadcn/ui components
- SSE for live trace streaming

---

## Planned middleware

| Middleware | Level | Purpose |
|-----------|-------|---------|
| `WithMaxIterations` | L1 | Terminate agent loops at N iterations |
| `WithTokenBudget` | L4 | Terminate when cumulative token spend exceeds limit |
| `WithSemanticCache` | L4 | Return cached response for semantically similar prompts |
| `WithPIIScrub` | L3 | Strip PII before sending prompt to provider |
| `WithRateLimit` | L1 | Token bucket rate limiting per caller |
| `WithBearerJWT` | L1 | JWT authentication |
| `WithRBAC` | L1 | Role-based access control |

---

## Planned graph primitives

| Primitive | Purpose |
|-----------|---------|
| `SubagentNode[Outer, Inner]` | Run a `Graph[Inner]` as a node in `Graph[Outer]` |
| `ParallelNode[S]` | Run multiple nodes concurrently, merge results |
| `HumanNode[S]` | Pause execution, emit interrupt event, resume on approval |
| `WithMaxIterations` middleware | Loop guard without modifying state struct |
