// llm demonstrates how to create a typed LLM provider, send a message,
// and read back the response using the bunshin-go pkg/llm package.
//
// Usage:
//
//	OPENAI_API_KEY=sk-...        go run ./examples/llm
//	GOOGLE_API_KEY=AIza...       go run ./examples/llm
//	ANTHROPIC_API_KEY=sk-ant-... go run ./examples/llm
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
	provider, err := providerFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider: %v\n", err)
		os.Exit(1)
	}

	resp, err := provider.Complete(context.Background(), &llm.Request{
		Messages: []llm.Message{
			llm.NewTextMessage(llm.RoleUser, "Say hello in three words."),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invoke: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Response:", resp.Content)
}

// providerFromEnv picks the first available provider from environment variables.
func providerFromEnv() (llm.LLMProvider, error) {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o-mini", MaxTokens: 256})
	}
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		return llm.NewGoogleProvider(llm.GoogleConfig{APIKey: key, Model: "gemini-2.0-flash", MaxTokens: 256})
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return llm.NewAnthropicProvider(llm.AnthropicConfig{APIKey: key, Model: "claude-haiku-4-5-20251001", MaxTokens: 256})
	}
	fmt.Fprintln(os.Stderr, "set OPENAI_API_KEY, GOOGLE_API_KEY, or ANTHROPIC_API_KEY")
	os.Exit(1)
	return nil, nil
}
