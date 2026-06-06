// Command bunshin is the CLI entry point for bunshin-go.
//
// # Subcommands
//
//	bunshin serve           Start the HTTP workflow server
//	bunshin healthz <addr>  Check health/readiness of a remote node
//	bunshin pprof <addr>    Fetch and open pprof from a remote node
//	bunshin version         Print version information
//	bunshin docs            Generate CLI documentation (markdown)
//	bunshin llm             Single LLM call demo
//	bunshin mcp-sandbox     MCP tool discovery + sandboxed code execution demo
//	bunshin workflow        Workflow CRUD and run
//	bunshin prompt          Prompt fragment CRUD, lifecycle, and run
//	bunshin eval            Eval suite CRUD and run
//	bunshin thread          Conversation thread management
//	bunshin memory          MessageStore CRUD
//	bunshin vector          VectorStore CRUD and search
//	bunshin embed           Embed text and upsert into VectorStore
//
// # Global flags (all subcommands)
//
//	--provider   LLM provider instance ID (BUNSHIN_PROVIDER)
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

	pf := root.PersistentFlags()
	pf.String("provider", "fake", "LLM provider instance ID")
	pf.String("model", "", "Model ID (default: provider-specific)")
	pf.String("api-key", "", "API key for the chosen provider")
	pf.String("log-level", "info", "Log level: debug|info|warn|error")

	mustBindPersistentPFlag(root, "provider", "provider")
	mustBindPersistentPFlag(root, "model", "model")
	mustBindPersistentPFlag(root, "api_key", "api-key")
	mustBindPersistentPFlag(root, "log_level", "log-level")

	root.AddCommand(
		newServeCmd(),
		newHealthzCmd(),
		newPprofCmd(),
		newVersionCmd(),
		newDocsCmd(),
		newLLMCmd(),
		newMCPSandboxCmd(),
		newWorkflowCmd(),
		newPromptCmd(),
		newEvalCmd(),
		newThreadCmd(),
		newMemoryCmd(),
		newVectorCmd(),
		newEmbedCmd(),
	)
	return root
}

// mustBindPersistentPFlag binds a persistent flag to a viper key; panics on error.
func mustBindPersistentPFlag(cmd *cobra.Command, viperKey, flagName string) {
	if err := viper.BindPFlag(viperKey, cmd.PersistentFlags().Lookup(flagName)); err != nil {
		panic(fmt.Sprintf("viper bind persistent %s→%s: %v", flagName, viperKey, err))
	}
}
