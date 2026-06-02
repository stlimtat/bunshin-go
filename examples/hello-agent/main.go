// hello-agent demonstrates a full agent loop using bunshin-go's graph package.
//
// The agent has two tools (calculator, echo) and loops until the LLM
// decides to return a final answer (no more tool calls).
//
// Run:
//
//	go run ./examples/hello-agent
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// agentState carries the conversation across agent loop iterations.
type agentState struct {
	Question string
	Steps    []string
	Answer   string
	Done     bool
}

func main() {
	// Register tools.
	reg := tools.NewToolRegistry()
	reg.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "calc", Description: "Evaluate a simple arithmetic expression like '2+2'"},
		func(_ context.Context, input any) (any, error) {
			expr := fmt.Sprintf("%v", input)
			parts := strings.Fields(expr)
			if len(parts) == 3 {
				a, _ := strconv.Atoi(parts[0])
				b, _ := strconv.Atoi(parts[2])
				switch parts[1] {
				case "+":
					return a + b, nil
				case "*":
					return a * b, nil
				}
			}
			return nil, fmt.Errorf("unsupported expression: %q", expr)
		},
	))
	reg.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "echo", Description: "Echo the input back"},
		func(_ context.Context, input any) (any, error) { return input, nil },
	))

	// Build agent graph.
	// reason: decide whether to use a tool or answer directly.
	// tool:   execute the chosen tool.
	// answer: format and return the final answer.
	reasonNode := core.NewRunnableFunc("reason", func(_ context.Context, input any) (any, error) {
		state := input.(agentState)
		// Stub: real agent would call LLM here.
		// For demo: if question contains "+", use calc tool.
		if strings.Contains(state.Question, "+") && !state.Done {
			return agentState{
				Question: state.Question,
				Steps:    append(state.Steps, "decided to use calc"),
				Done:     false,
			}, nil
		}
		return agentState{
			Question: state.Question,
			Steps:    state.Steps,
			Answer:   "42",
			Done:     true,
		}, nil
	})

	toolNode := core.NewRunnableFunc("tool", func(_ context.Context, input any) (any, error) {
		state := input.(agentState)
		tool, _ := reg.Get("calc")
		result, err := tool.Invoke(context.Background(), state.Question)
		if err != nil {
			return nil, err
		}
		return agentState{
			Question: state.Question,
			Steps:    append(state.Steps, fmt.Sprintf("calc=%v", result)),
			Answer:   fmt.Sprintf("%v", result),
			Done:     true,
		}, nil
	})

	answerNode := core.NewRunnableFunc("answer", func(_ context.Context, input any) (any, error) {
		return input.(agentState).Answer, nil
	})

	g := graph.New("agent-loop").
		AddNode(graph.Node{
			ID:       "reason",
			Runnable: reasonNode,
			Router: graph.ConditionalRouter(
				func(out any) string {
					if out.(agentState).Done {
						return "answer"
					}
					return "tool"
				},
				map[string]string{"answer": "answer", "tool": "tool"},
			),
		}).
		AddNode(graph.Node{
			ID:       "tool",
			Runnable: toolNode,
			Router:   graph.StaticRouter("reason"),
		}).
		AddNode(graph.Node{
			ID:       "answer",
			Runnable: answerNode,
		}).
		SetEntry("reason")

	// Wrap with logging.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	agent := middleware.Chain(g, middleware.WithLogging(logger), middleware.WithPanicRecovery())

	question := "3 + 4"
	result, err := agent.Invoke(context.Background(), agentState{Question: question})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Question: %s\nAnswer: %v\n", question, result)
}
