package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/stlimtat/bunshin-go/pkg/core"
)

// Chain applies middlewares to r from right to left, so that the first
// middleware in mw is outermost (executed first on Invoke).
//
//	Chain(r, A, B, C) → A(B(C(r)))
func Chain(r core.Runnable, mw ...Middleware) core.Runnable {
	for i := len(mw) - 1; i >= 0; i-- {
		r = mw[i](r)
	}
	return r
}

// WithLogging logs the name, duration, and error (if any) for every Invoke call.
// Stream calls are delegated to next without log wrapping to preserve token-by-token latency.
func WithLogging(logger zerolog.Logger) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				start := time.Now()
				out, err := next.Invoke(ctx, input)
				dur := time.Since(start)
				if err != nil {
					logger.Error().
						Str("runnable", next.Name()).
						Dur("duration", dur).
						Err(err).
						Msg("runnable error")
				} else {
					logger.Info().
						Str("runnable", next.Name()).
						Dur("duration", dur).
						Msg("runnable ok")
				}
				return out, err
			},
			next.Stream,
		)
	}
}

// WithPanicRecovery catches panics from next and converts them to errors.
// Without this, a panicking tool or LLM adapter can crash the whole process.
func WithPanicRecovery() Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (out any, err error) {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("runnable %q panicked: %v", next.Name(), r)
					}
				}()
				return next.Invoke(ctx, input)
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				inner, err := next.Stream(ctx, input)
				if err != nil {
					return nil, err
				}
				out := make(chan core.StreamChunk)
				name := next.Name()
				go func() {
					defer close(out)
					defer func() {
						if r := recover(); r != nil {
							select {
							case out <- core.StreamChunk{Err: fmt.Errorf("runnable %q stream goroutine panicked: %v", name, r)}:
							case <-ctx.Done():
							}
						}
					}()
					for chunk := range inner {
						select {
						case out <- chunk:
						case <-ctx.Done():
							return
						}
					}
				}()
				return out, nil
			},
		)
	}
}

// WithRetry retries next.Invoke on error according to cfg.
// Context cancellation stops retrying immediately.
// Stream is delegated without retry — partial streams cannot be replayed.
func WithRetry(cfg RetryConfig) Middleware {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	if cfg.RetryIf == nil {
		cfg.RetryIf = func(error) bool { return true }
	}
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				delay := cfg.InitialDelay
				var lastErr error
				for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
					if attempt > 0 {
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-time.After(delay):
						}
						delay = time.Duration(float64(delay) * cfg.Multiplier)
					}
					out, err := next.Invoke(ctx, input)
					if err == nil {
						return out, nil
					}
					lastErr = err
					if !cfg.RetryIf(err) {
						return nil, err
					}
				}
				return nil, fmt.Errorf("all %d attempts failed: %w", cfg.MaxAttempts, lastErr)
			},
			next.Stream,
		)
	}
}

// WithTimeout adds a per-invocation deadline to each Invoke and Stream call.
func WithTimeout(d time.Duration) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				ctx, cancel := context.WithTimeout(ctx, d)
				defer cancel()
				return next.Invoke(ctx, input)
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				ctx, cancel := context.WithTimeout(ctx, d)
				ch, err := next.Stream(ctx, input)
				if err != nil {
					cancel()
					return nil, err
				}
				wrapped := make(chan core.StreamChunk)
				go func() {
					defer close(wrapped)
					defer cancel()
					for {
						select {
						case chunk, ok := <-ch:
							if !ok {
								return
							}
							select {
							case wrapped <- chunk:
							case <-ctx.Done():
								select {
								case wrapped <- core.StreamChunk{Err: ctx.Err()}:
								default:
								}
								return
							}
						case <-ctx.Done():
							select {
							case wrapped <- core.StreamChunk{Err: ctx.Err()}:
							default:
							}
							return
						}
					}
				}()
				return wrapped, nil
			},
		)
	}
}

// WithMetadata injects key-value pairs into the context before calling next.
// Useful at the Workflow (L1) boundary to attach trace IDs and session tokens.
func WithMetadata(kv ...any) Middleware {
	return func(next core.Runnable) core.Runnable {
		inject := func(ctx context.Context) context.Context {
			for i := 0; i+1 < len(kv); i += 2 {
				ctx = context.WithValue(ctx, kv[i], kv[i+1])
			}
			return ctx
		}
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				return next.Invoke(inject(ctx), input)
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				return next.Stream(inject(ctx), input)
			},
		)
	}
}
