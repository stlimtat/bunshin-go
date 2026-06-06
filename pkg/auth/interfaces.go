// Package auth provides identity primitives for bunshin-go.
//
// Principal is the authenticated caller identity written to context.Context by
// auth middleware and read by WithRBAC and store implementations. TenantID
// scopes all multi-tenant data operations.
package auth

import "context"

type contextKey struct{}

// FromContext returns the Principal stored in ctx, and reports whether one was found.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

// MustFromContext returns the Principal stored in ctx.
// Panics if no Principal is present — use only in middleware-guarded handlers.
func MustFromContext(ctx context.Context) Principal {
	p, ok := FromContext(ctx)
	if !ok {
		panic("auth: no Principal in context")
	}
	return p
}

// WithContext returns a copy of ctx carrying p.
func WithContext(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
