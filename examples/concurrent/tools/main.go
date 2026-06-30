// concurrent/tools fires three independent tools in parallel using errgroup,
// then collects results. No LLM needed — shows how a tool dispatcher
// (e.g. an agent node) can fan out tool calls concurrently.
//
// Usage:
//
//	go run ./examples/concurrent/tools
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func main() {
	registry := tools.NewToolRegistry()

	registry.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "upper", Description: "Uppercase a string"},
		func(_ context.Context, input any) (any, error) {
			time.Sleep(50 * time.Millisecond) // simulate I/O
			return strings.ToUpper(fmt.Sprint(input)), nil
		},
	))
	registry.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "reverse", Description: "Reverse a string"},
		func(_ context.Context, input any) (any, error) {
			time.Sleep(80 * time.Millisecond)
			s := fmt.Sprint(input)
			r := []rune(s)
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
			return string(r), nil
		},
	))
	registry.MustRegister(tools.NewFuncTool(
		tools.ToolSchema{Name: "shout", Description: "Append exclamation marks"},
		func(_ context.Context, input any) (any, error) {
			time.Sleep(60 * time.Millisecond)
			return fmt.Sprint(input) + "!!!", nil
		},
	))

	// Simulate an agent deciding to call all three tools at once.
	calls := []struct {
		tool  string
		input string
	}{
		{"upper", "hello bunshin"},
		{"reverse", "hello bunshin"},
		{"shout", "hello bunshin"},
	}

	results := make([]string, len(calls))
	g, ctx := errgroup.WithContext(context.Background())

	start := time.Now()

	for i, c := range calls {
		i, c := i, c
		g.Go(func() error {
			t, err := registry.Get(c.tool)
			if err != nil {
				return err
			}
			out, err := t.Invoke(ctx, c.input)
			if err != nil {
				return fmt.Errorf("%s: %w", c.tool, err)
			}
			results[i] = fmt.Sprintf("%s(%q) → %v", c.tool, c.input, out)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Printf("\nAll tools in %s (slowest was 80ms; sequential would be 190ms)\n",
		elapsed.Round(time.Millisecond))
}
