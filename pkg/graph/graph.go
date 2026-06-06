package graph

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// Graph[S] executes a directed graph of Node[S] vertices.
// Implements TypedRunnable[State[S], State[S]] — graphs compose inside chains
// or other graphs via Graph.AsRunnable().
//
// Because Router[S] can route back to earlier nodes, Graph[S] supports cyclic
// execution for agent loops. Callers are responsible for loop termination
// (routing to END when done).
type Graph[S any] struct {
	name       string
	nodes      map[string]*Node[S]
	entryPoint string
}

// New constructs an empty Graph with the given name.
func New[S any](name string) *Graph[S] {
	return &Graph[S]{name: name, nodes: make(map[string]*Node[S])}
}

// Name returns the graph identifier used in traces and logs.
func (g *Graph[S]) Name() string { return g.name }

// AddNode adds a node to the graph. Panics on duplicate ID.
func (g *Graph[S]) AddNode(n Node[S]) *Graph[S] {
	if _, exists := g.nodes[n.ID]; exists {
		panic(fmt.Sprintf("graph %q: duplicate node %q", g.name, n.ID))
	}
	g.nodes[n.ID] = &n
	return g
}

// SetEntry sets the entry-point node ID. Must be called before Invoke.
func (g *Graph[S]) SetEntry(nodeID string) *Graph[S] {
	g.entryPoint = nodeID
	return g
}

// Invoke executes the graph starting from the entry-point node.
// State[S] flows through nodes; each node receives and returns the full state.
// Execution ends when a Router returns END or a node has no Router.
func (g *Graph[S]) Invoke(ctx context.Context, input core.State[S]) (core.State[S], error) {
	if g.entryPoint == "" {
		return input, fmt.Errorf("graph %q: no entry point set", g.name)
	}

	current := g.entryPoint
	currentInput := input

	for current != END {
		if err := ctx.Err(); err != nil {
			return currentInput, err
		}
		node, ok := g.nodes[current]
		if !ok {
			return currentInput, fmt.Errorf("graph %q: node %q not found", g.name, current)
		}

		out, err := node.Runnable.Invoke(ctx, currentInput)
		if err != nil {
			return currentInput, fmt.Errorf("graph %q node %q: %w", g.name, current, err)
		}

		if node.Router == nil {
			return out, nil
		}

		next, err := node.Router(ctx, out)
		if err != nil {
			return out, fmt.Errorf("graph %q node %q router: %w", g.name, current, err)
		}

		currentInput = out
		current = next
	}

	return currentInput, nil
}

// AsRunnable wraps the graph as a core.Runnable for middleware composition.
func (g *Graph[S]) AsRunnable() core.Runnable {
	return core.AsRunnable[core.State[S], core.State[S]](g.name, g)
}
