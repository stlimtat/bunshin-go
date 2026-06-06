package telemetry_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/telemetry"
)

func TestNewRunContext(t *testing.T) {
	rc := telemetry.NewRunContext("my-project")
	if rc.ProjectName != "my-project" {
		t.Fatalf("want project my-project, got %q", rc.ProjectName)
	}
	if rc.RunID == "" {
		t.Fatal("RunID must not be empty")
	}
	if rc.ParentRunID != "" {
		t.Fatalf("root RunContext must have empty ParentRunID, got %q", rc.ParentRunID)
	}
	if rc.TraceID != rc.RunID {
		t.Fatalf("root TraceID must equal RunID")
	}
}

func TestRunContext_Child(t *testing.T) {
	parent := telemetry.NewRunContext("proj")
	child := parent.Child()

	if child.RunID == parent.RunID {
		t.Fatal("child RunID must differ from parent")
	}
	if child.ParentRunID != parent.RunID {
		t.Fatalf("child ParentRunID must equal parent RunID")
	}
	if child.TraceID != parent.TraceID {
		t.Fatal("child TraceID must equal parent TraceID")
	}
	if child.ProjectName != parent.ProjectName {
		t.Fatal("child must inherit ProjectName")
	}
}

func TestContext_RoundTrip(t *testing.T) {
	rc := telemetry.NewRunContext("test")
	ctx := telemetry.WithContext(context.Background(), rc)

	got, ok := telemetry.FromContext(ctx)
	if !ok {
		t.Fatal("expected RunContext in context")
	}
	if got.RunID != rc.RunID {
		t.Fatalf("want RunID %q, got %q", rc.RunID, got.RunID)
	}
}

func TestFromContext_Missing(t *testing.T) {
	_, ok := telemetry.FromContext(context.Background())
	if ok {
		t.Fatal("expected no RunContext in empty context")
	}
}

func TestMustFromContext_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on missing RunContext")
		}
	}()
	telemetry.MustFromContext(context.Background())
}

func TestWithLangSmith_InjectsRunContext(t *testing.T) {
	var captured telemetry.RunContext

	inner := core.NewRunnableFuncWithStream(
		"capture",
		func(ctx context.Context, input any) (any, error) {
			rc, ok := telemetry.FromContext(ctx)
			if !ok {
				t.Error("no RunContext in context")
			}
			captured = rc
			return input, nil
		},
		nil,
	)

	mw := telemetry.WithLangSmith("test-project")
	wrapped := mw(inner)

	_, err := wrapped.Invoke(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.ProjectName != "test-project" {
		t.Fatalf("want project test-project, got %q", captured.ProjectName)
	}
	if captured.RunID == "" {
		t.Fatal("RunID must not be empty after middleware")
	}
}
