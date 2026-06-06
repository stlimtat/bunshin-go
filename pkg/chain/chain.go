package chain

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// Chain[S] executes steps sequentially, threading State[S] through each step.
// Output of step N becomes input to step N+1.
// Chain[S] implements TypedRunnable[State[S], State[S]] and can nest inside
// other chains or graphs via Chain.AsRunnable().
type Chain[S any] struct {
	name  string
	steps []Step[S]
}

// New constructs a Chain with the given steps.
func New[S any](name string, steps ...Step[S]) *Chain[S] {
	return &Chain[S]{name: name, steps: steps}
}

// Name returns the chain identifier used in traces and logs.
func (c *Chain[S]) Name() string { return c.name }

// Steps returns the chain's steps (read-only slice).
func (c *Chain[S]) Steps() []Step[S] { return c.steps }

// Invoke executes all steps in order, threading State[S] as both input and output.
// Returns ctx.Err() immediately if the context is cancelled between steps.
// On step error, returns the last successfully computed state alongside the error.
func (c *Chain[S]) Invoke(ctx context.Context, input core.State[S]) (core.State[S], error) {
	current := input
	for _, step := range c.steps {
		if err := ctx.Err(); err != nil {
			return current, err
		}
		out, err := step.Runnable.Invoke(ctx, current)
		if err != nil {
			return current, fmt.Errorf("chain %q step %q: %w", c.name, step.ID, err)
		}
		current = out
	}
	return current, nil
}

// AsRunnable wraps the chain as a core.Runnable for middleware composition.
func (c *Chain[S]) AsRunnable() core.Runnable {
	return core.AsRunnable[core.State[S], core.State[S]](c.name, c)
}
