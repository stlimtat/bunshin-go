package middleware

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// ErrMaxIterationsExceeded is returned when a Runnable is invoked more times
// than the WithMaxIterations limit allows.
var ErrMaxIterationsExceeded = errors.New("max iterations exceeded")

// ErrTokenBudgetExceeded is returned when the token budget has been depleted.
var ErrTokenBudgetExceeded = errors.New("token budget exceeded")

// WithMaxIterations limits how many times the wrapped Runnable may be invoked.
// On the (max+1)th call, Invoke returns ErrMaxIterationsExceeded and the
// Runnable is never reached. Stream behaves identically.
//
// Use at L1 (Workflow) to bound agent loops that might otherwise run forever.
//
//	safeAgent := middleware.Chain(agent, middleware.WithMaxIterations(10))
func WithMaxIterations(max int) Middleware {
	return func(next core.Runnable) core.Runnable {
		var count atomic.Int64
		invoke := func(ctx context.Context, input any) (any, error) {
			n := count.Add(1)
			if n > int64(max) {
				return nil, fmt.Errorf("%w: limit %d", ErrMaxIterationsExceeded, max)
			}
			return next.Invoke(ctx, input)
		}
		stream := func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
			n := count.Add(1)
			if n > int64(max) {
				return nil, fmt.Errorf("%w: limit %d", ErrMaxIterationsExceeded, max)
			}
			return next.Stream(ctx, input)
		}
		return core.NewRunnableFuncWithStream(next.Name(), invoke, stream)
	}
}

// TokenBudgetTracker is a shared token counter that can be embedded in
// context.Context and decremented by multiple steps in a workflow.
// Construct one with NewTokenBudgetTracker and inject it via WithTokenBudget.
type TokenBudgetTracker struct {
	remaining atomic.Int64
}

// NewTokenBudgetTracker creates a tracker with the given initial budget.
func NewTokenBudgetTracker(budget int64) *TokenBudgetTracker {
	t := &TokenBudgetTracker{}
	t.remaining.Store(budget)
	return t
}

// Remaining returns the current remaining token budget.
func (t *TokenBudgetTracker) Remaining() int64 {
	return t.remaining.Load()
}

// Consume subtracts n tokens from the budget. Returns ErrTokenBudgetExceeded
// if the budget would drop below zero.
func (t *TokenBudgetTracker) Consume(n int64) error {
	for {
		cur := t.remaining.Load()
		if cur-n < 0 {
			return ErrTokenBudgetExceeded
		}
		if t.remaining.CompareAndSwap(cur, cur-n) {
			return nil
		}
	}
}

type budgetKey struct{}

// WithTokenBudget injects tracker into the context and, if extractor is non-nil,
// decrements the budget after each successful Invoke using the token count
// returned by extractor(output). If the budget is exhausted, the next call
// returns ErrTokenBudgetExceeded before reaching the wrapped Runnable.
//
//	tracker := middleware.NewTokenBudgetTracker(100_000)
//	safe := middleware.Chain(llmNode, middleware.WithTokenBudget(tracker, func(out any) int64 {
//	    resp := out.(*llm.Response)
//	    return int64(resp.Usage.TotalTokens)
//	}))
func WithTokenBudget(tracker *TokenBudgetTracker, extractor func(any) int64) Middleware {
	return func(next core.Runnable) core.Runnable {
		invoke := func(ctx context.Context, input any) (any, error) {
			if tracker.Remaining() <= 0 {
				return nil, ErrTokenBudgetExceeded
			}
			ctx = context.WithValue(ctx, budgetKey{}, tracker)
			out, err := next.Invoke(ctx, input)
			if err == nil && extractor != nil {
				if cerr := tracker.Consume(extractor(out)); cerr != nil {
					return out, cerr
				}
			}
			return out, err
		}
		stream := func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
			if tracker.Remaining() <= 0 {
				return nil, ErrTokenBudgetExceeded
			}
			ctx = context.WithValue(ctx, budgetKey{}, tracker)
			return next.Stream(ctx, input)
		}
		return core.NewRunnableFuncWithStream(next.Name(), invoke, stream)
	}
}

// BudgetFromContext retrieves the TokenBudgetTracker injected by WithTokenBudget.
// Returns nil if no tracker is present.
func BudgetFromContext(ctx context.Context) *TokenBudgetTracker {
	t, _ := ctx.Value(budgetKey{}).(*TokenBudgetTracker)
	return t
}
