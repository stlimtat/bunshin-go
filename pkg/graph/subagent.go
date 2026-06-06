package graph

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// SubagentNode[Outer, Inner any] adapts a Graph[Inner] into a Node[Outer].
//
// Use it when a sub-workflow operates on a narrower state type than the parent
// graph. InjectFn slices the relevant fields into an Inner state; ExtractFn
// merges the Inner output back into the Outer state.
type SubagentNode[Outer, Inner any] struct {
	// ID uniquely identifies this node in the parent graph.
	ID string
	// Subgraph is the inner graph to invoke.
	Subgraph *Graph[Inner]
	// InjectFn converts the parent Outer state into the inner state.
	InjectFn func(core.State[Outer]) (core.State[Inner], error)
	// ExtractFn merges the inner output back into the parent Outer state.
	ExtractFn func(core.State[Outer], core.State[Inner]) (core.State[Outer], error)
	// Router is the routing function for the parent graph after this node runs.
	// If nil, execution ends after this node.
	Router Router[Outer]
}

// Invoke implements TypedRunnable[core.State[Outer], core.State[Outer]].
func (s *SubagentNode[Outer, Inner]) Invoke(ctx context.Context, input core.State[Outer]) (core.State[Outer], error) {
	inner, err := s.InjectFn(input)
	if err != nil {
		return input, fmt.Errorf("subagent %q inject: %w", s.ID, err)
	}

	innerOut, err := s.Subgraph.Invoke(ctx, inner)
	if err != nil {
		return input, fmt.Errorf("subagent %q: %w", s.ID, err)
	}

	out, err := s.ExtractFn(input, innerOut)
	if err != nil {
		return input, fmt.Errorf("subagent %q extract: %w", s.ID, err)
	}
	return out, nil
}

// AsNode returns this as a Node[Outer] ready to be added to a Graph[Outer].
func (s *SubagentNode[Outer, Inner]) AsNode() Node[Outer] {
	return Node[Outer]{
		ID:       s.ID,
		Runnable: s,
		Router:   s.Router,
	}
}
