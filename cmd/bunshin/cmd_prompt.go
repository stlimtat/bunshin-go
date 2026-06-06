package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Manage prompt fragments and run them as agent loops",
	}
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
			fmt.Println("prompt list: not yet implemented")
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
	return &cobra.Command{
		Use:   "activate <id> <version>",
		Short: "Roll forward to a specific version (roll-forward only)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("prompt activate %q version=%q: not yet implemented\n", args[0], args[1])
			return nil
		},
	}
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
		Short: "Force prompt cache refresh on this node",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("prompt refresh: not yet implemented")
			return nil
		},
	}
	cmd.Flags().String("addr", "", "Remote node address (default: local)")
	return cmd
}

func newPromptRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Run a prompt fragment as an agent loop",
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
