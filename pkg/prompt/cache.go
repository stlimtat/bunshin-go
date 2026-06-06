package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// templateSnapshot is the immutable, compiled in-process view of all active
// fragments for a tenant. It is swapped atomically via atomic.Pointer.
type templateSnapshot struct {
	fragments map[string]*Fragment
}

// PromptCache is a two-level cache on top of a PromptBackend:
//
//  1. Redis (shared): raw fragment JSON, keyed "{tenantID}:prompt:{fragmentID}".
//     All compute nodes share one Redis — a single write propagates to everyone.
//
//  2. In-process snapshot: an atomic.Pointer to a compiled templateSnapshot.
//     The snapshot is refreshed on a 5-second poll interval or on demand via Refresh().
//
// Refresh() can be triggered manually (e.g. by the /v1/prompts/refresh HTTP handler)
// to force an immediate pull from Redis without waiting for the poll interval.
type PromptCache struct {
	backend  PromptBackend
	redis    *redis.Client
	tenantID string

	snapshot atomic.Pointer[templateSnapshot]
	mu       sync.Mutex // guards concurrent Refresh() calls

	refreshCh chan struct{}
	stopCh    chan struct{}
}

// NewPromptCache constructs a cache and starts the background poll goroutine.
// Call Close to stop the background goroutine.
func NewPromptCache(backend PromptBackend, rdb *redis.Client, tenantID string) *PromptCache {
	c := &PromptCache{
		backend:   backend,
		redis:     rdb,
		tenantID:  tenantID,
		refreshCh: make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}
	c.snapshot.Store(&templateSnapshot{fragments: make(map[string]*Fragment)})
	go c.poll()
	return c
}

// Get returns the cached active fragment, falling through to Redis then the backend.
func (c *PromptCache) Get(_ context.Context, id string) (*Fragment, error) {
	snap := c.snapshot.Load()
	if f, ok := snap.fragments[id]; ok {
		return f, nil
	}
	// Miss: check Redis.
	return c.fetchFromRedis(context.Background(), id)
}

// GetVersion bypasses the cache and reads directly from the backend.
func (c *PromptCache) GetVersion(ctx context.Context, id, version string) (*Fragment, error) {
	return c.backend.GetVersion(ctx, id, version)
}

// List returns cached active fragments. Tag filtering is applied in-process.
func (c *PromptCache) List(_ context.Context, tags ...string) ([]*Fragment, error) {
	snap := c.snapshot.Load()
	var out []*Fragment
	for _, f := range snap.fragments {
		if len(tags) == 0 || hasAllTags(f, tags) {
			out = append(out, f)
		}
	}
	return out, nil
}

// Watch delegates to the underlying backend.
func (c *PromptCache) Watch(ctx context.Context, id string) (<-chan *Fragment, error) {
	return c.backend.Watch(ctx, id)
}

// Refresh triggers an immediate pull from Redis into the in-process snapshot.
// Non-blocking: if a refresh is already in progress the request is coalesced.
func (c *PromptCache) Refresh() {
	select {
	case c.refreshCh <- struct{}{}:
	default:
	}
}

// Close stops the background poll goroutine.
func (c *PromptCache) Close() {
	close(c.stopCh)
}

// WriteToRedis serialises a fragment and stores it in Redis under the cache key.
// Call this after promoting a draft to active so all nodes see the new version.
func (c *PromptCache) WriteToRedis(ctx context.Context, f *Fragment) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("prompt cache: marshal: %w", err)
	}
	key := c.redisKey(f.ID)
	if err := c.redis.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("prompt cache: redis set %q: %w", key, err)
	}
	return nil
}

func (c *PromptCache) redisKey(fragmentID string) string {
	return fmt.Sprintf("%s:prompt:%s", c.tenantID, fragmentID)
}

func (c *PromptCache) fetchFromRedis(ctx context.Context, id string) (*Fragment, error) {
	data, err := c.redis.Get(ctx, c.redisKey(id)).Bytes()
	if err == redis.Nil {
		// Not in Redis — fall through to backend.
		return c.backend.Get(ctx, id)
	}
	if err != nil {
		return nil, fmt.Errorf("prompt cache: redis get %q: %w", id, err)
	}
	var f Fragment
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("prompt cache: decode: %w", err)
	}
	return &f, nil
}

// doRefresh fetches all active fragments from Redis and replaces the snapshot.
func (c *PromptCache) doRefresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Scan all tenant fragment keys from Redis.
	pattern := fmt.Sprintf("%s:prompt:*", c.tenantID)
	keys, err := c.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return
	}

	newSnap := &templateSnapshot{fragments: make(map[string]*Fragment, len(keys))}
	for _, key := range keys {
		data, err := c.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var f Fragment
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		newSnap.fragments[f.ID] = &f
	}
	c.snapshot.Store(newSnap)
}

func (c *PromptCache) poll() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.doRefresh()
		case <-c.refreshCh:
			c.doRefresh()
		}
	}
}
