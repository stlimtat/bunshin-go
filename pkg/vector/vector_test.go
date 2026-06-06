package vector_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/vector"
)

func TestMemoryVectorStore_UpsertSearch(t *testing.T) {
	s := vector.NewMemoryVectorStore()
	ctx := context.Background()

	docs := []vector.Document{
		{ID: "a", Content: "hello", Vector: []float32{1, 0, 0}, Metadata: map[string]any{"tenant_id": "t1"}},
		{ID: "b", Content: "world", Vector: []float32{0, 1, 0}, Metadata: map[string]any{"tenant_id": "t1"}},
		{ID: "c", Content: "other", Vector: []float32{0, 0, 1}, Metadata: map[string]any{"tenant_id": "t2"}},
	}
	if err := s.Upsert(ctx, docs); err != nil {
		t.Fatal(err)
	}

	// Search with tenant filter — should only return t1 docs
	results, err := s.Search(ctx, []float32{1, 0, 0}, 5, map[string]any{"tenant_id": "t1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("expected top result 'a', got %q", results[0].ID)
	}
	if results[0].Score < 0.99 {
		t.Errorf("expected score ~1.0 for identical vector, got %f", results[0].Score)
	}
}

func TestMemoryVectorStore_Delete(t *testing.T) {
	s := vector.NewMemoryVectorStore()
	ctx := context.Background()

	_ = s.Upsert(ctx, []vector.Document{
		{ID: "x", Content: "foo", Vector: []float32{1, 0}, Metadata: map[string]any{}},
	})
	_ = s.Delete(ctx, []string{"x"})

	results, _ := s.Search(ctx, []float32{1, 0}, 5, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(results))
	}
}

func TestMemoryVectorStore_Upsert_Overwrites(t *testing.T) {
	s := vector.NewMemoryVectorStore()
	ctx := context.Background()

	_ = s.Upsert(ctx, []vector.Document{
		{ID: "a", Content: "v1", Vector: []float32{1, 0}, Metadata: map[string]any{}},
	})
	_ = s.Upsert(ctx, []vector.Document{
		{ID: "a", Content: "v2", Vector: []float32{1, 0}, Metadata: map[string]any{}},
	})
	results, _ := s.Search(ctx, []float32{1, 0}, 5, nil)
	if len(results) != 1 || results[0].Content != "v2" {
		t.Errorf("expected upsert to overwrite, got %+v", results)
	}
}

func TestMemoryVectorStore_TopK(t *testing.T) {
	s := vector.NewMemoryVectorStore()
	ctx := context.Background()

	_ = s.Upsert(ctx, []vector.Document{
		{ID: "1", Content: "a", Vector: []float32{1, 0}, Metadata: map[string]any{}},
		{ID: "2", Content: "b", Vector: []float32{0.9, 0.1}, Metadata: map[string]any{}},
		{ID: "3", Content: "c", Vector: []float32{0, 1}, Metadata: map[string]any{}},
	})
	results, _ := s.Search(ctx, []float32{1, 0}, 2, nil)
	if len(results) != 2 {
		t.Errorf("expected topK=2, got %d", len(results))
	}
}
