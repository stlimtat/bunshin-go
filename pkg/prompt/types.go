package prompt

import (
	"errors"
	"fmt"
)

// ErrNotSupported is returned by read-only backends (EmbedStore, FSStore) for
// write operations such as Rename and Promote.
var ErrNotSupported = errors.New("prompt: operation not supported by this backend")

// Fragment is the atomic, independently testable unit of a prompt.
//
// Two identifiers:
//   - ID:   UUIDv4 (PostgresStore) or UUIDv5 slug-derived (file-backed stores).
//     Immutable — survives slug renames. Used for stable HTTP API references.
//   - Slug: human-readable, tenant-unique, mutable string (e.g. "investigation-report-fragment").
//     Used for runtime template resolution and YAML workflow `prompt:` references.
type Fragment struct {
	// ID is the immutable UUID for this fragment.
	ID string
	// Slug is the human-readable, tenant-unique, mutable identifier.
	Slug string
	// Content is the Go text/template string. Variables are referenced as {{.VarName}}.
	Content string
	// Variables declares the input variables this fragment expects.
	Variables []string
	// Tags classify the fragment (e.g. "system", "persona", "tone", "task").
	Tags []string
	// Version allows pinning to a specific revision.
	Version string
	// Status is the lifecycle state: "draft", "active", or "deleted".
	// Empty on file-backed stores (EmbedStore, FSStore) which have no lifecycle.
	Status string
}

// Validate returns an error if any declared variable is absent from vars.
func (f *Fragment) Validate(vars map[string]any) error {
	for _, v := range f.Variables {
		if _, ok := vars[v]; !ok {
			return fmt.Errorf("fragment %q: missing variable %q", f.Slug, v)
		}
	}
	return nil
}

// FragmentRef is a reference to a Fragment within a PromptTemplate.
// Slug is the tenant-unique slug used for runtime resolution.
type FragmentRef struct {
	// Slug is the Fragment.Slug to fetch at render time.
	Slug      string
	Overrides map[string]any
	Condition string
}

// PromptTemplate is an ordered list of fragment references.
type PromptTemplate struct {
	Fragments []FragmentRef
	Separator string
}
