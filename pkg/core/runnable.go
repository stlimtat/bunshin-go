package core

import "context"

// RunnableFunc adapts a plain function to the Runnable interface.
// Useful for quick one-off Runnables without defining a struct.
type RunnableFunc struct {
	name   string
	invoke func(ctx context.Context, input any) (any, error)
	stream func(ctx context.Context, input any) (<-chan StreamChunk, error)
}

// NewRunnableFunc constructs a Runnable from an invoke function.
// Stream defaults to wrapping Invoke in a single-chunk channel.
// Panics if fn is nil — a nil invoke is a programming error.
func NewRunnableFunc(name string, fn func(ctx context.Context, input any) (any, error)) *RunnableFunc {
	if fn == nil {
		panic("core: NewRunnableFunc: invoke function must not be nil")
	}
	return &RunnableFunc{name: name, invoke: fn}
}

// NewRunnableFuncWithStream constructs a Runnable with explicit invoke and stream functions.
// If streamFn is nil, Stream falls back to the single-chunk wrapper identical to NewRunnableFunc.
// Panics if fn is nil — a nil invoke is a programming error.
func NewRunnableFuncWithStream(
	name string,
	fn func(ctx context.Context, input any) (any, error),
	streamFn func(ctx context.Context, input any) (<-chan StreamChunk, error),
) *RunnableFunc {
	if fn == nil {
		panic("core: NewRunnableFuncWithStream: invoke function must not be nil")
	}
	return &RunnableFunc{name: name, invoke: fn, stream: streamFn}
}

func (r *RunnableFunc) Name() string { return r.name }

func (r *RunnableFunc) Invoke(ctx context.Context, input any) (any, error) {
	return r.invoke(ctx, input)
}

func (r *RunnableFunc) Stream(ctx context.Context, input any) (<-chan StreamChunk, error) {
	if r.stream != nil {
		return r.stream(ctx, input)
	}
	// Buffer=1: single result, send never blocks regardless of consumer timing.
	ch := make(chan StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := r.invoke(ctx, input)
		ch <- StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}
