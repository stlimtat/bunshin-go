// Package fault provides the FaultInjector middleware for HA and chaos testing.
//
// FaultInjector wraps a Runnable and injects configurable failures to simulate
// real-world failure modes:
//   - Transient errors (rate limits, timeouts, network blips)
//   - Added latency (slow provider responses)
//   - Partial failures (some calls succeed, some fail)
//
// Use in integration tests to verify that retry, fallback, and recovery
// mechanisms behave correctly under adversity.
//
// Example:
//
//	chain := middleware.Chain(myLLMRunnable,
//	    fault.WithFaultInjection(fault.Config{
//	        ErrorRate:  0.3,
//	        ErrorTypes: []fault.ErrorType{fault.Timeout, fault.RateLimit},
//	        LatencyP99: 500 * time.Millisecond,
//	    }),
//	    middleware.WithRetry(...),
//	)
package fault

import (
	"context"
	"errors"
	"time"
)

// ErrorType classifies injected errors by the failure mode they simulate.
type ErrorType string

const (
	// Timeout simulates context.DeadlineExceeded.
	Timeout ErrorType = "timeout"
	// RateLimit simulates HTTP 429 / provider rate limit responses.
	RateLimit ErrorType = "rate_limit"
	// Unavailable simulates HTTP 503 / service unavailable.
	Unavailable ErrorType = "unavailable"
	// InternalError simulates HTTP 500 / unexpected provider errors.
	InternalError ErrorType = "internal_error"
)

// errorMessages maps each ErrorType to its sentinel error value.
var errorMessages = map[ErrorType]error{
	Timeout:       context.DeadlineExceeded,
	RateLimit:     errors.New("rate limit exceeded (429)"),
	Unavailable:   errors.New("service unavailable (503)"),
	InternalError: errors.New("internal provider error (500)"),
}

// Config controls fault injection behaviour.
type Config struct {
	// ErrorRate is the probability [0.0, 1.0] that any given call returns an error.
	ErrorRate float64
	// ErrorTypes lists the error types to inject. Defaults to [Unavailable] if empty.
	ErrorTypes []ErrorType
	// LatencyP99 adds random latency up to this value to every call.
	LatencyP99 time.Duration
	// Seed for the random number generator. 0 uses a time-based seed.
	Seed int64
}
