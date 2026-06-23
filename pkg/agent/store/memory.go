package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/agent"
)

// entry holds all versions of one agent for one tenant.
type entry struct {
	versions    map[string]*agent.AgentSpec // version → spec
	versionList []string                     // insertion-ordered version strings (oldest first)
	active      string                       // version string of active spec, "" = none
	deleted     bool
}

// MemoryStore is a thread-safe in-memory implementation of Store.
// All data is lost when the process exits. Use for testing and local development.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]map[string]*entry // tenantID → name → entry
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]map[string]*entry)}
}

// Create persists spec as a new draft. Idempotent: same content = same version,
// second call is a no-op and returns the existing version.
// If the agent was previously soft-deleted, Create resurrects it.
func (s *MemoryStore) Create(ctx context.Context, tenantID string, spec *agent.AgentSpec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("memory.Store.Create: spec is nil")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("memory.Store.Create: spec.Name must not be empty")
	}

	version, err := contentHashYAML(spec)
	if err != nil {
		return "", fmt.Errorf("memory.Store.Create: hash: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.tenantBucket(tenantID)
	e, ok := bucket[spec.Name]
	if !ok {
		e = &entry{versions: make(map[string]*agent.AgentSpec)}
		bucket[spec.Name] = e
	}
	// Resurrect soft-deleted entries.
	e.deleted = false

	if _, exists := e.versions[version]; exists {
		return version, nil
	}
	clone := cloneSpec(spec)
	bucket[spec.Name] = e
	e.versions[version] = clone
	e.versionList = append(e.versionList, version)
	return version, nil
}

// Get returns the active version, or an error if none exists.
func (s *MemoryStore) Get(ctx context.Context, tenantID, name string) (*agent.AgentSpec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return nil, err
	}
	if e.active == "" {
		return nil, fmt.Errorf("agent %q tenant %q: no active version", name, tenantID)
	}
	return cloneSpec(e.versions[e.active]), nil
}

// GetVersion returns the specific version, or an error if absent.
func (s *MemoryStore) GetVersion(ctx context.Context, tenantID, name, version string) (*agent.AgentSpec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return nil, err
	}
	sp, ok := e.versions[version]
	if !ok {
		return nil, fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
	}
	return cloneSpec(sp), nil
}

// List returns non-deleted agent names for the tenant.
func (s *MemoryStore) List(ctx context.Context, tenantID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bucket, ok := s.data[tenantID]
	if !ok {
		return nil, nil
	}
	names := make([]string, 0, len(bucket))
	for name, e := range bucket {
		if !e.deleted {
			names = append(names, name)
		}
	}
	return names, nil
}

// ListVersions returns all version metadata for name in insertion order (oldest first, newest last).
func (s *MemoryStore) ListVersions(ctx context.Context, tenantID, name string) ([]AgentVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return nil, err
	}

	result := make([]AgentVersion, 0, len(e.versionList))
	for _, v := range e.versionList {
		status := "draft"
		if v == e.active {
			status = "active"
		}
		result = append(result, AgentVersion{
			Version:   v,
			Status:    status,
			CreatedAt: time.Now(), // memory store doesn't track creation times
		})
	}
	// Reverse to return newest-first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

// Activate promotes version to active. Returns an error if version is absent.
func (s *MemoryStore) Activate(ctx context.Context, tenantID, name, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return err
	}
	if _, ok := e.versions[version]; !ok {
		return fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
	}
	// Mark the previously active version back to draft.
	if e.active != "" && e.active != version {
		// (no explicit Status field to update in memory)
	}
	e.active = version
	return nil
}

// Delete soft-deletes the agent. Get will return an error afterwards.
func (s *MemoryStore) Delete(ctx context.Context, tenantID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return err
	}
	e.deleted = true
	e.active = ""
	return nil
}

// findEntry returns the entry for tenantID+name, or an error.
// Must be called with at least an RLock held.
func (s *MemoryStore) findEntry(tenantID, name string) (*entry, error) {
	bucket, ok := s.data[tenantID]
	if !ok {
		return nil, fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}
	e, ok := bucket[name]
	if !ok || e.deleted {
		return nil, fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}
	return e, nil
}

// tenantBucket returns the bucket for tenantID, creating if needed.
// Must be called with a lock held.
func (s *MemoryStore) tenantBucket(tenantID string) map[string]*entry {
	b, ok := s.data[tenantID]
	if !ok {
		b = make(map[string]*entry)
		s.data[tenantID] = b
	}
	return b
}

// cloneSpec returns a shallow copy of spec.
func cloneSpec(s *agent.AgentSpec) *agent.AgentSpec {
	if s == nil {
		return nil
	}
	clone := *s
	return &clone
}

