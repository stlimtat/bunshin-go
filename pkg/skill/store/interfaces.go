// Package store provides the Store interface and implementations for skill storage.
//
// Skills follow the same draft→active versioning lifecycle as workflow.Spec.
// Versions are content hashes: sha256:<hex16> of the canonical YAML bytes.
// Identical content yields identical version across all backends, making
// Create idempotent and cross-backend migrations lossless.
package store

import (
	"context"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/skill"
)

// Store persists SkillSpecs with draft→active versioning.
type Store interface {
	// Create persists spec as a new draft version. Returns the content-hash
	// version string. Calling Create with identical content is idempotent.
	Create(ctx context.Context, tenantID string, spec *skill.Spec) (version string, err error)

	// Get returns the active version for name, or skill.ErrNotFound if none is active.
	Get(ctx context.Context, tenantID, name string) (*skill.Spec, error)

	// GetVersion returns the named version, or skill.ErrNotFound if absent.
	GetVersion(ctx context.Context, tenantID, name, version string) (*skill.Spec, error)

	// List returns the names of all non-deleted skills for the tenant.
	List(ctx context.Context, tenantID string) ([]string, error)

	// ListVersions returns all version strings for name in insertion order (oldest first).
	ListVersions(ctx context.Context, tenantID, name string) ([]string, error)

	// Activate promotes version to active for name.
	Activate(ctx context.Context, tenantID, name, version string) error

	// Delete soft-deletes the skill. Active runs are unaffected.
	Delete(ctx context.Context, tenantID, name string) error
}

// SkillVersion describes metadata about a skill version.
type SkillVersion struct {
	// Version is the content-hash version string.
	Version string
	// Status is "draft" or "active".
	Status string
	// CreatedAt is the timestamp when this version was created.
	CreatedAt time.Time
}
