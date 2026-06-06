package eval_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stlimtat/bunshin-go/pkg/eval"
)

func TestJSONLDatasetBackend_PushAndPull(t *testing.T) {
	dir := t.TempDir()
	b := eval.NewJSONLDatasetBackend(dir)

	ds := &eval.Dataset{
		ID:   uuid.New(),
		Name: "greet",
		Examples: []*eval.Example{
			{ID: uuid.New(), Input: map[string]any{"q": "hi"}, Reference: map[string]any{"a": "hello"}},
			{ID: uuid.New(), Input: map[string]any{"q": "bye"}, Reference: map[string]any{"a": "goodbye"}},
		},
	}

	if err := b.Push(context.Background(), ds); err != nil {
		t.Fatalf("push: %v", err)
	}

	got, err := b.Pull(context.Background(), "greet")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(got.Examples) != 2 {
		t.Fatalf("want 2 examples, got %d", len(got.Examples))
	}
	if got.Examples[0].Input["q"] != "hi" {
		t.Fatalf("wrong input: %v", got.Examples[0].Input)
	}
}

func TestJSONLDatasetBackend_PullMissing(t *testing.T) {
	dir := t.TempDir()
	b := eval.NewJSONLDatasetBackend(dir)

	_, err := b.Pull(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing dataset")
	}
}

func TestJSONLDatasetBackend_ListDatasets(t *testing.T) {
	dir := t.TempDir()
	b := eval.NewJSONLDatasetBackend(dir)

	for _, name := range []string{"ds-a", "ds-b"} {
		_ = b.Push(context.Background(), &eval.Dataset{ID: uuid.New(), Name: name})
	}

	list, err := b.ListDatasets(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 datasets, got %d", len(list))
	}
}

func TestJSONLDatasetBackend_PushResults(t *testing.T) {
	dir := t.TempDir()
	b := eval.NewJSONLDatasetBackend(dir)

	report := &eval.EvalReport{
		ID:        uuid.New(),
		DatasetID: uuid.New(),
		Scores:    map[string]float64{"exact_match": 1.0},
	}
	if err := b.PushResults(context.Background(), report); err != nil {
		t.Fatalf("push results: %v", err)
	}
}

func TestJSONLDatasetBackend_RoundtripPreservesIDs(t *testing.T) {
	dir := t.TempDir()
	b := eval.NewJSONLDatasetBackend(dir)

	id := uuid.New()
	ds := &eval.Dataset{
		Name: "ids",
		Examples: []*eval.Example{
			{ID: id, Input: map[string]any{"x": 1}},
		},
	}
	_ = b.Push(context.Background(), ds)

	got, _ := b.Pull(context.Background(), "ids")
	if got.Examples[0].ID != id {
		t.Fatalf("ID not preserved: got %v, want %v", got.Examples[0].ID, id)
	}
}
