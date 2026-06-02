// hello-llm demonstrates the simplest possible bunshin-go usage:
// a single LLM call with minimal configuration.
//
// Run:
//
//	OPENAI_API_KEY=... go run ./examples/hello-llm
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY not set — using fake provider for demo")
	}

	// Use the fake provider for demo purposes.
	// Replace with openai.New(openai.Config{APIKey: apiKey, Model: "gpt-4o-mini"})
	// once the OpenAI adapter is implemented.
	provider := llm.NewFakeProvider(llm.ProviderOpenAI, "Go is a statically typed, compiled programming language designed for simplicity and efficiency.")

	req := &llm.Request{
		Messages: []llm.Message{
			llm.NewTextMessage(llm.RoleSystem, "You are a helpful assistant."),
			llm.NewTextMessage(llm.RoleUser, "What is Go?"),
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Content)
}
