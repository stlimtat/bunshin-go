// Package core defines the fundamental interfaces for bunshin-go.
//
// The central abstraction is Runnable — a composable unit of work that
// accepts input, produces output, and can be streamed. Runnables compose
// into chains, graphs, and agent loops.
//
// Two-layer design:
//   - TypedRunnable[In, Out] — type-safe at definition time
//   - Runnable — untyped shell for composition and middleware
//
// Use AsRunnable to bridge between the two layers.
package core

import "context"

// Runnable is the composable unit of work in bunshin-go.
// Every chain step, LLM call, tool, and prompt renderer implements this interface.
// Input and output are typed as any to allow heterogeneous composition;
// use TypedRunnable for internal type safety.
type Runnable interface {
	// Name returns the identifier used in traces and logs.
	Name() string

	// Invoke executes the runnable synchronously and returns the result.
	Invoke(ctx context.Context, input any) (any, error)

	// Stream executes the runnable and emits partial results as they arrive.
	// The channel is closed when the runnable completes or ctx is cancelled.
	Stream(ctx context.Context, input any) (<-chan StreamChunk, error)
}

// TypedRunnable is the type-safe inner layer. Define your Runnables against
// this interface; wrap with AsRunnable to use them in chains.
type TypedRunnable[In, Out any] interface {
	Invoke(ctx context.Context, input In) (Out, error)
}

// TypedStreamRunnable extends TypedRunnable with streaming support.
type TypedStreamRunnable[In, Out any] interface {
	TypedRunnable[In, Out]
	Stream(ctx context.Context, input In) (<-chan Out, error)
}
