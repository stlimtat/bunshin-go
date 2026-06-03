package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// Config holds resolved runtime configuration for all bunshin subcommands.
// Populated from flags (highest priority) → BUNSHIN_* env vars → defaults.
type Config struct {
	Provider string // fake | openai | anthropic | google | ollama
	Model    string // model ID; empty = provider default
	APIKey   string // BUNSHIN_API_KEY
	LogLevel string // debug | info | warn | error
	Addr     string // HTTP listen address (serve subcommand)
}

// loadConfig reads all values from viper into a Config.
// Call after cobra flag parsing and PersistentPreRunE have run.
func loadConfig() Config {
	return Config{
		Provider: viper.GetString("provider"),
		Model:    viper.GetString("model"),
		APIKey:   viper.GetString("api_key"),
		LogLevel: viper.GetString("log_level"),
		Addr:     viper.GetString("addr"),
	}
}

// newProvider constructs an LLMProvider from cfg.
// Only ProviderFake is implemented today; other providers return a clear error
// so callers get actionable feedback rather than a panic.
func newProvider(cfg Config) (llm.LLMProvider, error) {
	switch cfg.Provider {
	case "", string(llm.ProviderFake):
		return llm.NewFakeProvider(
			llm.ProviderFake,
			"This is a fake response. Use --provider and --api-key to call a real LLM.",
		), nil
	case string(llm.ProviderOpenAI),
		string(llm.ProviderAnthropic),
		string(llm.ProviderGoogle),
		string(llm.ProviderOllama):
		return nil, fmt.Errorf(
			"provider %q adapter not yet implemented — contribution welcome at github.com/stlimtat/bunshin-go",
			cfg.Provider,
		)
	default:
		return nil, fmt.Errorf(
			"unknown provider %q; valid choices: fake, openai, anthropic, google, ollama",
			cfg.Provider,
		)
	}
}

// mustBindFlag binds a local flag to a viper key; panics on error (programming error).
func mustBindFlag(cmd *cobra.Command, viperKey, flagName string) {
	if err := viper.BindPFlag(viperKey, cmd.Flags().Lookup(flagName)); err != nil {
		panic(fmt.Sprintf("viper bind %s→%s: %v", flagName, viperKey, err))
	}
}
