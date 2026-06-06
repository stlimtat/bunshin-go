// hello-agent demonstrates an agent loop using pkg/graph.
// The agent loop increments a counter at each step and routes back to itself
// until the counter reaches 5, then terminates.
//
// This pattern is the basis for real LLM+tool agent loops: the LLM node decides
// whether to call a tool (loop) or produce a final answer (END).
//
// Usage:
//
//	go run ./examples/hello-agent
package main

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

type agentState struct {
	Step    int
	History []string
}

func main() {
	g := graph.New[agentState]("hello-agent").
		AddNode(graph.Node[agentState]{
			ID: "step",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[agentState]) (core.State[agentState], error) {
				next := agentState{
					Step:    s.Data.Step + 1,
					History: append(s.Data.History, fmt.Sprintf("step %d", s.Data.Step+1)),
				}
				return core.NewState(next), nil
			}),
			Router: func(_ context.Context, s core.State[agentState]) (string, error) {
				if s.Data.Step >= 5 {
					return graph.END, nil
				}
				return "step", nil
			},
		}).
		SetEntry("step")

	out, err := g.Invoke(context.Background(), core.NewState(agentState{}))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("Completed in %d steps:\n", out.Data.Step)
	for _, h := range out.Data.History {
		fmt.Println(" -", h)
	}
}
