package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// agentState carries conversation state across agent loop iterations.
type agentState struct {
	Question  string
	ToolInput string // extracted arithmetic expression passed to the calc tool
	Steps     []string
	Answer    string
	Done      bool
}

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent loop with arithmetic tools demo",
		Long: `Demonstrates a full agent loop using bunshin-go's graph package.

The agent has two built-in tools:
  calc   Evaluate arithmetic expressions: "2 + 2", "6 * 7"
  echo   Echo the input back

The agent loops (reason → tool → reason) until it produces a final answer.
Natural-language questions like "What is 6*7?" are handled by extracting
the arithmetic expression before dispatching to the calc tool.`,
		Example: `  bunshin agent --question "3 + 4"
  bunshin agent --question "What is 6*7?"
  bunshin agent --question "Tell me something"`,
		RunE: runAgent,
	}
	cmd.Flags().String("question", "3 + 4", "Question for the agent to answer")
	mustBindFlag(cmd, "agent_question", "question")
	return cmd
}

// extractArithExpr strips natural language, returning only the arithmetic expression
// normalised with spaces. "What is 2+2?" → "2 + 2"
func extractArithExpr(q string) string {
	for _, op := range []string{"+", "-", "*", "/"} {
		q = strings.ReplaceAll(q, op, " "+op+" ")
	}
	var b strings.Builder
	for _, c := range q {
		if (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '*' || c == '/' || c == ' ' {
			b.WriteRune(c)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func runAgent(_ *cobra.Command, _ []string) error {
	question := viper.GetString("agent_question")

	reg := tools.NewToolRegistry()
	reg.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "calc", Description: "Evaluate a simple arithmetic expression like '2 + 2'"},
		func(_ context.Context, input any) (any, error) {
			expr := fmt.Sprintf("%v", input)
			parts := strings.Fields(expr)
			if len(parts) == 3 {
				a, _ := strconv.Atoi(parts[0])
				b, _ := strconv.Atoi(parts[2])
				switch parts[1] {
				case "+":
					return a + b, nil
				case "-":
					return a - b, nil
				case "*":
					return a * b, nil
				case "/":
					if b == 0 {
						return nil, fmt.Errorf("division by zero")
					}
					return a / b, nil
				}
			}
			return nil, fmt.Errorf("unsupported expression: %q", expr)
		},
	))
	reg.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "echo", Description: "Echo the input back"},
		func(_ context.Context, input any) (any, error) { return input, nil },
	))

	reasonNode := core.NewRunnableFunc("reason", func(_ context.Context, input any) (any, error) {
		state := input.(agentState)
		if state.Done {
			return state, nil
		}
		if strings.ContainsAny(state.Question, "+-*/") {
			expr := extractArithExpr(state.Question)
			return agentState{
				Question:  state.Question,
				ToolInput: expr,
				Steps:     append(state.Steps, "decided to use calc for: "+expr),
			}, nil
		}
		return agentState{
			Question: state.Question,
			Steps:    state.Steps,
			Answer:   "42",
			Done:     true,
		}, nil
	})

	toolNode := core.NewRunnableFunc("tool", func(ctx context.Context, input any) (any, error) {
		state := input.(agentState)
		t, _ := reg.Get("calc")
		result, err := t.Invoke(ctx, state.ToolInput)
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

	agent := middleware.Chain(g, middleware.WithLogging(log.Logger), middleware.WithPanicRecovery())

	result, err := agent.Invoke(context.Background(), agentState{Question: question})
	if err != nil {
		return err
	}
	fmt.Printf("Question: %s\nAnswer: %v\n", question, result)
	return nil
}
