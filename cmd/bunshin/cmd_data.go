package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/vector"
)

// --- thread ---

func newThreadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread",
		Short: "Manage conversation threads (MessageStore)",
	}
	cmd.PersistentFlags().String("server", "http://localhost:8080", "bunshin server address")
	_ = viper.BindPFlag("server", cmd.PersistentFlags().Lookup("server"))

	cmd.AddCommand(
		newThreadListCmd(),
		newThreadShowCmd(),
		newThreadDeleteCmd(),
		newThreadExportCmd(),
	)
	return cmd
}

func newThreadListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List conversation threads (GET /v1/threads)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := viper.GetString("server")
			client := newServerClient(server)
			var result any
			if err := client.getJSON("/v1/threads", &result); err != nil {
				return err
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func newThreadShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show thread messages (GET /v1/threads/{id}/messages)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := viper.GetString("server")
			client := newServerClient(server)
			var result any
			if err := client.getJSON("/v1/threads/"+args[0]+"/messages", &result); err != nil {
				return err
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
}

func newThreadDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft delete a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("thread delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newThreadExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export thread messages as NDJSON or Markdown",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			server := viper.GetString("server")
			client := newServerClient(server)

			var result map[string]any
			if err := client.getJSON("/v1/threads/"+args[0]+"/messages", &result); err != nil {
				return err
			}

			msgs, _ := result["messages"].([]any)
			switch format {
			case "markdown":
				fmt.Printf("# Thread: %s\n\n", args[0])
				for _, m := range msgs {
					msg, _ := m.(map[string]any)
					role, _ := msg["role"].(string)
					content, _ := msg["content"].(string)
					fmt.Printf("**%s**\n\n%s\n\n---\n\n", role, content)
				}
			default: // ndjson
				for _, m := range msgs {
					line, _ := json.Marshal(m)
					fmt.Println(string(line))
				}
			}
			return nil
		},
	}
	cmd.Flags().String("format", "ndjson", "Output format: ndjson|markdown")
	return cmd
}

// --- memory ---

func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "MessageStore CRUD",
	}
	cmd.AddCommand(
		newMemoryListCmd(),
		newMemoryShowCmd(),
		newMemoryAppendCmd(),
		newMemoryDeleteCmd(),
	)
	return cmd
}

func newMemoryListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MessageStore entries for a thread",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("memory list: not yet implemented — use thread show <id> via the HTTP server")
			return nil
		},
	}
	cmd.Flags().String("thread", "", "Thread ID (required)")
	_ = cmd.MarkFlagRequired("thread")
	return cmd
}

func newMemoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show message detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("memory show %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newMemoryAppendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "append",
		Short: "Append a message to a thread",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("memory append: not yet implemented")
			return nil
		},
	}
	cmd.Flags().String("thread", "", "Thread ID (required)")
	cmd.Flags().String("role", "user", "Message role: user|assistant|system")
	cmd.Flags().String("content", "", "Message content (required)")
	_ = cmd.MarkFlagRequired("thread")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func newMemoryDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("memory delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

// --- vector ---

func newVectorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vector",
		Short: "VectorStore CRUD and semantic search",
	}
	cmd.AddCommand(
		newVectorListCmd(),
		newVectorSearchCmd(),
		newVectorUpsertCmd(),
		newVectorDeleteCmd(),
	)
	return cmd
}

func newVectorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List documents in VectorStore",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("vector list: not yet implemented")
			return nil
		},
	}
}

func newVectorSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Semantic search with optional filter",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("vector search %q: not yet implemented\n", args[0])
			return nil
		},
	}
	cmd.Flags().Int("top-k", 10, "Number of results to return")
	cmd.Flags().StringToString("filter", nil, "Metadata filter key=value pairs")
	return cmd
}

func newVectorUpsertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upsert",
		Short: "Upsert a document with its vector into VectorStore",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("vector upsert: not yet implemented")
			return nil
		},
	}
	cmd.Flags().String("id", "", "Document ID (required)")
	cmd.Flags().String("content", "", "Document text content (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func newVectorDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a document from VectorStore",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("vector delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

// --- embed ---

func newEmbedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Embed text and upsert into VectorStore (seed a datastore)",
	}
	cmd.AddCommand(newEmbedCreateCmd())
	return cmd
}

func newEmbedCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Embed text or file contents and upsert into VectorStore",
		Example: `  bunshin embed create --text "Hello world"
  bunshin embed create --file ./docs/corpus.txt`,
		RunE: runEmbedCreate,
	}
	cmd.Flags().String("text", "", "Text to embed (mutually exclusive with --file)")
	cmd.Flags().String("file", "", "Path to file whose contents will be embedded")
	cmd.Flags().StringToString("metadata", nil, "Metadata key=value pairs attached to the document")
	return cmd
}

func runEmbedCreate(cmd *cobra.Command, _ []string) error {
	text, _ := cmd.Flags().GetString("text")
	filePath, _ := cmd.Flags().GetString("file")
	metadata, _ := cmd.Flags().GetStringToString("metadata")

	if text == "" && filePath == "" {
		return fmt.Errorf("one of --text or --file is required")
	}
	if text != "" && filePath != "" {
		return fmt.Errorf("--text and --file are mutually exclusive")
	}
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		text = string(data)
	}

	cfg := loadConfig()
	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}

	embedder, ok := provider.(llm.Embedder)
	if !ok {
		return fmt.Errorf("provider %q does not support embeddings; use --provider openai or a provider that implements Embedder", cfg.Provider)
	}

	vecs, err := embedder.Embed(context.Background(), []string{text})
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	store := vector.NewMemoryVectorStore()
	doc := vector.Document{
		ID:       "cli-embed-" + fmt.Sprintf("%d", len(text)),
		Content:  text,
		Vector:   vecs[0],
		Metadata: make(map[string]any, len(metadata)),
	}
	for k, v := range metadata {
		doc.Metadata[k] = v
	}

	if err := store.Upsert(context.Background(), []vector.Document{doc}); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	preview := text
	if len(preview) > 80 {
		preview = preview[:80]
	}
	out, _ := json.MarshalIndent(map[string]any{
		"id":         doc.ID,
		"vector_dim": len(vecs[0]),
		"content":    preview,
	}, "", "  ")
	fmt.Println(string(out))
	return nil
}
