package core_test

import (
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

func TestWrapError_Nil(t *testing.T) {
	if core.WrapError("r", nil) != nil {
		t.Error("expected nil for nil error")
	}
}

func TestWrapError_NonNil(t *testing.T) {
	cause := errors.New("boom")
	err := core.WrapError("my-runnable", cause)
	if err == nil {
		t.Fatal("expected non-nil")
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is chain broken: %v", err)
	}
	if err.Error() == "" {
		t.Error("Error() must be non-empty")
	}
}

func TestRunnableError_Unwrap(t *testing.T) {
	cause := errors.New("root")
	wrapped := core.WrapError("r", cause)
	var re *core.RunnableError
	if !errors.As(wrapped, &re) {
		t.Fatal("expected *RunnableError")
	}
	if re.Unwrap() != cause {
		t.Error("Unwrap must return original cause")
	}
	if re.Runnable != "r" {
		t.Errorf("expected Runnable=r, got %q", re.Runnable)
	}
}

func TestTypeMismatchError(t *testing.T) {
	err := &core.TypeMismatchError{Runnable: "foo", Got: 42}
	if err.Error() == "" {
		t.Error("Error() must be non-empty")
	}
}

func TestAsRunnable_Name(t *testing.T) {
	r := core.AsRunnable[string, string]("my-name", &struct {
		core.TypedRunnable[string, string]
	}{})
	_ = r.Name // compile-time check only
}
