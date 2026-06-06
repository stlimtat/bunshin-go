package graph

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// StaticRouter always routes to the same next node.
func StaticRouter[S any](next string) Router[S] {
	return func(_ context.Context, _ core.State[S]) (string, error) { return next, nil }
}

// ConditionalRouter routes based on a key derived from the current state.
// routes maps a string key (returned by keyFn) to a node ID.
// Returns END if no route matches.
func ConditionalRouter[S any](keyFn func(core.State[S]) string, routes map[string]string) Router[S] {
	return func(_ context.Context, s core.State[S]) (string, error) {
		key := keyFn(s)
		next, ok := routes[key]
		if !ok {
			return END, nil
		}
		return next, nil
	}
}
