package skill

import (
	"context"
	"fmt"
	"sync"
)

// FakeStore is a test double for store.Store with recorded calls.
type FakeStore struct {
	mu              sync.RWMutex
	specs           map[string]map[string]*Spec  // tenantID → name → spec
	activeVersions  map[string]map[string]string // tenantID → name → version
	deleted         map[string]map[string]bool   // tenantID → name → deleted
	createCalls     []createCall
	getCalls        []getCall
	getVersionCalls []getVersionCall
	activateCalls   []activateCall
	deleteCalls     []deleteCall
}

type createCall struct {
	tenantID string
	spec     *Spec
}

type getCall struct {
	tenantID string
	name     string
}

type getVersionCall struct {
	tenantID string
	name     string
	version  string
}

type activateCall struct {
	tenantID string
	name     string
	version  string
}

type deleteCall struct {
	tenantID string
	name     string
}

// NewFakeStore returns an empty FakeStore.
func NewFakeStore() *FakeStore {
	return &FakeStore{
		specs:          make(map[string]map[string]*Spec),
		activeVersions: make(map[string]map[string]string),
		deleted:        make(map[string]map[string]bool),
	}
}

// Create records the call and stores the spec.
func (f *FakeStore) Create(ctx context.Context, tenantID string, spec *Spec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if spec == nil {
		return "", fmt.Errorf("FakeStore.Create: spec is nil")
	}

	f.createCalls = append(f.createCalls, createCall{tenantID, spec})

	if _, ok := f.specs[tenantID]; !ok {
		f.specs[tenantID] = make(map[string]*Spec)
		f.deleted[tenantID] = make(map[string]bool)
	}

	f.specs[tenantID][spec.Name] = spec
	f.deleted[tenantID][spec.Name] = false
	return spec.Version, nil
}

// Get returns the active spec or ErrNotFound.
func (f *FakeStore) Get(ctx context.Context, tenantID, name string) (*Spec, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.getCalls = append(f.getCalls, getCall{tenantID, name})

	if f.deleted[tenantID][name] {
		return nil, fmt.Errorf("skill %q: %w", name, ErrNotFound)
	}

	version, ok := f.activeVersions[tenantID][name]
	if !ok || version == "" {
		return nil, fmt.Errorf("skill %q: %w", name, ErrNotFound)
	}

	spec, ok := f.specs[tenantID][name]
	if !ok {
		return nil, fmt.Errorf("skill %q: %w", name, ErrNotFound)
	}
	return spec, nil
}

// GetVersion returns the named version or ErrNotFound.
func (f *FakeStore) GetVersion(ctx context.Context, tenantID, name, version string) (*Spec, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.getVersionCalls = append(f.getVersionCalls, getVersionCall{tenantID, name, version})

	spec, ok := f.specs[tenantID][name]
	if !ok {
		return nil, fmt.Errorf("skill %q version %q: %w", name, version, ErrNotFound)
	}
	if spec.Version != version {
		return nil, fmt.Errorf("skill %q version %q: %w", name, version, ErrNotFound)
	}
	return spec, nil
}

// List returns all non-deleted skill names for the tenant.
func (f *FakeStore) List(ctx context.Context, tenantID string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	bucket, ok := f.specs[tenantID]
	if !ok {
		return nil, nil
	}

	var names []string
	for name, spec := range bucket {
		if !f.deleted[tenantID][name] && spec != nil {
			names = append(names, name)
		}
	}
	return names, nil
}

// ListVersions returns all versions for the named skill.
func (f *FakeStore) ListVersions(ctx context.Context, tenantID, name string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	spec, ok := f.specs[tenantID][name]
	if !ok {
		return nil, fmt.Errorf("skill %q: %w", name, ErrNotFound)
	}
	return []string{spec.Version}, nil
}

// Activate records the call and marks the version as active.
func (f *FakeStore) Activate(ctx context.Context, tenantID, name, version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.activateCalls = append(f.activateCalls, activateCall{tenantID, name, version})

	spec, ok := f.specs[tenantID][name]
	if !ok || spec.Version != version {
		return fmt.Errorf("skill %q version %q: %w", name, version, ErrVersionConflict)
	}

	if _, ok := f.activeVersions[tenantID]; !ok {
		f.activeVersions[tenantID] = make(map[string]string)
	}
	f.activeVersions[tenantID][name] = version
	return nil
}

// Delete records the call and soft-deletes the skill.
func (f *FakeStore) Delete(ctx context.Context, tenantID, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deleteCalls = append(f.deleteCalls, deleteCall{tenantID, name})

	if _, ok := f.specs[tenantID][name]; !ok {
		return fmt.Errorf("skill %q: %w", name, ErrNotFound)
	}

	f.deleted[tenantID][name] = true
	f.activeVersions[tenantID][name] = ""
	return nil
}

// CreateCalls returns the recorded Create calls.
func (f *FakeStore) CreateCalls() []createCall {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]createCall, len(f.createCalls))
	copy(result, f.createCalls)
	return result
}

// ActivateCalls returns the recorded Activate calls.
func (f *FakeStore) ActivateCalls() []activateCall {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]activateCall, len(f.activateCalls))
	copy(result, f.activateCalls)
	return result
}

// DeleteCalls returns the recorded Delete calls.
func (f *FakeStore) DeleteCalls() []deleteCall {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]deleteCall, len(f.deleteCalls))
	copy(result, f.deleteCalls)
	return result
}
