package vector

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// MemoryIndexer embeds texts using an llm.Embedder and upserts them into a VectorStore.
type MemoryIndexer struct {
	embedder llm.Embedder
	store    VectorStore
}

// NewMemoryIndexer returns an Indexer that uses embedder to vectorise texts
// and stores results in store.
func NewMemoryIndexer(embedder llm.Embedder, store VectorStore) *MemoryIndexer {
	return &MemoryIndexer{embedder: embedder, store: store}
}

func (m *MemoryIndexer) Index(ctx context.Context, texts []string, metadata map[string]any) error {
	vectors, err := m.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	docs := make([]Document, len(texts))
	for i, text := range texts {
		docs[i] = Document{
			ID:       fmt.Sprintf("doc-%d", i),
			Content:  text,
			Vector:   vectors[i],
			Metadata: metadata,
		}
	}
	return m.store.Upsert(ctx, docs)
}
