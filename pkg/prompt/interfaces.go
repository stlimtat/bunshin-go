// Package prompt provides composable prompt management.
//
// Prompts are built from Fragments — atomic, independently testable pieces
// of prompt text. Fragments are assembled into PromptTemplates and rendered
// into final strings sent to LLMs.
//
// The Fragment → PromptTemplate → Renderer pipeline allows:
//   - Mix-and-match: swap individual fragments (tone, persona, task) per step
//   - Independent testing: validate each fragment in isolation
//   - Versioning: pin a workflow to a specific fragment version
//   - Hot reload: FileBackend and RemoteBackend Watch() methods push updates
//
// Backends are pluggable: use EmbedBackend to ship prompts in the binary,
// FileBackend for file-system prompts with hot reload, or RemoteBackend for
// a LangSmith-compatible prompt hub.
package prompt

import "context"

// TemplateEngine renders a template string with the given variables.
type TemplateEngine interface {
	Render(tmpl string, vars map[string]any) (string, error)
	// RenderLenient renders with missing variables treated as empty strings rather than errors.
	// Used for condition expressions where absent variables should evaluate as falsy.
	RenderLenient(tmpl string, vars map[string]any) (string, error)
	// Validate parses the template and returns any syntax errors.
	Validate(tmpl string) error
}

// PromptBackend stores and retrieves Fragments.
// Implementations include EmbedBackend, MemoryBackend, FileBackend, and RemoteBackend.
type PromptBackend interface {
	// Get retrieves a fragment by ID.
	Get(ctx context.Context, id string) (*Fragment, error)
	// List returns all fragments matching the given tags (empty = all).
	List(ctx context.Context, tags ...string) ([]*Fragment, error)
	// GetVersion retrieves a specific version of a fragment.
	GetVersion(ctx context.Context, id, version string) (*Fragment, error)
	// Watch sends updates to the returned channel when the fragment changes.
	// The channel is closed when ctx is cancelled.
	Watch(ctx context.Context, id string) (<-chan *Fragment, error)
}
