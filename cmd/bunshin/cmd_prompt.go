package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newPromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Manage prompt fragments and run them as agent loops",
	}
	cmd.PersistentFlags().String("server", "http://localhost:8080", "bunshin server address")
	_ = viper.BindPFlag("server", cmd.PersistentFlags().Lookup("server"))

	cmd.AddCommand(
		newPromptListCmd(),
		newPromptShowCmd(),
		newPromptCreateCmd(),
		newPromptEditCmd(),
		newPromptActivateCmd(),
		newPromptDeleteCmd(),
		newPromptRefreshCmd(),
		newPromptRunCmd(),
	)
	return cmd
}

func newPromptListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List fragments and their active versions",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("prompt list: not yet implemented — manage via PostgresStore or MemoryBackend directly")
			return nil
		},
	}
	cmd.Flags().String("tenant", "", "Filter by tenant ID")
	return cmd
}

func newPromptShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show fragment content and version history",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("prompt show %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newPromptCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new fragment (starts as draft)",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("prompt create: not yet implemented")
			return nil
		},
	}
	cmd.Flags().String("name", "", "Fragment name (required)")
	cmd.Flags().String("file", "", "Path to template file")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newPromptEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a draft fragment (creates a new version)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("prompt edit %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newPromptActivateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <name>",
		Short: "Roll forward the newest draft to active (POST /v1/prompts/{name}/activate)",
		Example: `  bunshin prompt activate my-system-prompt
  bunshin prompt activate my-system-prompt --server http://prod:8080`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			client := newServerClient(server)
			if err := client.postJSON("/v1/prompts/"+args[0]+"/activate", nil, nil); err != nil {
				return err
			}
			fmt.Printf("prompt %q activated\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("server", "http://localhost:8080", "bunshin server address")
	return cmd
}

func newPromptDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft delete a fragment",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("prompt delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newPromptRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Force prompt cache refresh on the target node (POST /v1/prompts/refresh)",
		Example: `  # Refresh this node (default)
  bunshin prompt refresh

  # Refresh a specific remote node
  bunshin prompt refresh --server http://node2:8080`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			client := newServerClient(server)
			if err := client.postJSON("/v1/prompts/refresh", nil, nil); err != nil {
				return err
			}
			fmt.Printf("prompt cache refresh requested on %s\n", server)
			return nil
		},
	}
	cmd.Flags().String("server", "http://localhost:8080", "bunshin server address")
	return cmd
}

func newPromptRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Render a prompt fragment with variables",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("prompt run %q: not yet implemented\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("question", "", "Input question for the agent")
	mustBindFlag(cmd, "prompt_question", "question")
	return cmd
}
