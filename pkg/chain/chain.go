package chain

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// Chain executes steps sequentially.
// Output of step N becomes input of step N+1.
// Chain implements core.Runnable and can nest inside other chains or graphs.
type Chain struct {
	name  string
	steps []Step
}

// New constructs a Chain with the given steps.
func New(name string, steps ...Step) *Chain {
	return &Chain{name: name, steps: steps}
}

func (c *Chain) Name() string { return c.name }

// Steps returns the chain's steps (read-only).
func (c *Chain) Steps() []Step { return c.steps }

// Invoke executes all steps in order, threading outputs as inputs.
func (c *Chain) Invoke(ctx context.Context, input any) (any, error) {
	current := input
	for _, step := range c.steps {
		out, err := step.Runnable.Invoke(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("chain %q step %q: %w", c.name, step.ID, err)
		}
		current = out
	}
	return current, nil
}

// Stream executes all steps sequentially, streaming only the final step.
func (c *Chain) Stream(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
	if len(c.steps) == 0 {
		ch := make(chan core.StreamChunk)
		close(ch)
		return ch, nil
	}

	current := input
	for _, step := range c.steps[:len(c.steps)-1] {
		out, err := step.Runnable.Invoke(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("chain %q step %q: %w", c.name, step.ID, err)
		}
		current = out
	}

	last := c.steps[len(c.steps)-1]
	return last.Runnable.Stream(ctx, current)
}
