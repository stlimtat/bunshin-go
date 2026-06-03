package llm

import "context"

// Pinger is implemented by providers that support connectivity health checks.
// HTTPTransport uses this to probe provider availability via /ready endpoint.
type Pinger interface {
	// Ping makes a lightweight call to verify the provider is reachable and
	// the API key is valid. Returns nil on success.
	Ping(ctx context.Context) error
}
