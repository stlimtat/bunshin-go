// Package vector provides semantic search over embedded documents.
//
// VectorStore is intentionally separate from MessageStore: the two have
// different semantics (semantic retrieval vs conversation history) but may
// share a pgvector-backed Postgres connection pool.
//
// Document vectors are produced by an llm.Embedder before upsert.
// Filter in Search is required for per-tenant and per-thread scoping.
package vector

import "context"

// VectorStore stores and retrieves documents by semantic similarity.
type VectorStore interface {
	// Upsert inserts or updates documents. Documents with existing IDs are replaced.
	Upsert(ctx context.Context, docs []Document) error

	// Search returns the topK documents nearest to query, restricted by filter.
	// Filter maps to SQL WHERE on metadata columns — omitting it searches all tenants.
	Search(ctx context.Context, query []float32, topK int, filter map[string]any) ([]SearchResult, error)

	// Delete removes documents with the given IDs.
	Delete(ctx context.Context, ids []string) error
}

// Indexer embeds raw text and upserts the resulting Document into a VectorStore.
type Indexer interface {
	// Index embeds texts and upserts them as Documents with the given metadata.
	Index(ctx context.Context, texts []string, metadata map[string]any) error
}
