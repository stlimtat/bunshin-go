package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// promptFragment is the API representation of a stored prompt fragment.
type promptFragment struct {
	ID      string   `json:"id"`
	Slug    string   `json:"slug"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Version string   `json:"version"`
}

// promptListResponse is returned by GET /v1/prompts.
type promptListResponse struct {
	Fragments []promptFragment `json:"fragments"`
}

// readContent returns the contents of filePath, or reads from stdin when filePath is empty.
func readContent(filePath string) (string, error) {
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read file %q: %w", filePath, err)
		}
		return string(data), nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return string(data), nil
}

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
		RunE: func(cmd *cobra.Command, _ []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			client := newServerClient(server)
			var resp promptListResponse
			if err := client.getJSON("/v1/prompts", &resp); err != nil {
				return err
			}
			for _, f := range resp.Fragments {
				preview := f.Content
				if len(preview) > 40 {
					preview = preview[:40]
				}
				fmt.Printf("%-30s  %s\n", f.Slug, preview)
			}
			return nil
		},
	}
	cmd.Flags().String("tenant", "", "Filter by tenant ID")
	return cmd
}

func newPromptShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <slug>",
		Short: "Show fragment content and version history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			client := newServerClient(server)
			var frag promptFragment
			if err := client.getJSON("/v1/prompts/"+args[0], &frag); err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(frag)
		},
	}
}

func newPromptCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new fragment (starts as draft)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			name, _ := cmd.Flags().GetString("name")
			filePath, _ := cmd.Flags().GetString("file")
			content, err := readContent(filePath)
			if err != nil {
				return err
			}
			client := newServerClient(server)
			body := map[string]string{"content": content}
			var frag promptFragment
			if err := client.putJSON("/v1/prompts/"+name, body, &frag); err != nil {
				return err
			}
			fmt.Printf("prompt %q created\n", name)
			return nil
		},
	}
	cmd.Flags().String("name", "", "Fragment name/slug (required)")
	cmd.Flags().String("file", "", "Path to template file (reads stdin if omitted)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newPromptEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <slug>",
		Short: "Edit a draft fragment (creates a new version)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			content, err := readContent("")
			if err != nil {
				return err
			}
			client := newServerClient(server)
			body := map[string]string{"content": content}
			var frag promptFragment
			if err := client.putJSON("/v1/prompts/"+args[0], body, &frag); err != nil {
				return err
			}
			fmt.Printf("prompt %q updated\n", args[0])
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
		Use:   "delete <slug>",
		Short: "Soft delete a fragment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server, _ := cmd.Flags().GetString("server")
			if server == "" {
				server = viper.GetString("server")
			}
			client := newServerClient(server)
			if err := client.deleteHTTP("/v1/prompts/" + args[0]); err != nil {
				return err
			}
			fmt.Printf("prompt %q deleted\n", args[0])
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
