// Package probe exposes health, liveness, readiness, Prometheus metrics, and
// pprof endpoints.
//
// Callers register Checker implementations for liveness and readiness. The
// HTTP handlers aggregate all registered checkers and return 200 only when all
// pass.
//
// Endpoints carry no API version prefix:
//
//	GET /healthz    — aggregate liveness check
//	GET /readyz     — aggregate readiness check
//	GET /metrics    — Prometheus metrics
//	GET /debug/pprof/* — Go pprof profiling
package probe

import "context"

// Checker is a named health check. Register implementations via Handler.
type Checker interface {
	// Check returns nil when the component is healthy.
	Check(ctx context.Context) error
}

// CheckerFunc adapts a plain function to the Checker interface.
type CheckerFunc func(ctx context.Context) error

func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }
