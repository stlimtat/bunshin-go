// Package middleware provides the bunshin-go middleware system.
//
// A Middleware is a function that wraps a Runnable, adding cross-cutting
// behaviour without modifying the Runnable itself. The pattern mirrors
// net/http middleware: each layer calls the next, unwinding on return.
//
// Five interception levels (outermost → innermost):
//
//	L1 Workflow  — auth, rate limiting, cost budget, distributed trace start
//	L2 Runnable  — step timing, structured logging, panic recovery, validation
//	L3 Prompt    — PII scrubbing, injection guard, context window trimming
//	L4 LLM call  — semantic cache, retry/backoff, model fallback, token counting
//	L5 Tool      — tool authorisation, sandboxing, result size limits
//
// Apply middlewares with Chain. Middlewares are applied right-to-left so the
// first middleware in the list is the outermost wrapper (called first).
package middleware

import (
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// Middleware wraps a Runnable with additional behaviour.
type Middleware func(core.Runnable) core.Runnable

// RetryConfig controls the retry behaviour of WithRetry.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (including the first).
	MaxAttempts int
	// InitialDelay is the wait time before the second attempt.
	InitialDelay time.Duration
	// Multiplier multiplies the delay after each failure (exponential backoff).
	// Use 1.0 for constant delay.
	Multiplier float64
	// RetryIf, if non-nil, is called with each error to decide whether to retry.
	// Defaults to retrying all errors.
	RetryIf func(error) bool
}
