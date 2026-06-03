package telemetry

import "context"

// Pinger is implemented by components that support lightweight connectivity checks.
// ProviderRegistry uses this to probe LLM provider availability.
type Pinger interface {
	// Ping makes a minimal call to verify the endpoint is reachable and credentials
	// are valid. Returns nil on success.
	Ping(ctx context.Context) error
}
