package eval_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stlimtat/bunshin-go/pkg/eval"
)

func makeDataset(n int) *eval.Dataset {
	ds := &eval.Dataset{ID: uuid.New(), Name: "test-ds"}
	for i := 0; i < n; i++ {
		ds.Examples = append(ds.Examples, &eval.Example{
			ID:        uuid.New(),
			Input:     map[string]any{"text": "input"},
			Reference: map[string]any{"output": "expected"},
		})
	}
	return ds
}

// ---- EvalRunner ----

func TestEvalRunner_Run_AllPass(t *testing.T) {
	fn := func(_ context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"output": "expected"}, nil
	}
	runner := eval.NewEvalRunner(fn, []eval.Evaluator{&eval.ExactMatch{Key: "output"}}, 3)
	report, err := runner.Run(context.Background(), makeDataset(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Runs) != 5 {
		t.Fatalf("want 5 runs, got %d", len(report.Runs))
	}
	if score := report.Scores["exact_match.output"]; score != 1.0 {
		t.Fatalf("want score=1.0, got %f", score)
	}
}

func TestEvalRunner_Run_AllFail(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (map[string]any, error) {
		return map[string]any{"output": "wrong"}, nil
	}
	runner := eval.NewEvalRunner(fn, []eval.Evaluator{&eval.ExactMatch{Key: "output"}}, 2)
	report, _ := runner.Run(context.Background(), makeDataset(3))
	if score := report.Scores["exact_match.output"]; score != 0.0 {
		t.Fatalf("want score=0.0, got %f", score)
	}
}

func TestEvalRunner_Run_FnError(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (map[string]any, error) {
		return nil, errors.New("model unavailable")
	}
	runner := eval.NewEvalRunner(fn, []eval.Evaluator{&eval.ExactMatch{Key: "output"}}, 1)
	report, _ := runner.Run(context.Background(), makeDataset(2))
	for _, run := range report.Runs {
		if run.Err == nil {
			t.Fatal("expected error on each run")
		}
	}
}

func TestEvalRunner_Run_EmptyDataset(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (map[string]any, error) {
		return nil, nil
	}
	runner := eval.NewEvalRunner(fn, nil, 1)
	report, err := runner.Run(context.Background(), makeDataset(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Runs) != 0 {
		t.Fatalf("want 0 runs, got %d", len(report.Runs))
	}
}

// ---- ExactMatch ----

func TestExactMatch_Pass(t *testing.T) {
	ev := &eval.ExactMatch{Key: "answer"}
	run := &eval.EvalRun{
		Actual:    map[string]any{"answer": "42"},
		Reference: map[string]any{"answer": "42"},
	}
	s, err := ev.Evaluate(context.Background(), run)
	if err != nil || s.Value != 1.0 {
		t.Fatalf("want score=1.0, got %f %v", s.Value, err)
	}
}

func TestExactMatch_Fail(t *testing.T) {
	ev := &eval.ExactMatch{Key: "answer"}
	run := &eval.EvalRun{
		Actual:    map[string]any{"answer": "43"},
		Reference: map[string]any{"answer": "42"},
	}
	s, _ := ev.Evaluate(context.Background(), run)
	if s.Value != 0.0 {
		t.Fatalf("want score=0.0, got %f", s.Value)
	}
}

func TestExactMatch_RunError(t *testing.T) {
	ev := &eval.ExactMatch{Key: "answer"}
	run := &eval.EvalRun{Err: errors.New("boom")}
	s, _ := ev.Evaluate(context.Background(), run)
	if s.Value != 0.0 {
		t.Fatalf("want score=0.0 on run error, got %f", s.Value)
	}
}

// ---- ContainsAll ----

func TestContainsAll_Pass(t *testing.T) {
	ev := &eval.ContainsAll{Key: "text", Required: []string{"Paris", "France"}}
	run := &eval.EvalRun{Actual: map[string]any{"text": "Paris is the capital of France"}}
	s, _ := ev.Evaluate(context.Background(), run)
	if s.Value != 1.0 {
		t.Fatalf("want 1.0, got %f (%s)", s.Value, s.Comment)
	}
}

func TestContainsAll_Fail(t *testing.T) {
	ev := &eval.ContainsAll{Key: "text", Required: []string{"Berlin"}}
	run := &eval.EvalRun{Actual: map[string]any{"text": "Paris is the capital of France"}}
	s, _ := ev.Evaluate(context.Background(), run)
	if s.Value != 0.0 {
		t.Fatalf("want 0.0, got %f", s.Value)
	}
}

// ---- Latency ----

func TestLatency_Pass(t *testing.T) {
	ev := &eval.Latency{MaxDuration: time.Second}
	run := &eval.EvalRun{Latency: 50 * time.Millisecond}
	s, _ := ev.Evaluate(context.Background(), run)
	if s.Value != 1.0 {
		t.Fatalf("want 1.0, got %f", s.Value)
	}
}

func TestLatency_Fail(t *testing.T) {
	ev := &eval.Latency{MaxDuration: 100 * time.Millisecond}
	run := &eval.EvalRun{Latency: 500 * time.Millisecond}
	s, _ := ev.Evaluate(context.Background(), run)
	if s.Value != 0.0 {
		t.Fatalf("want 0.0, got %f", s.Value)
	}
}

// ---- MemoryDatasetBackend ----

func TestMemoryDatasetBackend_PushAndPull(t *testing.T) {
	b := eval.NewMemoryDatasetBackend()
	ds := makeDataset(3)
	ds.Name = "myds"
	_ = b.Push(context.Background(), ds)

	got, err := b.Pull(context.Background(), "myds")
	if err != nil || len(got.Examples) != 3 {
		t.Fatalf("unexpected: %v %d", err, len(got.Examples))
	}
}

func TestMemoryDatasetBackend_Pull_Missing(t *testing.T) {
	b := eval.NewMemoryDatasetBackend()
	_, err := b.Pull(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMemoryDatasetBackend_PushResults(t *testing.T) {
	b := eval.NewMemoryDatasetBackend()
	_ = b.PushResults(context.Background(), &eval.EvalReport{ID: uuid.New()})
	// No assertion needed — just ensure it doesn't error.
}

func TestMemoryDatasetBackend_ListDatasets(t *testing.T) {
	b := eval.NewMemoryDatasetBackend()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		ds := &eval.Dataset{ID: uuid.New(), Name: name}
		_ = b.Push(context.Background(), ds)
	}
	list, err := b.ListDatasets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 datasets, got %d", len(list))
	}
}
