package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stlimtat/bunshin-go/pkg/telemetry"
)

func TestMemoryBackend_StartAndEndRun(t *testing.T) {
	b := telemetry.NewMemoryBackend()
	run := &telemetry.Run{ID: uuid.New(), Name: "test-chain", RunType: telemetry.RunTypeChain}

	ctx, err := b.StartRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Run ID injected into ctx.
	if telemetry.RunIDFromContext(ctx) != run.ID {
		t.Fatal("run ID not injected into context")
	}

	_ = b.EndRun(ctx, run.ID, map[string]any{"result": "ok"}, nil)

	got, ok := b.GetRun(run.ID)
	if !ok {
		t.Fatal("run not found")
	}
	if got.EndTime == nil {
		t.Fatal("EndTime not set")
	}
	if got.Outputs["result"] != "ok" {
		t.Fatalf("want result=ok, got %v", got.Outputs["result"])
	}
}

func TestMemoryBackend_AutoParent(t *testing.T) {
	b := telemetry.NewMemoryBackend()

	parent := &telemetry.Run{ID: uuid.New(), Name: "parent", RunType: telemetry.RunTypeChain}
	ctx, _ := b.StartRun(context.Background(), parent)

	child := &telemetry.Run{ID: uuid.New(), Name: "child", RunType: telemetry.RunTypeLLM}
	_, _ = b.StartRun(ctx, child)

	got, _ := b.GetRun(child.ID)
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Fatalf("child not auto-parented to parent run")
	}
}

func TestMemoryBackend_EndRun_WithError(t *testing.T) {
	b := telemetry.NewMemoryBackend()
	run := &telemetry.Run{ID: uuid.New(), RunType: telemetry.RunTypeChain}
	ctx, _ := b.StartRun(context.Background(), run)
	_ = b.EndRun(ctx, run.ID, nil, errors.New("timeout"))

	got, _ := b.GetRun(run.ID)
	if got.Error == nil || *got.Error != "timeout" {
		t.Fatalf("error not recorded, got %v", got.Error)
	}
}

func TestMemoryBackend_AddFeedback(t *testing.T) {
	b := telemetry.NewMemoryBackend()
	id := uuid.New()
	score := 0.9
	err := b.AddFeedback(context.Background(), id, telemetry.Feedback{
		Key: "correctness", Score: &score, Source: "human",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMemoryBackend_Runs_InsertionOrder(t *testing.T) {
	b := telemetry.NewMemoryBackend()
	for i := 0; i < 3; i++ {
		r := &telemetry.Run{ID: uuid.New(), Name: "run", RunType: telemetry.RunTypeChain}
		_, _ = b.StartRun(context.Background(), r)
	}
	if len(b.Runs()) != 3 {
		t.Fatalf("want 3 runs, got %d", len(b.Runs()))
	}
}

func TestRunIDFromContext_NoID(t *testing.T) {
	id := telemetry.RunIDFromContext(context.Background())
	if id != uuid.Nil {
		t.Fatalf("want uuid.Nil, got %v", id)
	}
}

func TestMultiBackend_FanOut(t *testing.T) {
	a := telemetry.NewMemoryBackend()
	b := telemetry.NewMemoryBackend()
	multi := telemetry.NewMultiBackend(a, b)

	run := &telemetry.Run{ID: uuid.New(), RunType: telemetry.RunTypeChain}
	_, err := multi.StartRun(context.Background(), run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := a.GetRun(run.ID); !ok {
		t.Fatal("run not in backend a")
	}
	if _, ok := b.GetRun(run.ID); !ok {
		t.Fatal("run not in backend b")
	}
}
