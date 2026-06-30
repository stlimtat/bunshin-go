// concurrent/docs summarises five documents in parallel using one goroutine
// per document. Total time is bounded by the slowest request, not the sum.
//
// Usage:
//
//	OPENAI_API_KEY=sk-...  go run ./examples/concurrent/docs
//	GOOGLE_API_KEY=AIza... go run ./examples/concurrent/docs
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

var documents = []string{
	"Go is a statically typed, compiled language designed at Google. It emphasises simplicity, concurrency, and fast build times.",
	"Goroutines are lightweight threads managed by the Go runtime. You can run thousands of them concurrently with minimal memory overhead.",
	"The Go standard library includes packages for HTTP servers, JSON encoding, cryptography, and file I/O — batteries included.",
	"Go modules replaced GOPATH in Go 1.11. A go.mod file declares the module path and dependency versions, enabling reproducible builds.",
	"Interfaces in Go are satisfied implicitly. Any type that implements the required methods satisfies the interface — no 'implements' keyword.",
}

func main() {
	provider, err := providerFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider: %v\n", err)
		os.Exit(1)
	}

	summaries := make([]string, len(documents))
	g, ctx := errgroup.WithContext(context.Background())

	start := time.Now()

	for i, doc := range documents {
		i, doc := i, doc
		g.Go(func() error {
			r, err := provider.Complete(ctx, &llm.Request{
				Messages: []llm.Message{
					llm.NewTextMessage(llm.RoleUser, "Summarise in one sentence: "+doc),
				},
			})
			if err != nil {
				return fmt.Errorf("doc %d: %w", i, err)
			}
			summaries[i] = r.Content
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	for i, s := range summaries {
		fmt.Printf("[%d] %s\n", i+1, s)
	}
	fmt.Printf("\n%d docs in %s (sequential would be ~%s)\n",
		len(documents), elapsed.Round(time.Millisecond),
		(elapsed * time.Duration(len(documents))).Round(time.Millisecond))
}

func providerFromEnv() (llm.LLMProvider, error) {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o-mini", MaxTokens: 64})
	}
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		return llm.NewGoogleProvider(llm.GoogleConfig{APIKey: key, Model: "gemini-2.0-flash", MaxTokens: 64})
	}
	fmt.Fprintln(os.Stderr, "set OPENAI_API_KEY or GOOGLE_API_KEY")
	os.Exit(1)
	return nil, nil
}
