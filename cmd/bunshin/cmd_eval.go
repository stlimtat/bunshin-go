package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Manage and run eval suites",
	}
	cmd.AddCommand(
		newEvalListCmd(),
		newEvalShowCmd(),
		newEvalCreateCmd(),
		newEvalUpdateCmd(),
		newEvalDeleteCmd(),
		newEvalRunCmd(),
	)
	return cmd
}

func newEvalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List eval suites",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("eval list: not yet implemented")
			return nil
		},
	}
}

func newEvalShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show eval config and last results",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval show %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newEvalCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a new eval suite",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("eval create: not yet implemented")
			return nil
		},
	}
}

func newEvalUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <id>",
		Short: "Update eval configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval update %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newEvalDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft delete an eval suite",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newEvalRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Run an eval suite and stream results",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval run %q: not yet implemented\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("dataset", "", "Local JSONL dataset file (overrides LangSmith dataset)")
	mustBindFlag(cmd, "eval_dataset", "dataset")
	return cmd
}
