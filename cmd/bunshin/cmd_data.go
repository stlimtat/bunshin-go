package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// --- thread ---

func newThreadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread",
		Short: "Manage conversation threads (MessageStore)",
	}
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
		Short: "List conversation threads",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("thread list: not yet implemented")
			return nil
		},
	}
}

func newThreadShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show thread messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("thread show %q: not yet implemented\n", args[0])
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
		Short: "Export thread as NDJSON or Markdown",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("thread export %q: not yet implemented\n", args[0])
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
			fmt.Println("memory list: not yet implemented")
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
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("embed create: not yet implemented")
			return nil
		},
	}
	cmd.Flags().String("text", "", "Text to embed (mutually exclusive with --file)")
	cmd.Flags().String("file", "", "Path to file whose contents will be embedded")
	cmd.Flags().StringToString("metadata", nil, "Metadata key=value pairs attached to the document")
	return cmd
}
