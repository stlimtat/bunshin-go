package telemetry

import (
	"context"

	"github.com/google/uuid"
)

// contextKey is the unexported key type for run IDs in context.
type contextKey struct{}

// WithRunID injects a run ID into ctx for child run parenting.
func WithRunID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// RunIDFromContext extracts the current run ID from ctx.
// Returns uuid.Nil if no run ID is present.
func RunIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(contextKey{}).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}
