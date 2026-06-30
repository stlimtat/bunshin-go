// concurrent/providers calls OpenAI and Anthropic simultaneously using
// two goroutines and one errgroup. Wall-clock time proves they ran in parallel.
//
// Usage:
//
//	OPENAI_API_KEY=sk-... ANTHROPIC_API_KEY=sk-ant-... go run ./examples/concurrent/providers
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
	openaiKey := os.Getenv("OPENAI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if openaiKey == "" || anthropicKey == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY and ANTHROPIC_API_KEY must be set")
		os.Exit(1)
	}

	openai, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
		APIKey: openaiKey,
		Model:  "gpt-4o-mini",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "openai: %v\n", err)
		os.Exit(1)
	}

	anthropic, err := llm.NewAnthropicProvider(llm.AnthropicConfig{
		APIKey: anthropicKey,
		Model:  "claude-haiku-4-5-20251001",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "anthropic: %v\n", err)
		os.Exit(1)
	}

	prompt := "Name one advantage of Go over Python. One sentence."
	req := &llm.Request{
		Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)},
	}

	var openaiResp, anthropicResp string
	g, ctx := errgroup.WithContext(context.Background())

	start := time.Now()

	g.Go(func() error {
		r, err := openai.Complete(ctx, req)
		if err != nil {
			return fmt.Errorf("openai: %w", err)
		}
		openaiResp = r.Content
		return nil
	})

	g.Go(func() error {
		r, err := anthropic.Complete(ctx, req)
		if err != nil {
			return fmt.Errorf("anthropic: %w", err)
		}
		anthropicResp = r.Content
		return nil
	})

	if err := g.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	fmt.Printf("OpenAI   (gpt-4o-mini):          %s\n", openaiResp)
	fmt.Printf("Anthropic (claude-haiku): %s\n", anthropicResp)
	fmt.Printf("Wall-clock time: %s (sequential would be ~2×)\n", elapsed.Round(time.Millisecond))
}
