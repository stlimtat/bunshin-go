package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// echoTyped is a TypedRunnable[string, string] that returns its input.
type echoTyped struct{}

func (e *echoTyped) Invoke(_ context.Context, input string) (string, error) {
	return input, nil
}

// failTyped always returns an error.
type failTyped struct{ err error }

func (f *failTyped) Invoke(_ context.Context, _ string) (string, error) {
	return "", f.err
}

func TestAsRunnable_Invoke_TypeMatch(t *testing.T) {
	r := core.AsRunnable[string, string]("echo", &echoTyped{})
	out, err := r.Invoke(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello" {
		t.Fatalf("want %q, got %q", "hello", out)
	}
}

func TestAsRunnable_Invoke_TypeMismatch(t *testing.T) {
	r := core.AsRunnable[string, string]("echo", &echoTyped{})
	_, err := r.Invoke(context.Background(), 42) // int, not string
	if err == nil {
		t.Fatal("expected type mismatch error, got nil")
	}
}

func TestAsRunnable_Invoke_ErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom")
	r := core.AsRunnable[string, string]("fail", &failTyped{err: sentinel})
	_, err := r.Invoke(context.Background(), "x")
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
}

func TestAsRunnable_Stream_SingleChunk(t *testing.T) {
	r := core.AsRunnable[string, string]("echo", &echoTyped{})
	ch, err := r.Stream(context.Background(), "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []core.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Value != "world" {
		t.Fatalf("want %q, got %v", "world", chunks[0].Value)
	}
}

func TestAsRunnable_Stream_TypeMismatch(t *testing.T) {
	r := core.AsRunnable[string, string]("echo", &echoTyped{})
	_, err := r.Stream(context.Background(), 99)
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
}

func TestRunnableFunc_Invoke(t *testing.T) {
	r := core.NewRunnableFunc("double", func(_ context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})
	out, err := r.Invoke(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 10 {
		t.Fatalf("want 10, got %v", out)
	}
}

func TestRunnableFunc_Stream_DefaultsToInvoke(t *testing.T) {
	r := core.NewRunnableFunc("inc", func(_ context.Context, input any) (any, error) {
		return input.(int) + 1, nil
	})
	ch, err := r.Stream(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []core.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if len(chunks) != 1 || chunks[0].Value != 4 {
		t.Fatalf("want [4], got %v", chunks)
	}
}

func TestRunnableFunc_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	r := core.NewRunnableFunc("check-ctx", func(ctx context.Context, _ any) (any, error) {
		return nil, ctx.Err()
	})
	_, err := r.Invoke(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRunnableFunc_NilInput(t *testing.T) {
	r := core.NewRunnableFunc("nil-ok", func(_ context.Context, input any) (any, error) {
		return input, nil
	})
	out, err := r.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Fatalf("want nil, got %v", out)
	}
}

func TestRunnableFunc_NilInvoke_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil invoke")
		}
	}()
	core.NewRunnableFunc("bad", nil)
}

func TestRunnableFuncWithStream_ExplicitStreamCalled(t *testing.T) {
	streamCalled := false
	invoke := func(_ context.Context, input any) (any, error) { return input, nil }
	streamFn := func(_ context.Context, input any) (<-chan core.StreamChunk, error) {
		streamCalled = true
		ch := make(chan core.StreamChunk, 2)
		ch <- core.StreamChunk{Value: "chunk1"}
		ch <- core.StreamChunk{Value: "chunk2"}
		close(ch)
		return ch, nil
	}
	r := core.NewRunnableFuncWithStream("multi", invoke, streamFn)
	ch, err := r.Stream(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []core.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if !streamCalled {
		t.Fatal("explicit stream function was not called")
	}
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Value != "chunk1" || chunks[1].Value != "chunk2" {
		t.Fatalf("unexpected chunk values: %v", chunks)
	}
}

func TestRunnableFuncWithStream_NilStream_FallsBackToInvoke(t *testing.T) {
	r := core.NewRunnableFuncWithStream("fallback",
		func(_ context.Context, input any) (any, error) { return input.(int) * 3, nil },
		nil,
	)
	ch, err := r.Stream(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []core.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if len(chunks) != 1 || chunks[0].Value != 21 {
		t.Fatalf("want [21], got %v", chunks)
	}
}

func TestRunnableFuncWithStream_NilInvoke_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil invoke")
		}
	}()
	core.NewRunnableFuncWithStream("bad", nil, nil)
}
