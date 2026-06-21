# Fragment identity: UUID + mutable slug, multi-tenant PromptBackend

Fragments gain two distinct identifiers: an immutable `ID` (UUIDv4 for PostgresStore, UUIDv5 slug-derived for file-backed stores) and a mutable `Slug` (human-readable, tenant-unique string such as `"investigation-report-fragment"`). Previously `Fragment.ID` served as the slug, which made stable references impossible across renames.

## Why both fields

Runtime template resolution uses `Slug` — `PromptBackend.Get(ctx, tenantID, slug)` — so YAML workflow nodes reference fragments by slug and remain readable. Management-plane operations (`DELETE`, rename, activate) use UUID for stability: if a slug is renamed, existing UUID-based references survive while YAML files referencing the old slug need updating (acceptable; rename is an operator action).

The alternative (UUID-only everywhere) makes YAML unreadable. Slug-only (current state) makes management operations fragile across renames.

## UUID generation

PostgresStore generates UUIDv4 randomly at `Put` time. EmbedStore and FSStore derive UUIDv5 deterministically from `namespace + tenantID + slug`. Deterministic UUIDs mean file-backed stores require no UUID storage — the same slug always yields the same UUID across restarts and binary rebuilds. Renaming a slug in a file-backed store produces a new UUID, which is correct: the fragment has a new identity.

## PromptBackend interface

All methods now take explicit `tenantID` — one store instance serves all tenants (mirrors `workflow.Store`). The previous pattern of baking `tenantID` into the constructor (`NewPostgresStore(pool, tenantID)`) is removed. This is a breaking change on all five backends.

New interface shape:

```go
type PromptBackend interface {
    Put(ctx context.Context, tenantID string, f *Fragment) error
    Get(ctx context.Context, tenantID, slug string) (*Fragment, error)
    GetByID(ctx context.Context, tenantID, uuid string) (*Fragment, error)  // management plane only
    GetVersion(ctx context.Context, tenantID, slug, version string) (*Fragment, error)
    List(ctx context.Context, tenantID string, tags ...string) ([]*Fragment, error)
    Rename(ctx context.Context, tenantID, uuid, newSlug string) error
    Watch(ctx context.Context, tenantID, slug string) (<-chan *Fragment, error)
}
```

`Promote(ctx, tenantID, ref string)` on `PromptActivator` tries UUID parse first; if `ref` is not a valid UUID it falls back to slug lookup. Resolution logic lives inside each backend implementation, not in the interface.

EmbedStore and FSStore return `ErrNotSupported` for `Rename` and `Promote` (read-only backends).

## HTTP API

```
GET    /v1/prompts                         list slugs
GET    /v1/prompts/{slug}                  get active version
GET    /v1/prompts/{slug}/versions         list versions
GET    /v1/prompts/{slug}/versions/{ver}   get specific version
POST   /v1/prompts                         create draft → returns {uuid, slug}
PUT    /v1/prompts/{uuid}/slug             rename slug
POST   /v1/prompts/{uuid}/activate         promote (body: {version})
DELETE /v1/prompts/{uuid}                  soft delete
POST   /v1/prompts/refresh                 force cache refresh (unchanged)
```

Read paths use slug (matches YAML). Write paths that survive rename use UUID. The existing `POST /v1/prompts/{name}/activate` route is renamed to `POST /v1/prompts/{uuid}/activate` — one breaking change on an existing endpoint.

## Consequences

- All five `PromptBackend` implementations need updating (breaking change).
- `PromptCache` Redis key prefix becomes `{tenantID}:prompt:{slug}` — unchanged from current.
- Renaming a slug invalidates any YAML `prompt:` references to the old slug; callers must update those YAML files.
- `tenantID` removed from `NewPostgresStore` constructor signature.
- `Fragment.ID` was previously the slug; any persisted data using the old schema needs migration (add `uuid` column, populate from `gen_random_uuid()`, add `slug` column from existing `fragment_id`).
