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
func NewRunnableFunc(name string, fn func(ctx context.Context, input any) (any, error)) *RunnableFunc {
	return &RunnableFunc{name: name, invoke: fn}
}

func (r *RunnableFunc) Name() string { return r.name }

func (r *RunnableFunc) Invoke(ctx context.Context, input any) (any, error) {
	return r.invoke(ctx, input)
}

func (r *RunnableFunc) Stream(ctx context.Context, input any) (<-chan StreamChunk, error) {
	if r.stream != nil {
		return r.stream(ctx, input)
	}
	ch := make(chan StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := r.invoke(ctx, input)
		ch <- StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}
