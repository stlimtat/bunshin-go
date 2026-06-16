package workflow

import (
	"context"
	"fmt"
	"sync"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

// runnableEntry holds a named Runnable for custom YAML nodes.
type runnableEntry struct {
	name     string
	runnable core.Runnable
}

// RunnableRegistry holds Go-defined Runnables addressable by YAML custom nodes.
//
// Custom Runnables MUST accept core.State[map[string]any] as input and return
// core.State[map[string]any] as output. Returning a different type causes a
// runtime error when the compiler tries to extract the updated state.
//
// Typical registration:
//
//	reg.Register("my-step", core.AsRunnable[core.State[map[string]any], core.State[map[string]any]](
//	    "my-step", myTypedRunnable,
//	))
//
// Register at process start before any Compile calls. After that the registry
// is read-only and safe for concurrent access.
type RunnableRegistry struct {
	mu      sync.RWMutex
	entries map[string]runnableEntry
}

// NewRunnableRegistry returns an empty RunnableRegistry.
func NewRunnableRegistry() *RunnableRegistry {
	return &RunnableRegistry{entries: make(map[string]runnableEntry)}
}

// Register adds r under name. Panics if name is empty, r is nil, or name is already registered.
func (rr *RunnableRegistry) Register(name string, r core.Runnable) {
	if name == "" {
		panic("workflow.RunnableRegistry: name must be non-empty")
	}
	if r == nil {
		panic("workflow.RunnableRegistry: runnable must be non-nil")
	}
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if _, dup := rr.entries[name]; dup {
		panic(fmt.Sprintf("workflow.RunnableRegistry: duplicate name %q", name))
	}
	rr.entries[name] = runnableEntry{name: name, runnable: r}
}

// Get returns the Runnable registered under name, or an error if absent.
func (rr *RunnableRegistry) Get(name string) (core.Runnable, error) {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	e, ok := rr.entries[name]
	if !ok {
		return nil, fmt.Errorf("workflow.RunnableRegistry: runnable %q not found", name)
	}
	return e.runnable, nil
}

// RouterFactory is a function that builds a graph.Router[map[string]any] from config.
type RouterFactory func(config map[string]any) (graph.Router[map[string]any], error)

// RouterRegistry holds Router[map[string]any] factories addressable by YAML router type names.
// Register at process start before any Compile calls. After that the registry
// is read-only and safe for concurrent access.
type RouterRegistry struct {
	mu        sync.RWMutex
	factories map[string]RouterFactory
}

// NewRouterRegistry returns an empty RouterRegistry.
func NewRouterRegistry() *RouterRegistry {
	return &RouterRegistry{factories: make(map[string]RouterFactory)}
}

// Register adds factory under typeName. Panics on duplicate or empty name.
func (rr *RouterRegistry) Register(typeName string, factory RouterFactory) {
	if typeName == "" {
		panic("workflow.RouterRegistry: typeName must be non-empty")
	}
	if factory == nil {
		panic("workflow.RouterRegistry: factory must be non-nil")
	}
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if _, dup := rr.factories[typeName]; dup {
		panic(fmt.Sprintf("workflow.RouterRegistry: duplicate typeName %q", typeName))
	}
	rr.factories[typeName] = factory
}

// Build resolves ref to a graph.Router[map[string]any] using the registered factory.
func (rr *RouterRegistry) Build(ref *RouterRef, customRunnables *RunnableRegistry) (graph.Router[map[string]any], error) {
	if ref == nil {
		return nil, nil
	}
	if ref.Type == "custom" {
		return rr.buildCustom(ref, customRunnables)
	}
	rr.mu.RLock()
	factory, ok := rr.factories[ref.Type]
	rr.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("workflow.RouterRegistry: unknown router type %q", ref.Type)
	}
	return factory(ref.Config)
}

// buildCustom resolves a custom router by looking up a Runnable that returns a node ID string.
// The Runnable receives core.State[map[string]any] and must return a string node ID or graph.END.
func (rr *RouterRegistry) buildCustom(ref *RouterRef, cr *RunnableRegistry) (graph.Router[map[string]any], error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("workflow.RouterRegistry: custom router missing name")
	}
	r, err := cr.Get(ref.Name)
	if err != nil {
		return nil, fmt.Errorf("workflow.RouterRegistry: custom router: %w", err)
	}
	return func(ctx context.Context, state core.State[map[string]any]) (string, error) {
		out, err := r.Invoke(ctx, state)
		if err != nil {
			return "", err
		}
		s, ok := out.(string)
		if !ok {
			return "", fmt.Errorf("workflow: custom router %q returned %T, want string (node ID or graph.END)", ref.Name, out)
		}
		return s, nil
	}, nil
}
