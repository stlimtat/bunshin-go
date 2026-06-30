// concurrent/models calls gpt-4o-mini and gpt-4o simultaneously with one
// OpenAI API key. Shows goroutine fan-out across model tiers.
//
// Usage:
//
//	OPENAI_API_KEY=sk-... go run ./examples/concurrent/models
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY not set")
		os.Exit(1)
	}

	fast, err := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o-mini"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fast provider: %v\n", err)
		os.Exit(1)
	}
	smart, err := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "smart provider: %v\n", err)
		os.Exit(1)
	}

	prompt := "Name one advantage of Go over Python. One sentence."
	req := &llm.Request{
		Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)},
	}

	var fastResp, smartResp string
	g, ctx := errgroup.WithContext(context.Background())

	start := time.Now()

	g.Go(func() error {
		r, err := fast.Complete(ctx, req)
		if err != nil {
			return fmt.Errorf("gpt-4o-mini: %w", err)
		}
		fastResp = r.Content
		return nil
	})

	g.Go(func() error {
		r, err := smart.Complete(ctx, req)
		if err != nil {
			return fmt.Errorf("gpt-4o: %w", err)
		}
		smartResp = r.Content
		return nil
	})

	if err := g.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	fmt.Printf("gpt-4o-mini: %s\n", fastResp)
	fmt.Printf("gpt-4o:      %s\n", smartResp)
	fmt.Printf("Wall-clock time: %s (sequential would be ~2×)\n", elapsed.Round(time.Millisecond))
}
