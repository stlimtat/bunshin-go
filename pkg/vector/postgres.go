package vector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// PostgresVectorStore implements VectorStore using pgvector for ANN search.
//
// Requires the pgvector extension and the bunshin_vectors table.
//
// Schema (create once per database):
//
//	CREATE EXTENSION IF NOT EXISTS vector;
//
//	CREATE TABLE IF NOT EXISTS bunshin_vectors (
//	    id          TEXT       PRIMARY KEY,
//	    tenant_id   TEXT       NOT NULL,
//	    content     TEXT       NOT NULL,
//	    embedding   vector(1536),
//	    metadata    JSONB      DEFAULT '{}',
//	    created_at  TIMESTAMPTZ DEFAULT NOW()
//	);
//
//	-- IVFFlat index for approximate search (train after ~10K rows).
//	-- Adjust lists based on dataset size: ~sqrt(row_count).
//	CREATE INDEX IF NOT EXISTS bunshin_vectors_ann
//	    ON bunshin_vectors USING ivfflat (embedding vector_cosine_ops)
//	    WITH (lists = 100);
//
// The embedding dimension must match the model used. Default above is 1536
// (text-embedding-3-small). Adjust when using other models.
type PostgresVectorStore struct {
	pool     *pgxpool.Pool
	tenantID string
	dim      int
}

// NewPostgresVectorStore constructs a store scoped to a tenant.
// dim is the embedding vector dimension; must match the table schema.
func NewPostgresVectorStore(pool *pgxpool.Pool, tenantID string, dim int) *PostgresVectorStore {
	return &PostgresVectorStore{pool: pool, tenantID: tenantID, dim: dim}
}

const pgVecUpsert = `
INSERT INTO bunshin_vectors (id, tenant_id, content, embedding, metadata)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE
SET content    = EXCLUDED.content,
    embedding  = EXCLUDED.embedding,
    metadata   = EXCLUDED.metadata,
    tenant_id  = EXCLUDED.tenant_id`

// Upsert inserts or replaces documents. Each document must have a pre-computed Vector.
func (s *PostgresVectorStore) Upsert(ctx context.Context, docs []Document) error {
	for _, doc := range docs {
		meta, err := json.Marshal(doc.Metadata)
		if err != nil {
			return fmt.Errorf("postgres vector: marshal metadata for %q: %w", doc.ID, err)
		}
		vec := pgvector.NewVector(doc.Vector)
		if _, err := s.pool.Exec(ctx, pgVecUpsert,
			doc.ID, s.tenantID, doc.Content, vec, meta,
		); err != nil {
			return fmt.Errorf("postgres vector: upsert %q: %w", doc.ID, err)
		}
	}
	return nil
}

const pgVecSearch = `
SELECT id, content, metadata, 1 - (embedding <=> $1) AS score
FROM bunshin_vectors
WHERE tenant_id = $2
ORDER BY embedding <=> $1
LIMIT $3`

// Search returns the topK documents nearest to query using cosine similarity.
// filter is applied as an in-process metadata filter after the ANN retrieval.
func (s *PostgresVectorStore) Search(ctx context.Context, query []float32, topK int, filter map[string]any) ([]SearchResult, error) {
	vec := pgvector.NewVector(query)
	rows, err := s.pool.Query(ctx, pgVecSearch, vec, s.tenantID, topK*3) // over-fetch for filter
	if err != nil {
		return nil, fmt.Errorf("postgres vector: search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var doc Document
		var metaJSON []byte
		var score float32
		if err := rows.Scan(&doc.ID, &doc.Content, &metaJSON, &score); err != nil {
			return nil, fmt.Errorf("postgres vector: scan: %w", err)
		}
		if err := json.Unmarshal(metaJSON, &doc.Metadata); err != nil {
			return nil, fmt.Errorf("postgres vector: decode metadata: %w", err)
		}
		if len(filter) > 0 && !metadataMatches(doc.Metadata, filter) {
			continue
		}
		results = append(results, SearchResult{Document: doc, Score: score})
		if len(results) >= topK {
			break
		}
	}
	return results, rows.Err()
}

const pgVecDelete = `DELETE FROM bunshin_vectors WHERE id = ANY($1) AND tenant_id = $2`

// Delete removes documents with the given IDs from the tenant's store.
func (s *PostgresVectorStore) Delete(ctx context.Context, ids []string) error {
	_, err := s.pool.Exec(ctx, pgVecDelete, ids, s.tenantID)
	if err != nil {
		return fmt.Errorf("postgres vector: delete: %w", err)
	}
	return nil
}
