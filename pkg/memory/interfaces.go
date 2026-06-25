// Package memory defines the MessageStore interface and in-process backends.
//
// MessageStore holds conversation history as a reference/cursor rather than
// materialising all messages in memory. This keeps State[S] small regardless
// of conversation length — even 2M token contexts.
//
// Window returns the most recent messages that fit within a token budget.
// WindowFor additionally performs same-provider fast-path: if the target provider
// matches the store's origin provider, it returns the native wire format directly,
// bypassing canonical translation overhead.
package memory

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// MessageStore is the interface for conversation history backends.
// All methods are safe for concurrent use.
type MessageStore interface {
	// Append adds a message to the end of the history.
	Append(ctx context.Context, msg llm.Message) error

	// Window returns up to maxTokens tokens worth of the most recent messages,
	// newest-last. maxTokens <= 0 returns all messages.
	Window(ctx context.Context, maxTokens int) ([]llm.Message, error)

	// WindowFor returns a provider-native Request for the given provider.
	// If p.CanTransferContext(originProvider), the native format is used directly.
	// Otherwise messages are translated through the canonical format.
	WindowFor(ctx context.Context, p llm.LLMProvider, maxTokens int) (*llm.Request, error)

	// Snapshot persists the current state to the backing store.
	// No-op for in-memory backends.
	Snapshot(ctx context.Context) error

	// Restore loads state from the backing store.
	// No-op for in-memory backends.
	Restore(ctx context.Context) error

	// Len returns the total number of messages stored.
	Len() int
}

// ThreadRegistry manages per-thread MessageStores.
// All methods are safe for concurrent use.
type ThreadRegistry interface {
	// List returns the IDs of all known threads.
	List(ctx context.Context) ([]string, error)

	// GetOrCreate returns the MessageStore for threadID, creating one if absent.
	GetOrCreate(ctx context.Context, threadID string) (MessageStore, error)
}
