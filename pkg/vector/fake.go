package vector

import "context"

// FakeVectorStore is a thin wrapper around MemoryVectorStore for use in tests.
// It records the last Search call for assertion.
type FakeVectorStore struct {
	*MemoryVectorStore
	LastQuery  []float32
	LastFilter map[string]any
	LastTopK   int
}

func NewFakeVectorStore() *FakeVectorStore {
	return &FakeVectorStore{MemoryVectorStore: NewMemoryVectorStore()}
}

func (f *FakeVectorStore) Search(ctx context.Context, query []float32, topK int, filter map[string]any) ([]SearchResult, error) {
	f.LastQuery = query
	f.LastFilter = filter
	f.LastTopK = topK
	return f.MemoryVectorStore.Search(ctx, query, topK, filter)
}
