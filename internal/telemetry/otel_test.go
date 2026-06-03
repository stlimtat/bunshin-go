package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stlimtat/bunshin-go/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
)

func noopTracer() trace.Tracer {
	return trace.NewNoopTracerProvider().Tracer("test")
}

func TestOTELBackend_NilTracer_Error(t *testing.T) {
	_, err := telemetry.NewOTELBackend(nil)
	if err == nil {
		t.Fatal("expected error for nil tracer")
	}
}

func TestOTELBackend_StartAndEndRun(t *testing.T) {
	b, err := telemetry.NewOTELBackend(noopTracer())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	run := &telemetry.Run{ID: uuid.New(), Name: "otel-run", RunType: telemetry.RunTypeChain}
	ctx, err := b.StartRun(context.Background(), run)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := b.EndRun(ctx, run.ID, nil, nil); err != nil {
		t.Fatalf("EndRun: %v", err)
	}
}

func TestOTELBackend_EndRun_WithError(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	run := &telemetry.Run{ID: uuid.New(), RunType: telemetry.RunTypeLLM}
	ctx, _ := b.StartRun(context.Background(), run)
	if err := b.EndRun(ctx, run.ID, nil, errors.New("llm timeout")); err != nil {
		t.Fatalf("EndRun with error: %v", err)
	}
}

func TestOTELBackend_EndRun_UnknownID(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	err := b.EndRun(context.Background(), uuid.New(), nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown run ID")
	}
}

func TestOTELBackend_DuplicateStart_Error(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	run := &telemetry.Run{ID: uuid.New(), RunType: telemetry.RunTypeChain}
	_, _ = b.StartRun(context.Background(), run)
	_, err := b.StartRun(context.Background(), run)
	if err == nil {
		t.Fatal("expected error for duplicate run ID")
	}
}

func TestOTELBackend_StartRun_CancelledContext(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := &telemetry.Run{ID: uuid.New(), RunType: telemetry.RunTypeChain}
	_, err := b.StartRun(ctx, run)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestOTELBackend_AddFeedback_NoOp(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	score := 1.0
	if err := b.AddFeedback(context.Background(), uuid.New(), telemetry.Feedback{
		Key: "quality", Score: &score,
	}); err != nil {
		t.Fatalf("AddFeedback: %v", err)
	}
}

func TestOTELBackend_Flush_NoOp(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestOTELBackend_WithParentID(t *testing.T) {
	b, _ := telemetry.NewOTELBackend(noopTracer())
	parentID := uuid.New()
	run := &telemetry.Run{ID: uuid.New(), Name: "child", RunType: telemetry.RunTypeLLM, ParentID: &parentID}
	ctx, err := b.StartRun(context.Background(), run)
	if err != nil {
		t.Fatalf("StartRun with parent: %v", err)
	}
	_ = b.EndRun(ctx, run.ID, nil, nil)
}
