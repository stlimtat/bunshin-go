// concurrent/models calls two model tiers simultaneously using one API key.
// Defaults to OpenAI (gpt-4o-mini + gpt-4o); falls back to Google
// (gemini-2.0-flash-lite + gemini-2.0-flash) if OPENAI_API_KEY is unset.
//
// Usage:
//
//	OPENAI_API_KEY=sk-...  go run ./examples/concurrent/models
//	GOOGLE_API_KEY=AIza... go run ./examples/concurrent/models
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
	fast, smart, label, err := providersFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider: %v\n", err)
		os.Exit(1)
	}

	prompt := "Name one advantage of Go over Python. One sentence."

	var fastResp, smartResp string
	g, ctx := errgroup.WithContext(context.Background())

	start := time.Now()

	g.Go(func() error {
		r, err := fast.Complete(ctx, &llm.Request{
			Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)},
		})
		if err != nil {
			return fmt.Errorf("%s fast: %w", label, err)
		}
		fastResp = r.Content
		return nil
	})

	g.Go(func() error {
		r, err := smart.Complete(ctx, &llm.Request{
			Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)},
		})
		if err != nil {
			return fmt.Errorf("%s smart: %w", label, err)
		}
		smartResp = r.Content
		return nil
	})

	if err := g.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	fmt.Printf("[fast]  %s\n", fastResp)
	fmt.Printf("[smart] %s\n", smartResp)
	fmt.Printf("Wall-clock: %s (sequential would be ~2×)\n", elapsed.Round(time.Millisecond))
}

func providersFromEnv() (fast, smart llm.LLMProvider, label string, err error) {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		f, e := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o-mini"})
		if e != nil {
			return nil, nil, "", e
		}
		s, e := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o"})
		if e != nil {
			return nil, nil, "", e
		}
		return f, s, "openai", nil
	}
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		f, e := llm.NewGoogleProvider(llm.GoogleConfig{APIKey: key, Model: "gemini-2.0-flash-lite"})
		if e != nil {
			return nil, nil, "", e
		}
		s, e := llm.NewGoogleProvider(llm.GoogleConfig{APIKey: key, Model: "gemini-2.0-flash"})
		if e != nil {
			return nil, nil, "", e
		}
		return f, s, "google", nil
	}
	fmt.Fprintln(os.Stderr, "set OPENAI_API_KEY or GOOGLE_API_KEY")
	os.Exit(1)
	return nil, nil, "", nil
}
