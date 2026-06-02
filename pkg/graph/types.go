// Package graph provides DAG-based workflow execution and agent loops.
//
// A Graph is a directed acyclic graph of Nodes. Each Node is a Runnable
// with outgoing Edges that determine the next node based on the current output.
// A special END sentinel terminates the graph.
package graph

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// END is the sentinel node ID that terminates graph execution.
const END = "__end__"

// START is the built-in entry node ID.
const START = "__start__"

// Router is a function that decides which node to execute next.
// Return END to terminate execution.
type Router func(ctx context.Context, output any) (string, error)

// Node is one vertex in the execution graph.
type Node struct {
	// ID uniquely identifies this node within the graph.
	ID string
	// Runnable is the unit of work for this node.
	Runnable core.Runnable
	// Router determines the next node based on this node's output.
	// If nil, execution terminates (equivalent to routing to END).
	Router Router
}
