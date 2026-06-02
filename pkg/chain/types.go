// Package chain provides sequential Runnable composition.
//
// A Chain executes Steps in order, threading the output of each step as
// the input of the next. Each step can specify its own ModelConfig, enabling
// fast-model → reasoning-model patterns within a single chain.
//
// Chain itself implements the Runnable interface, so chains compose recursively.
package chain

import (
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// Step is one element of a Chain.
type Step struct {
	// ID identifies the step in traces and journal entries.
	ID string
	// Runnable is the unit of work for this step.
	Runnable core.Runnable
	// Model optionally specifies which LLM model this step should use.
	Model *llm.ModelConfig
}

// S is a convenience constructor for a Step without a model config.
func S(id string, r core.Runnable) Step {
	return Step{ID: id, Runnable: r}
}

// SWithModel constructs a Step with an explicit model config.
func SWithModel(id string, r core.Runnable, model llm.ModelConfig) Step {
	return Step{ID: id, Runnable: r, Model: &model}
}
