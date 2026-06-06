package sandbox

import (
	"errors"
	"sync"
)

// Tags is a set of key-value metadata attached to a registered Sandbox.
// Tags encode both selection criteria (language, env, tenant_tier) and
// resource limits (memory_mb, cpu_millicores, inline_max_bytes).
// This mirrors llm.Tags and is part of bunshin-go's tag-based registry pattern.
type Tags map[string]string

// sandboxEntry holds a sandbox backend with its associated tags.
type sandboxEntry struct {
	sandbox Sandbox
	tags    Tags
}

// SandboxRegistry holds named Sandbox instances tagged with selection criteria
// and resource limits. Callers select backends at call time using tag filters.
//
// Tags serve dual purpose: selection (which backend to use) and configuration
// (resource limits applied at registration time). Limits are immutable per entry.
//
// This is the same tag-based pattern as llm.ProviderRegistry, extended to
// sandboxes. Multiple backends of the same type coexist with different resource
// profiles for multi-tenant deployments.
type SandboxRegistry struct {
	mu      sync.RWMutex
	entries map[string]sandboxEntry
}

// NewSandboxRegistry returns an empty registry.
func NewSandboxRegistry() *SandboxRegistry {
	return &SandboxRegistry{entries: make(map[string]sandboxEntry)}
}

// Register adds a named sandbox backend with tags.
// Tags encode both selection criteria and resource limits.
func (r *SandboxRegistry) Register(name string, s Sandbox, tags Tags) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[name] = sandboxEntry{sandbox: s, tags: tags}
}

// Select returns sandboxes whose tags contain all key-value pairs in each filter.
// Multiple Tag() calls are ANDed. Returns nil if no match.
func (r *SandboxRegistry) Select(filters ...Tags) []Sandbox {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []Sandbox
	for _, entry := range r.entries {
		if sandboxTagsMatch(entry.tags, filters) {
			result = append(result, entry.sandbox)
		}
	}
	return result
}

// Get returns the sandbox registered under name, if any.
func (r *SandboxRegistry) Get(name string) (Sandbox, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e.sandbox, ok
}

func sandboxTagsMatch(entryTags Tags, filters []Tags) bool {
	for _, filter := range filters {
		for k, v := range filter {
			if entryTags[k] != v {
				return false
			}
		}
	}
	return true
}

// ErrUnsupportedLanguage is returned when the backend cannot run the requested language.
var ErrUnsupportedLanguage = errors.New("unsupported language")

// ErrSessionClosed is returned when Run is called on a closed Session.
var ErrSessionClosed = errors.New("session closed")
