package memory

import (
	"context"
	"sync"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// MemoryThreadRegistry is an in-process ThreadRegistry backed by MemoryStore instances.
// Suitable for development and testing; use a persistent registry in production.
type MemoryThreadRegistry struct {
	mu      sync.Mutex
	threads map[string]*MemoryStore
}

// NewMemoryThreadRegistry returns an empty in-memory registry.
func NewMemoryThreadRegistry() *MemoryThreadRegistry {
	return &MemoryThreadRegistry{threads: make(map[string]*MemoryStore)}
}

// List returns all known thread IDs.
func (r *MemoryThreadRegistry) List(_ context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.threads))
	for id := range r.threads {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetOrCreate returns the MessageStore for threadID, creating it if absent.
func (r *MemoryThreadRegistry) GetOrCreate(_ context.Context, threadID string) (MessageStore, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.threads[threadID]; ok {
		return s, nil
	}
	s := NewMemoryStore()
	r.threads[threadID] = s
	return s, nil
}

// defaultTokenCount estimates tokens: 1 per 4 characters of text content.
func defaultTokenCount(msg llm.Message) int {
	n := len(msg.Text())
	if n == 0 {
		return 1
	}
	t := n / 4
	if t == 0 {
		return 1
	}
	return t
}

// WithTokenCounter overrides the default token estimation function.
func WithTokenCounter(fn TokenCounter) MemoryStoreOption {
	return func(s *MemoryStore) { s.tokenCounter = fn }
}

// WithOriginProvider records which provider this store's messages come from.
// Enables WindowFor same-provider optimisation.
func WithOriginProvider(p llm.LLMProvider) MemoryStoreOption {
	return func(s *MemoryStore) { s.originProvider = p }
}

// MemoryStore is an in-process MessageStore backed by a slice.
// Safe for concurrent use. Useful for tests and short-lived workflows.
type MemoryStore struct {
	mu           sync.RWMutex
	messages     []llm.Message
	tokenCounter TokenCounter
	// originProvider tracks which provider produced messages in this store.
	// Used by WindowFor to apply the same-provider fast-path.
	originProvider llm.LLMProvider
}

// NewMemoryStore constructs an empty MemoryStore.
func NewMemoryStore(opts ...MemoryStoreOption) *MemoryStore {
	s := &MemoryStore{tokenCounter: defaultTokenCount}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *MemoryStore) Append(_ context.Context, msg llm.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return nil
}

func (s *MemoryStore) Window(_ context.Context, maxTokens int) ([]llm.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if maxTokens <= 0 {
		out := make([]llm.Message, len(s.messages))
		copy(out, s.messages)
		return out, nil
	}

	// Walk backwards, collecting messages until budget exhausted.
	budget := maxTokens
	start := len(s.messages)
	for i := len(s.messages) - 1; i >= 0; i-- {
		cost := s.tokenCounter(s.messages[i])
		if budget-cost < 0 {
			break
		}
		budget -= cost
		start = i
	}

	out := make([]llm.Message, len(s.messages)-start)
	copy(out, s.messages[start:])
	return out, nil
}

func (s *MemoryStore) WindowFor(ctx context.Context, p llm.LLMProvider, maxTokens int) (*llm.Request, error) {
	msgs, err := s.Window(ctx, maxTokens)
	if err != nil {
		return nil, err
	}

	// Pre-warm the native cache on each message so the provider adapter
	// can skip translation when building the wire-format request.
	for i := range msgs {
		if _, ok := msgs[i].Native(p.ID()); !ok {
			native, err := p.NativeMessages([]llm.Message{msgs[i]})
			if err != nil {
				return nil, err
			}
			msgs[i].CacheNative(p.ID(), native)
		}
	}

	return &llm.Request{Messages: msgs}, nil
}

func (s *MemoryStore) Snapshot(_ context.Context) error { return nil }
func (s *MemoryStore) Restore(_ context.Context) error  { return nil }

func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}
