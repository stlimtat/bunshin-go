+++
title = 'Vector Search'
date = '2026-06-30'
draft = false
toc = true
weight = 7
+++

# Vector Search

`pkg/vector` provides semantic search over embedded documents. It is intentionally separate from `pkg/memory` (conversation history) even though both can share a pgvector-backed Postgres pool.

---

## Concepts

| Type | Role |
|------|------|
| `VectorStore` | Stores documents by vector + retrieves nearest neighbours |
| `Indexer` | Embeds raw text via `llm.Embedder` and upserts the result |
| `Document` | Unit of storage: ID, content string, `[]float32` vector, metadata |
| `SearchResult` | `Document` plus cosine similarity score in [0, 1] |

---

## VectorStore interface

```go
type VectorStore interface {
    Upsert(ctx context.Context, docs []Document) error
    Search(ctx context.Context, query []float32, topK int, filter map[string]any) ([]SearchResult, error)
    Delete(ctx context.Context, ids []string) error
}
```

`filter` maps to SQL WHERE on metadata columns — omitting it searches across all tenants. Always pass at least `{"tenant_id": id}` in multi-tenant deployments.

---

## Indexer interface

```go
type Indexer interface {
    Index(ctx context.Context, texts []string, metadata map[string]any) error
}
```

`Indexer` combines embedding + upsert into a single call. Use it when you want to index raw strings without managing vectors yourself.

---

## Quickstart

```go
import (
    "github.com/stlimtat/bunshin-go/pkg/vector"
    "github.com/stlimtat/bunshin-go/pkg/llm"
)

// 1. Build an embedder (any llm.Embedder implementation)
embedder, _ := llm.NewOpenAIEmbedder(llm.OpenAIConfig{APIKey: key})

// 2. Create an in-memory store (swap for Postgres in production)
store := vector.NewMemoryStore()

// 3. Index documents
indexer := vector.NewIndexer(store, embedder)
indexer.Index(ctx, []string{
    "Go goroutines are lightweight threads.",
    "Channels are the primary concurrency primitive in Go.",
}, map[string]any{"tenant_id": "acme", "source": "go-docs"})

// 4. Search
queryVec, _ := embedder.Embed(ctx, "concurrent programming")
results, _ := store.Search(ctx, queryVec, 3, map[string]any{"tenant_id": "acme"})
for _, r := range results {
    fmt.Printf("%.3f  %s\n", r.Score, r.Content)
}
```

---

## Backends

| Backend | Package | Notes |
|---------|---------|-------|
| In-memory | `vector.NewMemoryStore()` | Tests + single-process demos |
| Postgres/pgvector | `vector.NewPostgresStore(pool)` | Production; shares pool with `pkg/memory` |

---

## Relationship to pkg/memory

`pkg/memory` (`MessageStore`) stores conversation history keyed by thread. `pkg/vector` (`VectorStore`) stores arbitrary document embeddings for retrieval-augmented generation (RAG). Both can be backed by the same Postgres instance but use different tables and semantics.
