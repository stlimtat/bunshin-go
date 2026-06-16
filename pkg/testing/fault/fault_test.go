package fault_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
	"github.com/stlimtat/bunshin-go/pkg/testing/fault"
)

var errBoom = errors.New("boom")

func echo() core.Runnable {
	return core.NewRunnableFuncWithStream(
		"echo",
		func(_ context.Context, in any) (any, error) { return in, nil },
		func(_ context.Context, in any) (<-chan core.StreamChunk, error) {
			ch := make(chan core.StreamChunk, 1)
			ch <- core.StreamChunk{Value: in}
			close(ch)
			return ch, nil
		},
	)
}

// ---- ErrorRate ----

func TestErrorRate_ZeroRate_NeverFails(t *testing.T) {
	r := middleware.Chain(echo(), fault.ErrorRate(0, errBoom, fault.WithSource(rand.NewSource(42))))
	for i := 0; i < 100; i++ {
		if _, err := r.Invoke(context.Background(), i); err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}
}

func TestErrorRate_FullRate_AlwaysFails_Invoke(t *testing.T) {
	r := middleware.Chain(echo(), fault.ErrorRate(1.0, errBoom, fault.WithSource(rand.NewSource(1))))
	for i := 0; i < 10; i++ {
		_, err := r.Invoke(context.Background(), i)
		if !errors.Is(err, errBoom) {
			t.Fatalf("iteration %d: want errBoom, got %v", i, err)
		}
	}
}

func TestErrorRate_FullRate_AlwaysFails_Stream(t *testing.T) {
	r := middleware.Chain(echo(), fault.ErrorRate(1.0, errBoom, fault.WithSource(rand.NewSource(1))))
	_, err := r.Stream(context.Background(), nil)
	if !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestErrorRate_PartialRate_SomeFail(t *testing.T) {
	r := middleware.Chain(echo(), fault.ErrorRate(0.5, errBoom, fault.WithSource(rand.NewSource(99))))
	var failures int
	for i := 0; i < 200; i++ {
		if _, err := r.Invoke(context.Background(), i); err != nil {
			failures++
		}
	}
	// 50% over 200 calls; allow [40, 160] to avoid flakiness.
	if failures < 40 || failures > 160 {
		t.Fatalf("expected ~50%% failures, got %d/200", failures)
	}
}

func TestErrorRate_SkipsWrappedCallOnFault(t *testing.T) {
	var calls int
	counter := core.NewRunnableFunc("counter", func(_ context.Context, in any) (any, error) {
		calls++
		return in, nil
	})
	r := middleware.Chain(counter, fault.ErrorRate(1.0, errBoom, fault.WithSource(rand.NewSource(7))))
	for i := 0; i < 5; i++ {
		r.Invoke(context.Background(), i) //nolint:errcheck
	}
	if calls != 0 {
		t.Fatalf("wrapped Runnable should not be called on fault; got %d calls", calls)
	}
}

func TestErrorRate_Deterministic_WithSeed(t *testing.T) {
	seq1 := faultSequence(rand.NewSource(42), 20)
	seq2 := faultSequence(rand.NewSource(42), 20)
	for i, v := range seq1 {
		if v != seq2[i] {
			t.Fatalf("sequence mismatch at index %d: %v vs %v", i, v, seq2[i])
		}
	}
}

func faultSequence(src rand.Source, n int) []bool {
	r := middleware.Chain(echo(), fault.ErrorRate(0.5, errBoom, fault.WithSource(src)))
	out := make([]bool, n)
	for i := 0; i < n; i++ {
		_, err := r.Invoke(context.Background(), i)
		out[i] = err != nil
	}
	return out
}

func TestErrorRate_Panics_InvalidP(t *testing.T) {
	for _, p := range []float64{-0.1, 1.1, -1} {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for p=%v", p)
				}
			}()
			fault.ErrorRate(p, errBoom)
		}()
	}
}

func TestErrorRate_Panics_NilErr(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil err")
		}
	}()
	fault.ErrorRate(0.5, nil)
}

// ---- LatencyP50 ----

func TestLatencyP50_AddsDelay(t *testing.T) {
	r := middleware.Chain(echo(), fault.LatencyP50(10*time.Millisecond, 20*time.Millisecond, fault.WithSource(rand.NewSource(1))))
	start := time.Now()
	if _, err := r.Invoke(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 1*time.Millisecond {
		t.Fatalf("expected nonzero delay, got %v", elapsed)
	}
}

func TestLatencyP50_CtxCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so sleep immediately aborts

	r := middleware.Chain(echo(), fault.LatencyP50(10*time.Second, 20*time.Second, fault.WithSource(rand.NewSource(5))))
	_, err := r.Invoke(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestLatencyP50_CtxCancelDuringSleep_Stream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := middleware.Chain(echo(), fault.LatencyP50(10*time.Second, 20*time.Second, fault.WithSource(rand.NewSource(5))))
	_, err := r.Stream(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestLatencyP50_Panics_InvalidMax(t *testing.T) {
	for _, max := range []time.Duration{0, -1} {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for max=%v", max)
				}
			}()
			fault.LatencyP50(0, max)
		}()
	}
}

func TestLatencyP50_Panics_MedianExceedsMax(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for median > max")
		}
	}()
	fault.LatencyP50(500*time.Millisecond, 100*time.Millisecond)
}

func TestLatencyP50_Panics_NegativeMedian(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for median < 0")
		}
	}()
	fault.LatencyP50(-1, 100*time.Millisecond)
}

func TestLatencyP50_MedianEqualsMax(t *testing.T) {
	// Edge case: median == max is valid (all calls take exactly max).
	r := middleware.Chain(echo(), fault.LatencyP50(10*time.Millisecond, 10*time.Millisecond, fault.WithSource(rand.NewSource(3))))
	if _, err := r.Invoke(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
