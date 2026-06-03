package fault_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
	"github.com/stlimtat/bunshin-go/pkg/testing/fault"
)

func echo() core.Runnable {
	return core.NewRunnableFunc("echo", func(_ context.Context, in any) (any, error) { return in, nil })
}

func TestFaultInjection_ZeroRate_NeverFails(t *testing.T) {
	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate: 0, Seed: 42,
	}))
	for i := 0; i < 100; i++ {
		_, err := r.Invoke(context.Background(), i)
		if err != nil {
			t.Fatalf("unexpected error at iteration %d: %v", i, err)
		}
	}
}

func TestFaultInjection_FullRate_AlwaysFails(t *testing.T) {
	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate:  1.0,
		ErrorTypes: []fault.ErrorType{fault.Unavailable},
		Seed:       1,
	}))
	for i := 0; i < 10; i++ {
		_, err := r.Invoke(context.Background(), i)
		if err == nil {
			t.Fatalf("expected error at iteration %d", i)
		}
	}
}

func TestFaultInjection_PartialRate_SomeFail(t *testing.T) {
	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate:  0.5,
		ErrorTypes: []fault.ErrorType{fault.RateLimit},
		Seed:       99,
	}))
	var successes, failures int
	for i := 0; i < 200; i++ {
		_, err := r.Invoke(context.Background(), i)
		if err != nil {
			failures++
		} else {
			successes++
		}
	}
	// With 50% rate and 200 calls, expect roughly 100 of each.
	// Allow generous tolerance (40–160) to avoid flakiness.
	if failures < 40 || failures > 160 {
		t.Fatalf("expected ~50%% failures, got %d/%d", failures, 200)
	}
}

func TestFaultInjection_TimeoutError(t *testing.T) {
	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate:  1.0,
		ErrorTypes: []fault.ErrorType{fault.Timeout},
		Seed:       7,
	}))
	_, err := r.Invoke(context.Background(), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

func TestFaultInjection_Latency(t *testing.T) {
	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate:  0,
		LatencyP99: 50 * time.Millisecond,
		Seed:       3,
	}))
	start := time.Now()
	_, err := r.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	// With LatencyP99=50ms and Seed=3 the jitter is deterministic and non-zero.
	// CI machines vary widely; allow generous upper bound of 2s.
	if elapsed > 2*time.Second {
		t.Fatalf("invocation took too long: %v (expected < 2s)", elapsed)
	}
}

func TestFaultInjection_ContextCancelDuringLatency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate:  0,
		LatencyP99: 10 * time.Second, // very long — should be interrupted
		Seed:       5,
	}))
	_, err := r.Invoke(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestFaultInjection_DefaultErrorType(t *testing.T) {
	// No ErrorTypes specified — defaults to Unavailable.
	r := middleware.Chain(echo(), fault.WithFaultInjection(fault.Config{
		ErrorRate: 1.0,
		Seed:      11,
	}))
	_, err := r.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error with default error type")
	}
}
