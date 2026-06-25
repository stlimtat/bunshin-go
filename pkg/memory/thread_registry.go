package memory

import (
	"context"
	"sync"
)

// MemoryThreadRegistry is an in-process ThreadRegistry backed by MemoryStore instances.
// Suitable for development and testing; use a persistent registry in production.
type MemoryThreadRegistry struct {
	mu      sync.Mutex
	threads map[string]*MemoryStore
}

// NewMemoryThreadRegistry returns an empty in-memory registry.
func NewMemoryThreadRegistry() *MemoryThreadRegistry {
	return &MemoryThreadRegistry{threads: make(map[string]*MemoryStore)}
}

// List returns all known thread IDs.
func (r *MemoryThreadRegistry) List(_ context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.threads))
	for id := range r.threads {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetOrCreate returns the MessageStore for threadID, creating it if absent.
func (r *MemoryThreadRegistry) GetOrCreate(_ context.Context, threadID string) (MessageStore, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.threads[threadID]; ok {
		return s, nil
	}
	s := NewMemoryStore()
	r.threads[threadID] = s
	return s, nil
}
