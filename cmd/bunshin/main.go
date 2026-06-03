// Command bunshin is the CLI entry point for bunshin-go.
//
// # Subcommands
//
//	bunshin serve         Start the HTTP workflow server
//	bunshin health        Check server health
//	bunshin version       Print version information
//	bunshin docs          Generate CLI documentation (markdown)
//	bunshin llm           Single LLM call demo
//	bunshin chain         Two-step entity extraction chain demo
//	bunshin agent         Agent loop with tools demo
//	bunshin mcp-sandbox   MCP tool discovery + sandboxed code execution demo
//
// # Global flags (all subcommands)
//
//	--provider   LLM provider: fake|openai|anthropic|google|ollama  (BUNSHIN_PROVIDER)
//	--model      Model ID (BUNSHIN_MODEL)
//	--api-key    API key for the chosen provider (BUNSHIN_API_KEY)
//	--log-level  Log level: debug|info|warn|error (BUNSHIN_LOG_LEVEL)
//
// All flags can be set via environment variables prefixed with BUNSHIN_.
// Flag form --foo-bar maps to env BUNSHIN_FOO_BAR.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const version = "0.1.0"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "bunshin",
		Short: "bunshin-go — Go LangChain/LangGraph/LangSmith clone",
		Long: `bunshin-go is a production-grade Go library for building LLM workflows.

All configuration can be provided via flags or BUNSHIN_* environment variables.
Run "bunshin docs" to generate full CLI documentation in markdown.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			viper.SetEnvPrefix("BUNSHIN")
			viper.AutomaticEnv()
			return nil
		},
	}

	// Persistent flags — inherited by every subcommand.
	pf := root.PersistentFlags()
	pf.String("provider", "fake", "LLM provider: fake|openai|anthropic|google|ollama")
	pf.String("model", "", "Model ID (default: provider-specific)")
	pf.String("api-key", "", "API key for the chosen provider")
	pf.String("log-level", "info", "Log level: debug|info|warn|error")

	mustBindPersistentPFlag(root, "provider", "provider")
	mustBindPersistentPFlag(root, "model", "model")
	mustBindPersistentPFlag(root, "api_key", "api-key")
	mustBindPersistentPFlag(root, "log_level", "log-level")

	root.AddCommand(
		newServeCmd(),
		newHealthCmd(),
		newVersionCmd(),
		newDocsCmd(),
		newLLMCmd(),
		newChainCmd(),
		newAgentCmd(),
		newMCPSandboxCmd(),
	)
	return root
}

// mustBindPersistentPFlag binds a persistent flag to a viper key; panics on error.
func mustBindPersistentPFlag(cmd *cobra.Command, viperKey, flagName string) {
	if err := viper.BindPFlag(viperKey, cmd.PersistentFlags().Lookup(flagName)); err != nil {
		panic(fmt.Sprintf("viper bind persistent %s→%s: %v", flagName, viperKey, err))
	}
}
