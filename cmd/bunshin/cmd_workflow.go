package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage and run YAML-defined workflows",
	}
	cmd.PersistentFlags().String("server", "http://localhost:8080", "bunshin server address")
	_ = viper.BindPFlag("server", cmd.PersistentFlags().Lookup("server"))

	cmd.AddCommand(
		newWorkflowListCmd(),
		newWorkflowShowCmd(),
		newWorkflowVersionsCmd(),
		newWorkflowCreateCmd(),
		newWorkflowActivateCmd(),
		newWorkflowDeleteCmd(),
		newWorkflowRunCmd(),
	)
	return cmd
}

func newWorkflowListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List workflow names (GET /v1/workflows)",
		RunE: func(_ *cobra.Command, _ []string) error {
			client := newServerClient(viper.GetString("server"))
			var result map[string]any
			if err := client.getJSON("/v1/workflows", &result); err != nil {
				return err
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func newWorkflowShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show active workflow spec (GET /v1/workflows/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			client := newServerClient(viper.GetString("server"))
			var result any
			if err := client.getJSON("/v1/workflows/"+args[0], &result); err != nil {
				return err
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func newWorkflowVersionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "versions <name>",
		Short: "List all versions of a workflow (GET /v1/workflows/{name}/versions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			client := newServerClient(viper.GetString("server"))
			var result any
			if err := client.getJSON("/v1/workflows/"+args[0]+"/versions", &result); err != nil {
				return err
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func newWorkflowCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new workflow draft from a YAML file or stdin",
		Example: `  bunshin workflow create --file my-workflow.yaml
  cat my-workflow.yaml | bunshin workflow create`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			var specBytes []byte
			if filePath != "" {
				var err error
				specBytes, err = os.ReadFile(filePath)
				if err != nil {
					return fmt.Errorf("read file: %w", err)
				}
			} else {
				var err error
				specBytes, err = os.ReadFile("/dev/stdin")
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
			}

			client := newServerClient(viper.GetString("server"))
			body := map[string]string{"spec": string(specBytes)}
			var result map[string]string
			if err := client.postJSON("/v1/workflows", body, &result); err != nil {
				return err
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
	cmd.Flags().String("file", "", "Path to YAML workflow spec file (reads stdin when omitted)")
	return cmd
}

func newWorkflowActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate <name> <version>",
		Short: "Promote a version to active (POST /v1/workflows/{name}/activate)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			client := newServerClient(viper.GetString("server"))
			body := map[string]string{"version": args[1]}
			if err := client.postJSON("/v1/workflows/"+args[0]+"/activate", body, nil); err != nil {
				return err
			}
			fmt.Printf("workflow %q version %q activated\n", args[0], args[1])
			return nil
		},
	}
}

func newWorkflowDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Soft-delete a workflow (DELETE /v1/workflows/{name})",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			client := newServerClient(viper.GetString("server"))
			if err := client.deleteHTTP("/v1/workflows/" + args[0]); err != nil {
				return err
			}
			fmt.Printf("workflow %q deleted\n", args[0])
			return nil
		},
	}
}

func newWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a workflow via the bunshin HTTP server",
		Example: `  bunshin workflow run my-flow --input '{"msg":"hello"}'
  bunshin workflow run my-flow --server http://prod:8080 --input '{}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			inputStr, _ := cmd.Flags().GetString("input")

			var inputBody any = map[string]any{}
			if inputStr != "" {
				if err := json.Unmarshal([]byte(inputStr), &inputBody); err != nil {
					return fmt.Errorf("--input is not valid JSON: %w", err)
				}
			}

			client := newServerClient(viper.GetString("server"))
			var result any
			if err := client.postJSON("/v1/workflows/"+name+"/invoke", inputBody, &result); err != nil {
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
