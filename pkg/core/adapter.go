package core

import (
	"context"
	"fmt"
)

// typedAdapter wraps TypedRunnable[In, Out] as an untyped Runnable.
// Type assertion errors surface as runtime errors with descriptive messages.
type typedAdapter[In, Out any] struct {
	name  string
	inner TypedRunnable[In, Out]
}

// AsRunnable wraps a TypedRunnable as a Runnable, erasing the type parameters
// at the composition boundary. Type mismatches surface at call time via error return.
func AsRunnable[In, Out any](name string, r TypedRunnable[In, Out]) Runnable {
	return &typedAdapter[In, Out]{name: name, inner: r}
}

func (a *typedAdapter[In, Out]) Name() string { return a.name }

func (a *typedAdapter[In, Out]) Invoke(ctx context.Context, input any) (any, error) {
	typed, ok := input.(In)
	if !ok {
		return nil, fmt.Errorf("runnable %q: input type mismatch: got %T", a.name, input)
	}
	return a.inner.Invoke(ctx, typed)
}

func (a *typedAdapter[In, Out]) Stream(ctx context.Context, input any) (<-chan StreamChunk, error) {
	typed, ok := input.(In)
	if !ok {
		return nil, fmt.Errorf("runnable %q: input type mismatch: got %T", a.name, input)
	}

	// Check if inner supports streaming; fall back to wrapping Invoke.
	if sr, ok := a.inner.(TypedStreamRunnable[In, Out]); ok {
		inner, err := sr.Stream(ctx, typed)
		if err != nil {
			return nil, err
		}
		ch := make(chan StreamChunk)
		go func() {
			defer close(ch)
			for v := range inner {
				select {
				case ch <- StreamChunk{Value: v}:
				case <-ctx.Done():
					select {
					case ch <- StreamChunk{Err: ctx.Err()}:
					default:
					}
					return
				}
			}
		}()
		return ch, nil
	}

	// Single-invoke fallback: buffer=1 so the send never blocks.
	ch := make(chan StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := a.inner.Invoke(ctx, typed)
		if ctx.Err() != nil {
			ch <- StreamChunk{Err: ctx.Err()}
			return
		}
		ch <- StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}
