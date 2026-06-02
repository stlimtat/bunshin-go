package graph

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// Graph executes a DAG of Nodes.
// Implements core.Runnable — graphs compose inside chains or other graphs.
type Graph struct {
	name       string
	nodes      map[string]*Node
	entryPoint string
}

// New constructs an empty Graph with the given name.
func New(name string) *Graph {
	return &Graph{name: name, nodes: make(map[string]*Node)}
}

func (g *Graph) Name() string { return g.name }

// AddNode adds a node to the graph. Panics on duplicate ID.
func (g *Graph) AddNode(n Node) *Graph {
	if _, exists := g.nodes[n.ID]; exists {
		panic(fmt.Sprintf("graph %q: duplicate node %q", g.name, n.ID))
	}
	g.nodes[n.ID] = &n
	return g
}

// SetEntry sets the entry-point node ID. Must be called before Invoke.
func (g *Graph) SetEntry(nodeID string) *Graph {
	g.entryPoint = nodeID
	return g
}

// Invoke executes the graph starting from the entry-point node.
func (g *Graph) Invoke(ctx context.Context, input any) (any, error) {
	if g.entryPoint == "" {
		return nil, fmt.Errorf("graph %q: no entry point set", g.name)
	}

	current := g.entryPoint
	currentInput := input

	for current != END {
		node, ok := g.nodes[current]
		if !ok {
			return nil, fmt.Errorf("graph %q: node %q not found", g.name, current)
		}

		out, err := node.Runnable.Invoke(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("graph %q node %q: %w", g.name, current, err)
		}

		if node.Router == nil {
			return out, nil
		}

		next, err := node.Router(ctx, out)
		if err != nil {
			return nil, fmt.Errorf("graph %q node %q router: %w", g.name, current, err)
		}

		currentInput = out
		current = next
	}

	return currentInput, nil
}

// Stream executes the graph and wraps the terminal node's output in a single-chunk stream.
func (g *Graph) Stream(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := g.Invoke(ctx, input)
		ch <- core.StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}
