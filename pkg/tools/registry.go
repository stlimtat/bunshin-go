package tools

import (
	"fmt"
	"sync"
)

// ToolRegistry holds named tools and provides lookup by name.
// Safe for concurrent use.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewToolRegistry constructs an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Returns an error if the name is already taken.
func (r *ToolRegistry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Schema().Name
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// MustRegister registers a tool and panics if registration fails.
// Intended for use in init() functions where duplicate registration is a bug.
func (r *ToolRegistry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Get returns the tool with the given name, or an error if not found.
func (r *ToolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t, nil
}

// List returns the schemas of all registered tools.
func (r *ToolRegistry) List() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	schemas := make([]ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, t.Schema())
	}
	return schemas
}
