package fault

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

// ErrorRate returns a [middleware.Middleware] that fails a random fraction of
// calls with err. Each Invoke and Stream call independently draws a uniform
// random value; if it falls below p the call fails immediately without
// invoking the wrapped Runnable.
//
// p must be in [0, 1]. Panics otherwise.
// err must be non-nil. Panics otherwise.
func ErrorRate(p float64, err error, opts ...Option) middleware.Middleware {
	if p < 0 || p > 1 {
		panic("fault.ErrorRate: p must be in [0, 1]")
	}
	if err == nil {
		panic("fault.ErrorRate: err must be non-nil")
	}
	rng := newRand(opts)
	var mu sync.Mutex

	should := func() bool {
		mu.Lock()
		v := rng.Float64()
		mu.Unlock()
		return v < p
	}

	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				if should() {
					return nil, err
				}
				return next.Invoke(ctx, input)
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				if should() {
					return nil, err
				}
				return next.Stream(ctx, input)
			},
		)
	}
}

// LatencyP50 returns a [middleware.Middleware] that sleeps for a
// triangular-distributed duration on [0, max] with peak at median before each
// Invoke and Stream call. The median parameter is the true P50 of the
// distribution.
//
// max must be > 0. Panics otherwise.
// median must be in [0, max]. Panics otherwise.
//
// Sleep honours context cancellation: if ctx is cancelled before the sleep
// completes, the middleware returns ctx.Err() without calling the wrapped
// Runnable.
func LatencyP50(median, max time.Duration, opts ...Option) middleware.Middleware {
	if max <= 0 {
		panic("fault.LatencyP50: max must be > 0")
	}
	if median < 0 || median > max {
		panic("fault.LatencyP50: median must be in [0, max]")
	}
	rng := newRand(opts)
	var mu sync.Mutex

	sample := func() time.Duration {
		mu.Lock()
		u := rng.Float64()
		mu.Unlock()
		return triangular(u, 0, float64(max), float64(median))
	}

	sleep := func(ctx context.Context) error {
		d := sample()
		if d <= 0 {
			return nil
		}
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				if err := sleep(ctx); err != nil {
					return nil, err
				}
				return next.Invoke(ctx, input)
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				if err := sleep(ctx); err != nil {
					return nil, err
				}
				return next.Stream(ctx, input)
			},
		)
	}
}

// triangular samples from a triangular distribution on [lo, hi] with peak c
// given a uniform random value u in [0, 1).
// Reference: https://en.wikipedia.org/wiki/Triangular_distribution#Generating_triangular-distributed_random_variates
func triangular(u, lo, hi, c float64) time.Duration {
	span := hi - lo
	if span == 0 {
		return time.Duration(lo)
	}
	fc := (c - lo) / span
	var x float64
	if u < fc {
		x = lo + math.Sqrt(u*span*(c-lo))
	} else {
		x = hi - math.Sqrt((1-u)*span*(hi-c))
	}
	return time.Duration(x)
}
