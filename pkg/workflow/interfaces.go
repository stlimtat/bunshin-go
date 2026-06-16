// Package workflow provides declarative YAML workflow definitions that compile
// to Graph[core.State[map[string]any]] runnables.
//
// A Spec names a directed graph of nodes. Linear pipelines omit routing
// entirely; the compiler auto-wires position-based edges. Cyclic graphs
// (agent loops) declare an explicit router on the node that branches.
//
// # Node types
//
//   - llm    — call an LLMProvider with a rendered prompt fragment
//   - tool   — invoke a registered Tool
//   - custom — invoke a Go-registered Runnable by name
//
// # Version identity
//
// Versions are content hashes: sha256:<hex16> of the canonical YAML bytes.
// Identical content yields identical version across all backends, making
// Create idempotent and cross-backend migrations lossless.
//
// # Lifecycle
//
// Specs follow the same draft→active lifecycle as prompt Fragments.
// Create writes a draft; Activate promotes it. Delete soft-deletes.
package workflow

import "context"

// Store persists WorkflowSpecs with draft→active versioning.
type Store interface {
	// Create persists spec as a new draft version. Returns the content-hash
	// version string. Calling Create with identical content is idempotent.
	Create(ctx context.Context, tenantID string, spec *Spec) (version string, err error)

	// Get returns the active version for name, or ErrNotFound if none is active.
	Get(ctx context.Context, tenantID, name string) (*Spec, error)

	// GetVersion returns the named version, or ErrNotFound if absent.
	GetVersion(ctx context.Context, tenantID, name, version string) (*Spec, error)

	// List returns the names of all non-deleted workflows for the tenant.
	List(ctx context.Context, tenantID string) ([]string, error)

	// ListVersions returns all version strings for name, newest first.
	ListVersions(ctx context.Context, tenantID, name string) ([]string, error)

	// Activate promotes version to active for name.
	Activate(ctx context.Context, tenantID, name, version string) error

	// Delete soft-deletes the workflow. Active runs are unaffected.
	Delete(ctx context.Context, tenantID, name string) error
}

