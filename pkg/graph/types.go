// Package graph provides DAG-based workflow execution and agent loops.
//
// A Graph[S] is a directed graph of Node[S] vertices. Each node runs a
// TypedRunnable[State[S], State[S]] and a Router[S] that decides the next node.
// A special END sentinel terminates execution.
//
// Because Routers can route back to earlier nodes, Graph[S] supports cyclic
// execution — essential for agent loops where the LLM and tool nodes alternate
// until a termination condition is met.
package graph

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// END is the sentinel node ID that terminates graph execution.
const END = "__end__"

// START is the built-in entry node ID.
const START = "__start__"

// Router[S] decides which node to execute next given the current state.
// Return END to terminate execution.
type Router[S any] func(ctx context.Context, state core.State[S]) (string, error)

// Node[S] is one vertex in the execution graph.
type Node[S any] struct {
	// ID uniquely identifies this node within the graph.
	ID string
	// Runnable is the unit of work for this node.
	Runnable core.TypedRunnable[core.State[S], core.State[S]]
	// Router determines the next node based on the output state.
	// If nil, execution terminates (equivalent to routing to END).
	Router Router[S]
}
