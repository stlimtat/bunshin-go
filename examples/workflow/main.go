// hello-workflow demonstrates a two-node sequential workflow using pkg/graph.
// Node A uppercases the input; Node B appends an exclamation mark.
// The graph composes them into a typed State[string] workflow.
//
// Usage:
//
//	go run ./examples/workflow
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

func main() {
	g := graph.New[string]("hello-workflow").
		AddNode(graph.Node[string]{
			ID: "upper",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[string]) (core.State[string], error) {
				return core.NewState(strings.ToUpper(s.Data)), nil
			}),
			Router: graph.StaticRouter[string]("exclaim"),
		}).
		AddNode(graph.Node[string]{
			ID: "exclaim",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[string]) (core.State[string], error) {
				return core.NewState(s.Data + "!"), nil
			}),
		}).
		SetEntry("upper")

	out, err := g.Invoke(context.Background(), core.NewState("hello world"))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Println(out.Data) // HELLO WORLD!
}
