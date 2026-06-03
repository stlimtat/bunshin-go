package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"
)

func newDocsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate CLI documentation in markdown",
		Long: `Generates one markdown file per subcommand under --output-dir.

The generated files are structured for LLM consumption: each file contains
the command's synopsis, description, flag reference, and usage examples.
Feed the output directory to your RAG pipeline or paste into a context window.

Files produced:
  bunshin.md
  bunshin_serve.md
  bunshin_health.md
  bunshin_llm.md
  bunshin_chain.md
  bunshin_agent.md
  bunshin_mcp-sandbox.md
  ... (one per subcommand)`,
		Example: `  bunshin docs
  bunshin docs --output-dir ./docs/cli`,
		RunE: runDocs,
	}
	cmd.Flags().String("output-dir", "./docs/cli", "Directory to write markdown files into")
	mustBindFlag(cmd, "docs_output_dir", "output-dir")
	return cmd
}

func runDocs(cmd *cobra.Command, _ []string) error {
	outputDir := viper.GetString("docs_output_dir")
	if outputDir == "" {
		outputDir = "./docs/cli"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}
	root := cmd.Root()
	if err := doc.GenMarkdownTree(root, outputDir); err != nil {
		return fmt.Errorf("generate docs: %w", err)
	}
	fmt.Printf("CLI docs written to %s/\n", outputDir)
	return nil
}
