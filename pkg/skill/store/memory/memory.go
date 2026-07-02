// Package memory provides an in-memory implementation of skill.Store.
// All data is lost when the process exits. Use for testing and local development.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/stlimtat/bunshin-go/pkg/skill"
)

// entry holds all versions of one skill for one tenant.
type entry struct {
	versions    map[string]*skill.Spec // version → spec
	versionList []string               // insertion-ordered version strings (oldest first)
	active      string                 // version string of active spec, "" = none
	deleted     bool
}

// Store is a thread-safe in-memory implementation of skill.Store.
type Store struct {
	mu   sync.RWMutex
	data map[string]map[string]*entry // tenantID → name → entry
}

// New returns an empty in-memory Store.
func New() *Store {
	return &Store{data: make(map[string]map[string]*entry)}
}

func (s *Store) tenantBucket(tenantID string) map[string]*entry {
	b, ok := s.data[tenantID]
	if !ok {
		b = make(map[string]*entry)
		s.data[tenantID] = b
	}
	return b
}

// Create persists spec as a new draft. Idempotent: same content = same version,
// second call is a no-op and returns the existing version.
// If the skill was previously soft-deleted, Create resurrects it.
func (s *Store) Create(_ context.Context, tenantID string, spec *skill.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("memory.Store.Create: spec is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.tenantBucket(tenantID)
	e, ok := bucket[spec.Name]
	if !ok {
		e = &entry{versions: make(map[string]*skill.Spec)}
		bucket[spec.Name] = e
	}
	// Resurrect soft-deleted entries.
	e.deleted = false

	if _, exists := e.versions[spec.Version]; exists {
		return spec.Version, nil
	}
	clone := cloneSpec(spec)
	clone.Status = skill.StatusDraft
	e.versions[spec.Version] = clone
	e.versionList = append(e.versionList, spec.Version)
	return spec.Version, nil
}

// Get returns the active version, or skill.ErrNotFound if none.
func (s *Store) Get(_ context.Context, tenantID, name string) (*skill.Spec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return nil, err
	}
	if e.active == "" {
		return nil, fmt.Errorf("skill %q: no active version: %w", name, skill.ErrNotFound)
	}
	return cloneSpec(e.versions[e.active]), nil
}

// GetVersion returns the specific version, or skill.ErrNotFound if absent.
func (s *Store) GetVersion(_ context.Context, tenantID, name, version string) (*skill.Spec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return nil, err
	}
	sp, ok := e.versions[version]
	if !ok {
		return nil, fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrNotFound)
	}
	return cloneSpec(sp), nil
}

// List returns non-deleted skill names for the tenant.
func (s *Store) List(_ context.Context, tenantID string) ([]string, error) {
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

// ListVersions returns all version strings for name in insertion order (oldest first).
func (s *Store) ListVersions(_ context.Context, tenantID, name string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(e.versionList))
	copy(result, e.versionList)
	return result, nil
}

// Activate promotes version to active. Returns skill.ErrVersionConflict if absent.
func (s *Store) Activate(_ context.Context, tenantID, name, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, err := s.findEntry(tenantID, name)
	if err != nil {
		return err
	}
	if _, ok := e.versions[version]; !ok {
		return fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrVersionConflict)
	}
	// Mark the previously active version back to draft.
	if prev, ok := e.versions[e.active]; ok && e.active != version {
		prev.Status = skill.StatusDraft
	}
	e.versions[version].Status = skill.StatusActive
	e.active = version
	return nil
}

// Delete soft-deletes the skill. Get will return skill.ErrNotFound afterwards.
func (s *Store) Delete(_ context.Context, tenantID, name string) error {
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

// findEntry returns the entry for tenantID+name, or skill.ErrNotFound.
// Must be called with at least an RLock held.
func (s *Store) findEntry(tenantID, name string) (*entry, error) {
	bucket, ok := s.data[tenantID]
	if !ok {
		return nil, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	e, ok := bucket[name]
	if !ok || e.deleted {
		return nil, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	return e, nil
}

// cloneSpec returns a shallow copy of spec with a copied Files slice.
func cloneSpec(s *skill.Spec) *skill.Spec {
	clone := *s
	if len(s.Files) > 0 {
		clone.Files = make([]skill.FileRef, len(s.Files))
		copy(clone.Files, s.Files)
	}
	return &clone
}
