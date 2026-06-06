package vector

import (
	"context"
	"math"
	"sort"
	"sync"
)

// MemoryVectorStore is an in-process VectorStore backed by a plain slice.
// It performs exact cosine similarity search — O(n) per query.
// Use for tests and local development; not suitable for large corpora.
type MemoryVectorStore struct {
	mu   sync.RWMutex
	docs map[string]Document
}

// NewMemoryVectorStore returns an empty in-process VectorStore.
func NewMemoryVectorStore() *MemoryVectorStore {
	return &MemoryVectorStore{docs: make(map[string]Document)}
}

func (s *MemoryVectorStore) Upsert(_ context.Context, docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range docs {
		s.docs[d.ID] = d
	}
	return nil
}

func (s *MemoryVectorStore) Search(_ context.Context, query []float32, topK int, filter map[string]any) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		doc   Document
		score float32
	}
	var candidates []scored
	for _, d := range s.docs {
		if !metadataMatches(d.Metadata, filter) {
			continue
		}
		candidates = append(candidates, scored{doc: d, score: cosine(query, d.Vector)})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if topK < len(candidates) {
		candidates = candidates[:topK]
	}
	results := make([]SearchResult, len(candidates))
	for i, c := range candidates {
		results[i] = SearchResult{Document: c.doc, Score: c.score}
	}
	return results, nil
}

func (s *MemoryVectorStore) Delete(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.docs, id)
	}
	return nil
}

// metadataMatches reports whether doc metadata contains all filter key-value pairs.
func metadataMatches(meta, filter map[string]any) bool {
	for k, v := range filter {
		if meta[k] != v {
			return false
		}
	}
	return true
}

// cosine returns the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func cosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(magA) * math.Sqrt(magB)))
}
