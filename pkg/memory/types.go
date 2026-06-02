package memory

import (
	"errors"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// TokenCounter estimates the token count for a message.
// Default implementation uses a rough 4-chars-per-token heuristic.
// Replace with a provider-specific tiktoken implementation for accuracy.
type TokenCounter func(msg llm.Message) int

// MemoryStoreOption configures a MemoryStore.
type MemoryStoreOption func(*MemoryStore)

// ErrStoreUnavailable is returned when the backing store cannot be reached.
var ErrStoreUnavailable = errors.New("message store unavailable")
