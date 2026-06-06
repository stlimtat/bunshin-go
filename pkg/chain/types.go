// Package chain provides sequential Runnable composition.
//
// A Chain[S] executes Steps in order, threading State[S] through each step.
// Each step can specify its own ModelConfig, enabling fast-model → reasoning-model
// patterns within a single chain.
//
// Chain[S] implements TypedRunnable[State[S], State[S]] and can be nested inside
// other chains or graphs via core.AsRunnable.
package chain

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// Step[S] is one typed element of a Chain[S].
type Step[S any] struct {
	// ID identifies the step in traces and journal entries.
	ID string
	// Runnable is the unit of work for this step.
	Runnable core.TypedRunnable[core.State[S], core.State[S]]
	// Model optionally specifies which LLM model this step should use.
	Model *llm.ModelConfig
}

// Func wraps a plain function as a Step[S]. Convenience for tests and inline definitions.
func Func[S any](id string, fn func(context.Context, core.State[S]) (core.State[S], error)) Step[S] {
	return Step[S]{ID: id, Runnable: core.TypedFunc(fn)}
}

// FuncWithModel wraps a plain function as a Step[S] with an explicit model config.
func FuncWithModel[S any](id string, fn func(context.Context, core.State[S]) (core.State[S], error), model llm.ModelConfig) Step[S] {
	return Step[S]{ID: id, Runnable: core.TypedFunc(fn), Model: &model}
}
