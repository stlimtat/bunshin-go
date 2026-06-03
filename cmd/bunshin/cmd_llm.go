package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func newLLMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm",
		Short: "Send a single message to an LLM provider",
		Long: `Demonstrates the simplest possible bunshin-go usage: one LLM call.

Selects the provider from --provider (or BUNSHIN_PROVIDER).
With --provider fake (default) no API key is needed and the response is canned.
Set --provider openai --api-key $OPENAI_API_KEY for a real call.`,
		Example: `  # Fake provider (no key needed)
  bunshin llm --message "What is Go?"

  # OpenAI (once adapter is implemented)
  bunshin llm --provider openai --api-key $OPENAI_API_KEY --model gpt-4o-mini \
    --message "What is Go?"

  # Via environment variables
  BUNSHIN_PROVIDER=openai BUNSHIN_API_KEY=sk-... bunshin llm --message "What is Go?"`,
		RunE: runLLM,
	}
	cmd.Flags().String("message", "What is Go?", "User message to send to the LLM")
	mustBindFlag(cmd, "message", "message")
	return cmd
}

func runLLM(_ *cobra.Command, _ []string) error {
	cfg := loadConfig()
	message := viper.GetString("message")

	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}

	req := &llm.Request{
		Messages: []llm.Message{
			llm.NewTextMessage(llm.RoleSystem, "You are a helpful assistant."),
			llm.NewTextMessage(llm.RoleUser, message),
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	fmt.Println(resp.Content)
	return nil
}
