# Prompt CRUD HTTP API surface

Supersedes the HTTP API section of ADR 0004.

## Route table

```
PUT    /v1/prompts/{slug}                   — upsert (content update and/or rename)
GET    /v1/prompts                          — list active fragments for tenant
GET    /v1/prompts/{slug}                   — get active version by slug
GET    /v1/prompts/id/{id}                  — get by immutable UUID
GET    /v1/prompts/{slug}/versions          — list all versions, metadata only, newest-first
GET    /v1/prompts/{slug}/versions/{ver}    — get specific version (any status)
POST   /v1/prompts/{slug}/activate          — promote newest draft → active
DELETE /v1/prompts/{slug}                   — soft delete (tombstone) + cache refresh
POST   /v1/prompts/{slug}/purge             — hard delete (501 stub; future op)
POST   /v1/prompts/refresh                  — force in-process cache refresh
```

## Key decisions

### PUT as upsert

`PUT /v1/prompts/{slug}` is the single write surface. First call creates a draft;
subsequent calls create a new draft version. Body fields:

```json
{
  "content": "...",
  "tags": ["..."],
  "version_label": "...",
  "slug": "new-name"
}
```

If `slug` in body differs from `{slug}` in URL, the fragment is renamed first, then
content is updated (if provided). Either field may be omitted — rename-only and
content-update-only are both valid. This collapses `create`, `edit`, and `rename`
into one endpoint.

### Dual URL key (slug + UUID)

Read paths offer both `GET /v1/prompts/{slug}` (human-readable, matches YAML workflow
refs) and `GET /v1/prompts/id/{id}` (UUID, stable across renames). Management-plane
tools that follow a rename use the UUID path; YAML authors use the slug path.

This supersedes ADR 0004's split of "read by slug / write by UUID" — write now uses
slug too (server resolves UUID internally).

### Soft delete + purge

`DELETE /v1/prompts/{slug}` tombstones the fragment (all versions) and triggers a
cache refresh. Active workflows referencing the slug will fail at next cache reload.

`POST /v1/prompts/{slug}/purge` hard-deletes all rows/files. Returns 501 until
implemented. The two-step model lets operators soft-delete immediately and schedule
purge during a maintenance window.

### `DELETE` on `PromptBackend`

`Delete(ctx context.Context, tenantID, slug string) error` added to `PromptBackend`.
Soft-delete semantics (tombstone). `EmbedStore` and `FSStore` return `ErrNotSupported`
(read-only backends). `MemoryBackend` and `PostgresStore` implement it.

### TenantID extraction

All handlers extract `tenantID` from `auth.Principal` via `auth.FromContext(r.Context())`.
No URL segment, no header. The JWT auth middleware is responsible for putting `Principal`
into context before any `/v1/prompts` handler runs.

### Router wiring

`pkg/api.PromptActivator` interface is removed. `RouterConfig` gains:

```go
PromptBackend  prompt.PromptBackend   // CRUD operations
PromptActivator prompt.PromptActivator // Promote (nil → 501 for file-backed stores)
```

`PromptRefresher` remains — the cache refresh is a separate concern from the store.

### `GET /v1/prompts/{slug}/versions` response

Returns all versions (draft + active) newest-first. Metadata only:

```json
{
  "versions": [
    {"version": "v3", "status": "draft",  "created_at": "..."},
    {"version": "v2", "status": "active", "created_at": "..."},
    {"version": "v1", "status": "draft",  "created_at": "..."}
  ]
}
```

Full content requires `GET /v1/prompts/{slug}/versions/{ver}`.

## Consequences

- `PromptBackend` interface gains `Delete` — breaking change on all five implementations.
- `pkg/api.PromptActivator` interface deleted — callers use `prompt.PromptActivator` directly.
- ADR 0004 HTTP API section (POST /v1/prompts create, PUT /{uuid}/slug rename,
  DELETE /{uuid}) is superseded by this ADR.
- CLI commands `prompt list`, `prompt show`, `prompt create`, `prompt edit`,
  `prompt delete` are implemented as thin wrappers around these routes.
