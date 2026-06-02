package graph

import "context"

// StaticRouter always routes to the same next node.
func StaticRouter(next string) Router {
	return func(_ context.Context, _ any) (string, error) { return next, nil }
}

// ConditionalRouter routes based on the output's type or value.
// routes maps a string key (returned by keyFn) to a node ID.
// Returns END if no route matches.
func ConditionalRouter(keyFn func(output any) string, routes map[string]string) Router {
	return func(_ context.Context, output any) (string, error) {
		key := keyFn(output)
		next, ok := routes[key]
		if !ok {
			return END, nil
		}
		return next, nil
	}
}
