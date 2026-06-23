// Package store provides persistence for AgentSpec with draft→active lifecycle and content-hash versioning.
//
// Store is the interface that all agent storage backends must implement.
// One Store instance serves all tenants — tenantID is explicit per call.
//
// All specs are versioned by content hash: version = "sha256:" + hex(canonical_yaml[:32])
// Creating identical specs yields identical versions, enabling safe migrations and idempotency.
package store

import (
	"context"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/agent"
)

// AgentVersion describes a single version of an agent.
type AgentVersion struct {
	Version   string    // "sha256:..." content hash
	Status    string    // "draft", "active", or "deleted"
	CreatedAt time.Time // when this version was created
}

// Store persists agent.AgentSpec with draft→active lifecycle and content-hash versioning.
// One Store instance serves all tenants — tenantID explicit per call.
type Store interface {
	// Create writes the spec as a draft. Returns version string "sha256:...hex32(canonical_yaml)".
	// Idempotent: identical specs yield identical versions, allowing safe migrations.
	// If the agent was previously soft-deleted, Create resurrects it.
	Create(ctx context.Context, tenantID string, spec *agent.AgentSpec) (version string, err error)

	// Get returns the active AgentSpec for tenantID/name.
	Get(ctx context.Context, tenantID, name string) (*agent.AgentSpec, error)

	// GetVersion returns a specific version (any status: draft/active/deleted).
	GetVersion(ctx context.Context, tenantID, name, version string) (*agent.AgentSpec, error)

	// List returns all active agent names for tenantID.
	List(ctx context.Context, tenantID string) ([]string, error)

	// ListVersions returns all versions (newest-first) for tenantID/name.
	ListVersions(ctx context.Context, tenantID, name string) ([]AgentVersion, error)

	// Activate promotes a draft version to active for tenantID/name.
	Activate(ctx context.Context, tenantID, name, version string) error

	// Delete soft-deletes all versions for tenantID/name (sets status='deleted').
	Delete(ctx context.Context, tenantID, name string) error
}
