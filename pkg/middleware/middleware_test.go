package middleware_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

func echo() core.Runnable {
	return core.NewRunnableFunc("echo", func(_ context.Context, input any) (any, error) {
		return input, nil
	})
}

func alwaysFail(err error) core.Runnable {
	return core.NewRunnableFunc("fail", func(_ context.Context, _ any) (any, error) {
		return nil, err
	})
}

func panicker() core.Runnable {
	return core.NewRunnableFunc("panic", func(_ context.Context, _ any) (any, error) {
		panic("test panic")
	})
}

// ---- Chain ----

func TestChain_Order(t *testing.T) {
	var order []string
	wrap := func(label string) middleware.Middleware {
		return func(next core.Runnable) core.Runnable {
			return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
				order = append(order, label+"-pre")
				out, err := next.Invoke(ctx, input)
				order = append(order, label+"-post")
				return out, err
			})
		}
	}

	r := middleware.Chain(echo(), wrap("A"), wrap("B"), wrap("C"))
	_, _ = r.Invoke(context.Background(), nil)

	want := []string{"A-pre", "B-pre", "C-pre", "C-post", "B-post", "A-post"}
	for i, got := range order {
		if got != want[i] {
			t.Fatalf("order[%d]: want %q, got %q", i, want[i], got)
		}
	}
}

func TestChain_NoMiddleware(t *testing.T) {
	r := middleware.Chain(echo())
	out, err := r.Invoke(context.Background(), "x")
	if err != nil || out != "x" {
		t.Fatalf("want x nil, got %v %v", out, err)
	}
}

// ---- WithLogging ----

func TestWithLogging_NoError(t *testing.T) {
	logger := slog.Default() // just ensure it doesn't panic
	r := middleware.Chain(echo(), middleware.WithLogging(logger))
	_, err := r.Invoke(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithLogging_Error(t *testing.T) {
	logger := slog.Default()
	sentinel := errors.New("boom")
	r := middleware.Chain(alwaysFail(sentinel), middleware.WithLogging(logger))
	_, err := r.Invoke(context.Background(), nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
}

// ---- WithPanicRecovery ----

func TestWithPanicRecovery_ConvertsToError(t *testing.T) {
	r := middleware.Chain(panicker(), middleware.WithPanicRecovery())
	_, err := r.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from panic, got nil")
	}
}

func TestWithPanicRecovery_NoPanic(t *testing.T) {
	r := middleware.Chain(echo(), middleware.WithPanicRecovery())
	out, err := r.Invoke(context.Background(), "safe")
	if err != nil || out != "safe" {
		t.Fatalf("want safe nil, got %v %v", out, err)
	}
}

// ---- WithRetry ----

func TestWithRetry_SucceedsFirstAttempt(t *testing.T) {
	r := middleware.Chain(echo(), middleware.WithRetry(middleware.RetryConfig{MaxAttempts: 3}))
	out, err := r.Invoke(context.Background(), "ok")
	if err != nil || out != "ok" {
		t.Fatalf("want ok nil, got %v %v", out, err)
	}
}

func TestWithRetry_RetriesAndSucceeds(t *testing.T) {
	attempts := 0
	r := middleware.Chain(
		core.NewRunnableFunc("flaky", func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("transient")
			}
			return "done", nil
		}),
		middleware.WithRetry(middleware.RetryConfig{MaxAttempts: 5, InitialDelay: time.Millisecond}),
	)
	out, err := r.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("want done, got %v", out)
	}
	if attempts != 3 {
		t.Fatalf("want 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	sentinel := errors.New("permanent")
	r := middleware.Chain(
		alwaysFail(sentinel),
		middleware.WithRetry(middleware.RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond}),
	)
	_, err := r.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel wrapped in retry error, got %v", err)
	}
}

func TestWithRetry_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	r := middleware.Chain(
		core.NewRunnableFunc("slow-fail", func(_ context.Context, _ any) (any, error) {
			attempts++
			cancel() // cancel after first attempt
			return nil, errors.New("fail")
		}),
		middleware.WithRetry(middleware.RetryConfig{MaxAttempts: 10, InitialDelay: 10 * time.Millisecond}),
	)
	_, err := r.Invoke(ctx, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts > 2 {
		t.Fatalf("context cancel not respected, got %d attempts", attempts)
	}
}

func TestWithRetry_SkipsNonRetryableError(t *testing.T) {
	sentinel := errors.New("fatal")
	calls := 0
	r := middleware.Chain(
		core.NewRunnableFunc("fatal", func(_ context.Context, _ any) (any, error) {
			calls++
			return nil, sentinel
		}),
		middleware.WithRetry(middleware.RetryConfig{
			MaxAttempts: 5,
			RetryIf:     func(err error) bool { return !errors.Is(err, sentinel) },
		}),
	)
	_, err := r.Invoke(context.Background(), nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 call for non-retryable error, got %d", calls)
	}
}

// ---- WithTimeout ----

func TestWithTimeout_ExpiresSlowRunnable(t *testing.T) {
	r := middleware.Chain(
		core.NewRunnableFunc("slow", func(ctx context.Context, _ any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return "late", nil
			}
		}),
		middleware.WithTimeout(10*time.Millisecond),
	)
	_, err := r.Invoke(context.Background(), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

func TestWithTimeout_FastRunnableOK(t *testing.T) {
	r := middleware.Chain(echo(), middleware.WithTimeout(time.Second))
	out, err := r.Invoke(context.Background(), "fast")
	if err != nil || out != "fast" {
		t.Fatalf("want fast nil, got %v %v", out, err)
	}
}
