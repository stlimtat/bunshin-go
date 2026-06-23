package store

import (
	"context"
	"fmt"
)

// FakeStore is a test fake Store for use in agent.Compile and related tests.
type FakeStore struct {
	specs map[string]*AgentSpec // name → spec
}

// NewFakeStore returns an empty FakeStore.
func NewFakeStore() *FakeStore {
	return &FakeStore{
		specs: make(map[string]*AgentSpec),
	}
}

// Create records spec by name. Ignores tenantID.
func (s *FakeStore) Create(ctx context.Context, tenantID string, spec *AgentSpec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("fake.Store.Create: spec is nil")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("fake.Store.Create: spec.Name must not be empty")
	}
	version, err := contentHashYAML(spec)
	if err != nil {
		return "", err
	}
	s.specs[spec.Name] = spec
	return version, nil
}

// Get returns the spec by name.
func (s *FakeStore) Get(ctx context.Context, tenantID, name string) (*AgentSpec, error) {
	spec, ok := s.specs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q: not found", name)
	}
	return cloneSpec(spec), nil
}

// GetVersion returns the spec if name is found and version matches.
func (s *FakeStore) GetVersion(ctx context.Context, tenantID, name, version string) (*AgentSpec, error) {
	spec, ok := s.specs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q: not found", name)
	}
	specVersion, err := contentHashYAML(spec)
	if err != nil {
		return nil, err
	}
	if specVersion != version {
		return nil, fmt.Errorf("agent %q version %q: mismatch", name, version)
	}
	return cloneSpec(spec), nil
}

// List returns all recorded agent names.
func (s *FakeStore) List(ctx context.Context, tenantID string) ([]string, error) {
	names := make([]string, 0, len(s.specs))
	for name := range s.specs {
		names = append(names, name)
	}
	return names, nil
}

// ListVersions returns a single version for the agent.
func (s *FakeStore) ListVersions(ctx context.Context, tenantID, name string) ([]AgentVersion, error) {
	spec, ok := s.specs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q: not found", name)
	}
	version, err := contentHashYAML(spec)
	if err != nil {
		return nil, err
	}
	return []AgentVersion{
		{
			Version: version,
			Status:  "active",
		},
	}, nil
}

// Activate is a no-op.
func (s *FakeStore) Activate(ctx context.Context, tenantID, name, version string) error {
	spec, ok := s.specs[name]
	if !ok {
		return fmt.Errorf("agent %q: not found", name)
	}
	specVersion, err := contentHashYAML(spec)
	if err != nil {
		return err
	}
	if specVersion != version {
		return fmt.Errorf("agent %q version %q: mismatch", name, version)
	}
	return nil
}

// Delete removes the agent from the store.
func (s *FakeStore) Delete(ctx context.Context, tenantID, name string) error {
	_, ok := s.specs[name]
	if !ok {
		return fmt.Errorf("agent %q: not found", name)
	}
	delete(s.specs, name)
	return nil
}
