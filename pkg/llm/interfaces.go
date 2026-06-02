// Package llm defines the LLM provider abstraction and canonical message types.
//
// Key design decisions:
//   - ProviderID + ModelTier allow per-step model selection in workflows.
//   - CanTransferContext enables same-provider optimisation: when consecutive
//     workflow steps use the same provider, WindowFor skips translation and
//     passes the native message format directly, avoiding serialisation overhead.
//   - Message.native caches translated representations so cross-provider
//     translation is paid at most once per message.
package llm

import "context"

// LLMProvider is the interface every model adapter implements.
type LLMProvider interface {
	// ID returns the provider identifier.
	ID() ProviderID

	// Complete sends a request and returns the full response.
	Complete(ctx context.Context, req *Request) (*Response, error)

	// StreamComplete sends a request and returns a channel of incremental chunks.
	// The channel is closed after the final chunk (Chunk.Done == true).
	StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error)

	// CanTransferContext returns true if this provider can natively consume
	// context produced by from. When true, WindowFor skips canonical translation.
	CanTransferContext(from LLMProvider) bool

	// NativeMessages converts canonical Messages to the provider's wire format.
	// Used by MessageStore.WindowFor for same-provider fast-path and by the
	// translation layer for cross-provider handoffs.
	NativeMessages(msgs []Message) (any, error)
}
