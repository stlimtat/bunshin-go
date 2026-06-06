package middleware_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

func makeCounter(n *atomic.Int64) core.Runnable {
	return core.NewRunnableFunc("counter", func(_ context.Context, input any) (any, error) {
		n.Add(1)
		return input, nil
	})
}

func TestWithMaxIterations_AllowsUnderLimit(t *testing.T) {
	var calls atomic.Int64
	r := middleware.Chain(makeCounter(&calls), middleware.WithMaxIterations(3))
	for i := 0; i < 3; i++ {
		if _, err := r.Invoke(context.Background(), nil); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("want 3 calls, got %d", calls.Load())
	}
}

func TestWithMaxIterations_BlocksAtLimit(t *testing.T) {
	r := middleware.Chain(echo(), middleware.WithMaxIterations(2))
	_, _ = r.Invoke(context.Background(), nil)
	_, _ = r.Invoke(context.Background(), nil)
	_, err := r.Invoke(context.Background(), nil)
	if !errors.Is(err, middleware.ErrMaxIterationsExceeded) {
		t.Fatalf("want ErrMaxIterationsExceeded, got %v", err)
	}
}

func TestWithMaxIterations_StreamBlocksAtLimit(t *testing.T) {
	r := middleware.Chain(echo(), middleware.WithMaxIterations(1))
	_, _ = r.Stream(context.Background(), nil)
	_, err := r.Stream(context.Background(), nil)
	if !errors.Is(err, middleware.ErrMaxIterationsExceeded) {
		t.Fatalf("want ErrMaxIterationsExceeded on stream, got %v", err)
	}
}

func TestTokenBudgetTracker_Consume(t *testing.T) {
	tracker := middleware.NewTokenBudgetTracker(100)
	if err := tracker.Consume(40); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.Remaining() != 60 {
		t.Fatalf("want 60, got %d", tracker.Remaining())
	}
	if err := tracker.Consume(70); !errors.Is(err, middleware.ErrTokenBudgetExceeded) {
		t.Fatalf("want ErrTokenBudgetExceeded, got %v", err)
	}
}

func TestWithTokenBudget_BlocksWhenExhausted(t *testing.T) {
	tracker := middleware.NewTokenBudgetTracker(0)
	r := middleware.Chain(echo(), middleware.WithTokenBudget(tracker, nil))
	_, err := r.Invoke(context.Background(), "hi")
	if !errors.Is(err, middleware.ErrTokenBudgetExceeded) {
		t.Fatalf("want ErrTokenBudgetExceeded, got %v", err)
	}
}

func TestWithTokenBudget_ExtractorDecremented(t *testing.T) {
	tracker := middleware.NewTokenBudgetTracker(1000)
	extractor := func(_ any) int64 { return 300 }
	r := middleware.Chain(echo(), middleware.WithTokenBudget(tracker, extractor))

	if _, err := r.Invoke(context.Background(), "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.Remaining() != 700 {
		t.Fatalf("want 700 remaining, got %d", tracker.Remaining())
	}
}

func TestBudgetFromContext_InjectedByMiddleware(t *testing.T) {
	tracker := middleware.NewTokenBudgetTracker(500)
	var captured *middleware.TokenBudgetTracker

	inner := core.NewRunnableFunc("capture", func(ctx context.Context, input any) (any, error) {
		captured = middleware.BudgetFromContext(ctx)
		return input, nil
	})
	r := middleware.Chain(inner, middleware.WithTokenBudget(tracker, nil))
	_, _ = r.Invoke(context.Background(), nil)

	if captured == nil {
		t.Fatal("tracker not injected into context")
	}
	if captured.Remaining() != 500 {
		t.Fatalf("want 500, got %d", captured.Remaining())
	}
}

func TestWithTokenBudget_StreamBlocksWhenExhausted(t *testing.T) {
	tracker := middleware.NewTokenBudgetTracker(0)
	r := middleware.Chain(echo(), middleware.WithTokenBudget(tracker, nil))
	_, err := r.Stream(context.Background(), nil)
	if !errors.Is(err, middleware.ErrTokenBudgetExceeded) {
		t.Fatalf("want ErrTokenBudgetExceeded on stream, got %v", err)
	}
}
