// Package prompt provides composable prompt management.
//
// Prompts are built from Fragments — atomic, independently testable pieces
// of prompt text. Fragments are assembled into PromptTemplates and rendered
// into final strings sent to LLMs.
//
// Each Fragment has two identifiers:
//   - ID: immutable UUID (UUIDv4 for PostgresStore, UUIDv5 for file-backed stores)
//   - Slug: mutable, tenant-unique human slug (used in YAML workflow `prompt:` refs)
//
// All PromptBackend methods take an explicit tenantID — one backend instance
// serves all tenants.
package prompt

import "context"

// TemplateEngine renders a template string with the given variables.
type TemplateEngine interface {
	Render(tmpl string, vars map[string]any) (string, error)
	// RenderLenient renders with missing variables treated as empty strings.
	// Used for condition expressions where absent variables should be falsy.
	RenderLenient(tmpl string, vars map[string]any) (string, error)
	// Validate parses the template and returns any syntax errors.
	Validate(tmpl string) error
}

// PromptActivator transitions a draft Fragment to active status.
// PostgresStore implements this; file-backed stores (EmbedStore, FSStore)
// return ErrNotSupported.
type PromptActivator interface {
	// Promote transitions the newest draft to active for the fragment identified
	// by ref. ref may be a UUID (stable across slug renames) or a slug (fallback).
	Promote(ctx context.Context, tenantID, ref string) error
}

// PromptBackend stores and retrieves Fragments.
// One instance serves all tenants — tenantID is passed per call.
// Implementations: MemoryBackend, EmbedStore, FSStore, PostgresStore, PromptCache.
type PromptBackend interface {
	// Put saves a fragment as a new draft. Generates UUID if f.ID is empty.
	Put(ctx context.Context, tenantID string, f *Fragment) error

	// Get returns the active version of the fragment identified by slug.
	// Used for runtime template resolution; YAML workflow `prompt:` references slug.
	Get(ctx context.Context, tenantID, slug string) (*Fragment, error)

	// GetByID returns a fragment by its immutable UUID. Used for management-plane
	// operations (delete, rename, activate) that must survive slug renames.
	GetByID(ctx context.Context, tenantID, id string) (*Fragment, error)

	// GetVersion returns a specific version of a fragment by slug (any status).
	GetVersion(ctx context.Context, tenantID, slug, version string) (*Fragment, error)

	// List returns all active fragments for tenantID matching all given tags.
	// Passing no tags returns all active fragments.
	List(ctx context.Context, tenantID string, tags ...string) ([]*Fragment, error)

	// Rename changes the slug of the fragment identified by uuid to newSlug.
	// Returns ErrNotSupported on read-only backends (EmbedStore, FSStore).
	Rename(ctx context.Context, tenantID, id, newSlug string) error

	// Watch sends fragment updates to the returned channel when the (tenant, slug)
	// pair changes. The channel is closed when ctx is cancelled.
	Watch(ctx context.Context, tenantID, slug string) (<-chan *Fragment, error)
}
