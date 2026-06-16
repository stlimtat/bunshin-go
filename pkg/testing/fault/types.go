// Package fault provides middleware factories for chaos / HA testing.
//
// Two fault types are available, each independently composable via middleware.Chain:
//
//   - [ErrorRate] returns an error on a random fraction of calls.
//   - [LatencyP50] sleeps for a triangular-distributed duration before each call.
//
// Example:
//
//	r = middleware.Chain(r,
//	    fault.ErrorRate(0.3, context.DeadlineExceeded),
//	    fault.LatencyP50(100*time.Millisecond, 500*time.Millisecond),
//	)
//
// For deterministic tests, supply a seeded source:
//
//	fault.ErrorRate(0.5, errBoom, fault.WithSource(rand.NewSource(42)))
package fault

import (
	"math/rand"
	"time"
)

// Option configures a fault middleware instance.
type Option func(*faultConfig)

// WithSource replaces the default random source with src.
// Use rand.NewSource(seed) in tests for deterministic fault sequences.
func WithSource(src rand.Source) Option {
	return func(c *faultConfig) { c.src = src }
}

type faultConfig struct {
	src rand.Source
}

func newRand(opts []Option) *rand.Rand {
	cfg := &faultConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.src == nil {
		cfg.src = rand.NewSource(time.Now().UnixNano())
	}
	return rand.New(cfg.src) //nolint:gosec // fault injection uses math/rand intentionally
}
