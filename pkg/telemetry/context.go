package telemetry

import "context"

type contextKey struct{}

// WithContext stores a RunContext in ctx.
func WithContext(ctx context.Context, rc RunContext) context.Context {
	return context.WithValue(ctx, contextKey{}, rc)
}

// FromContext retrieves the RunContext from ctx.
// Returns (RunContext{}, false) if none was stored.
func FromContext(ctx context.Context) (RunContext, bool) {
	rc, ok := ctx.Value(contextKey{}).(RunContext)
	return rc, ok
}

// MustFromContext retrieves the RunContext or panics.
// Use in internal node code where the middleware contract guarantees presence.
func MustFromContext(ctx context.Context) RunContext {
	rc, ok := FromContext(ctx)
	if !ok {
		panic("telemetry: no RunContext in context — did you forget WithLangSmith middleware?")
	}
	return rc
}
