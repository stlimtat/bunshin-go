// hello-llm demonstrates how to create a typed LLM provider, send a message,
// and read back the response using the bunshin-go pkg/llm package.
//
// Usage:
//
//	OPENAI_API_KEY=sk-... go run ./examples/hello-llm
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY not set")
		os.Exit(1)
	}

	provider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
		APIKey:    key,
		Model:     "gpt-4o-mini",
		MaxTokens: 256,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider: %v\n", err)
		os.Exit(1)
	}

	registry := llm.NewProviderRegistry()
	registry.Register("openai-mini", provider, llm.Tag("vendor", string(llm.VendorOpenAI)))
	_ = registry

	req := &llm.Request{
		Messages: []llm.Message{
			llm.NewTextMessage(llm.RoleUser, "Say hello in three words."),
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invoke: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Response:", resp.Content)
}
