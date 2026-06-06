package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage and run registered workflows (Graph and Chain)",
	}
	cmd.AddCommand(
		newWorkflowListCmd(),
		newWorkflowShowCmd(),
		newWorkflowCreateCmd(),
		newWorkflowUpdateCmd(),
		newWorkflowDeleteCmd(),
		newWorkflowRunCmd(),
	)
	return cmd
}

func newWorkflowListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered workflows",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("workflow list: not yet implemented")
			return nil
		},
	}
}

func newWorkflowShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show workflow definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("workflow show %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newWorkflowCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Register a new workflow",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("workflow create: not yet implemented")
			return nil
		},
	}
}

func newWorkflowUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <id>",
		Short: "Update workflow configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("workflow update %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newWorkflowDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft delete a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("workflow delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Run a workflow with input",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("workflow run %q: not yet implemented\n", args[0])
			return nil
		},
	}
	cmd.Flags().String("input", "", "JSON input for the workflow")
	mustBindFlag(cmd, "workflow_input", "input")
	return cmd
}
