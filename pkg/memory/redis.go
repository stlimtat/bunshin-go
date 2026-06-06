package memory

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// RedisMessageStore persists messages in Redis as a JSON list.
// Messages are stored under the key "{tenantID}:messages:{threadID}" using
// RPUSH (append) and LRANGE (read). This is an ordered list — message ordering
// is preserved exactly as written.
//
// Redis is a suitable backend for short-to-medium conversation histories.
// For very long conversations (>2M tokens) use PostgresMessageStore or MinIOMessageStore.
type RedisMessageStore struct {
	client       *redis.Client
	key          string
	tokenCounter TokenCounter
	originProvider llm.LLMProvider
}

// NewRedisMessageStore constructs a store scoped to a single thread/tenant.
func NewRedisMessageStore(client *redis.Client, threadID, tenantID string, opts ...MemoryStoreOption) *RedisMessageStore {
	dummy := &MemoryStore{tokenCounter: defaultTokenCount}
	for _, o := range opts {
		o(dummy)
	}
	return &RedisMessageStore{
		client:         client,
		key:            fmt.Sprintf("%s:messages:%s", tenantID, threadID),
		tokenCounter:   dummy.tokenCounter,
		originProvider: dummy.originProvider,
	}
}

// Append serialises msg as JSON and appends it to the Redis list.
func (s *RedisMessageStore) Append(ctx context.Context, msg llm.Message) error {
	data, err := marshal(msg)
	if err != nil {
		return err
	}
	if err := s.client.RPush(ctx, s.key, data).Err(); err != nil {
		return fmt.Errorf("redis message store: rpush: %w", err)
	}
	return nil
}

// Window returns up to maxTokens tokens of the most recent messages.
func (s *RedisMessageStore) Window(ctx context.Context, maxTokens int) ([]llm.Message, error) {
	items, err := s.client.LRange(ctx, s.key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis message store: lrange: %w", err)
	}

	msgs := make([]llm.Message, 0, len(items))
	for _, item := range items {
		msg, err := unmarshal([]byte(item))
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return applyWindow(msgs, maxTokens, s.tokenCounter), nil
}

// WindowFor returns a provider-native Request.
func (s *RedisMessageStore) WindowFor(ctx context.Context, p llm.LLMProvider, maxTokens int) (*llm.Request, error) {
	msgs, err := s.Window(ctx, maxTokens)
	if err != nil {
		return nil, err
	}
	return &llm.Request{Messages: msgs}, nil
}

// Len returns the number of messages stored.
func (s *RedisMessageStore) Len() int {
	n, _ := s.client.LLen(context.Background(), s.key).Result()
	return int(n)
}

// Snapshot is a no-op — Redis writes are durable on Append.
func (s *RedisMessageStore) Snapshot(_ context.Context) error { return nil }

// Restore is a no-op — messages are always read from Redis.
func (s *RedisMessageStore) Restore(_ context.Context) error { return nil }
