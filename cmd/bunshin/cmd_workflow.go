package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage and run registered workflows (Graph and Chain)",
	}
	cmd.PersistentFlags().String("server", "http://localhost:8080", "bunshin server address")
	_ = viper.BindPFlag("server", cmd.PersistentFlags().Lookup("server"))

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
			fmt.Println("workflow list: not yet implemented — use bunshin serve + GET /v1/workflows")
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
		Short: "Run a workflow with JSON input via the bunshin HTTP server",
		Example: `  # Run workflow "chat" with a JSON payload
  bunshin workflow run chat --input '{"message":"hello"}'

  # Against a remote server
  bunshin workflow run chat --server http://prod.example.com:8080 --input '{}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			inputStr, _ := cmd.Flags().GetString("input")
			server := viper.GetString("server")

			var inputBody any = map[string]any{}
			if inputStr != "" {
				if err := json.Unmarshal([]byte(inputStr), &inputBody); err != nil {
					return fmt.Errorf("--input is not valid JSON: %w", err)
				}
			}

			client := newServerClient(server)
			var result any
			if err := client.postJSON("/v1/workflows/"+id, inputBody, &result); err != nil {
				return err
			}

			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		},
	}
	cmd.Flags().String("input", "", "JSON input for the workflow")
	cmd.Flags().String("server", "http://localhost:8080", "bunshin server address")
	mustBindFlag(cmd, "workflow_input", "input")
	return cmd
}
