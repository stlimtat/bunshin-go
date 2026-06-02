package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
// Uses log/slog so callers can inject their own handler.
func WithLogging(logger *slog.Logger) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
			start := time.Now()
			out, err := next.Invoke(ctx, input)
			dur := time.Since(start)
			if err != nil {
				logger.ErrorContext(ctx, "runnable error",
					slog.String("runnable", next.Name()),
					slog.Duration("duration", dur),
					slog.String("error", err.Error()),
				)
			} else {
				logger.InfoContext(ctx, "runnable ok",
					slog.String("runnable", next.Name()),
					slog.Duration("duration", dur),
				)
			}
			return out, err
		})
	}
}

// WithPanicRecovery catches panics from next and converts them to errors.
// Without this, a panicking tool or LLM adapter can crash the whole process.
func WithPanicRecovery() Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (out any, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("runnable %q panicked: %v", next.Name(), r)
				}
			}()
			return next.Invoke(ctx, input)
		})
	}
}

// WithRetry retries next on error according to cfg.
// Context cancellation stops retrying immediately.
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
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
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
		})
	}
}

// WithTimeout adds a per-invocation deadline to each Invoke call.
func WithTimeout(d time.Duration) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next.Invoke(ctx, input)
		})
	}
}

// WithMetadata injects key-value pairs into the context before calling next.
// Useful at the Workflow (L1) boundary to attach trace IDs and session tokens.
func WithMetadata(kv ...any) Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFunc(next.Name(), func(ctx context.Context, input any) (any, error) {
			for i := 0; i+1 < len(kv); i += 2 {
				ctx = context.WithValue(ctx, kv[i], kv[i+1])
			}
			return next.Invoke(ctx, input)
		})
	}
}
