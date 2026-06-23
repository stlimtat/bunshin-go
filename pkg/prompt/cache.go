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

// templateSnapshot is the immutable, compiled in-process view of active fragments
// for all tenants. Swapped atomically.
type templateSnapshot struct {
	// (tenantID, slug) → fragment
	fragments map[string]map[string]*Fragment
}

// PromptCache is a two-level cache on top of a PromptBackend:
//
//  1. Redis (shared): raw fragment JSON, keyed "{tenantID}:prompt:{slug}".
//     All compute nodes share one Redis — a single write propagates to everyone.
//
//  2. In-process snapshot: an atomic.Pointer to a templateSnapshot.
//     Refreshed on a 5-second poll interval or on demand via Refresh().
type PromptCache struct {
	backend PromptBackend
	redis   *redis.Client

	snapshot  atomic.Pointer[templateSnapshot]
	mu        sync.Mutex // guards concurrent Refresh() calls
	refreshCh chan struct{}
	stopCh    chan struct{}
}

// NewPromptCache constructs a cache and starts the background poll goroutine.
// Call Close to stop it.
func NewPromptCache(backend PromptBackend, rdb *redis.Client) *PromptCache {
	c := &PromptCache{
		backend:   backend,
		redis:     rdb,
		refreshCh: make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}
	c.snapshot.Store(&templateSnapshot{fragments: make(map[string]map[string]*Fragment)})
	go c.poll()
	return c
}

// Get returns the cached active fragment by (tenantID, slug), falling through
// to Redis then the backend.
func (c *PromptCache) Get(ctx context.Context, tenantID, slug string) (*Fragment, error) {
	snap := c.snapshot.Load()
	if tm, ok := snap.fragments[tenantID]; ok {
		if f, ok := tm[slug]; ok {
			return f, nil
		}
	}
	return c.fetchFromRedis(ctx, tenantID, slug)
}

// GetByID bypasses cache and reads from the backend.
func (c *PromptCache) GetByID(ctx context.Context, tenantID, id string) (*Fragment, error) {
	return c.backend.GetByID(ctx, tenantID, id)
}

// GetVersion bypasses cache and reads from the backend.
func (c *PromptCache) GetVersion(ctx context.Context, tenantID, slug, version string) (*Fragment, error) {
	return c.backend.GetVersion(ctx, tenantID, slug, version)
}

// List returns cached active fragments for tenantID with optional tag filter.
func (c *PromptCache) List(_ context.Context, tenantID string, tags ...string) ([]*Fragment, error) {
	snap := c.snapshot.Load()
	var out []*Fragment
	for _, f := range snap.fragments[tenantID] {
		if len(tags) == 0 || hasAllTags(f, tags) {
			out = append(out, f)
		}
	}
	return out, nil
}

// Put delegates to the underlying backend and writes to Redis.
func (c *PromptCache) Put(ctx context.Context, tenantID string, f *Fragment) error {
	if err := c.backend.Put(ctx, tenantID, f); err != nil {
		return err
	}
	return c.writeToRedis(ctx, tenantID, f)
}

// Delete delegates to the underlying backend then triggers a cache refresh.
func (c *PromptCache) Delete(ctx context.Context, tenantID, slug string) error {
	if err := c.backend.Delete(ctx, tenantID, slug); err != nil {
		return err
	}
	c.Refresh()
	return nil
}

// Rename delegates to the underlying backend.
func (c *PromptCache) Rename(ctx context.Context, tenantID, id, newSlug string) error {
	return c.backend.Rename(ctx, tenantID, id, newSlug)
}

// Watch delegates to the underlying backend.
func (c *PromptCache) Watch(ctx context.Context, tenantID, slug string) (<-chan *Fragment, error) {
	return c.backend.Watch(ctx, tenantID, slug)
}

// Refresh triggers an immediate pull from Redis into the in-process snapshot.
// Non-blocking: coalesced if a refresh is already pending.
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

func (c *PromptCache) writeToRedis(ctx context.Context, tenantID string, f *Fragment) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("prompt cache: marshal: %w", err)
	}
	key := redisKey(tenantID, f.Slug)
	if err := c.redis.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("prompt cache: redis set %q: %w", key, err)
	}
	return nil
}

func redisKey(tenantID, slug string) string {
	return fmt.Sprintf("%s:prompt:%s", tenantID, slug)
}

func (c *PromptCache) fetchFromRedis(ctx context.Context, tenantID, slug string) (*Fragment, error) {
	data, err := c.redis.Get(ctx, redisKey(tenantID, slug)).Bytes()
	if err == redis.Nil {
		return c.backend.Get(ctx, tenantID, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("prompt cache: redis get: %w", err)
	}
	var f Fragment
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("prompt cache: decode: %w", err)
	}
	return &f, nil
}

func (c *PromptCache) doRefresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Scan all tenant fragment keys: "{tenantID}:prompt:{slug}"
	keys, err := c.redis.Keys(ctx, "*:prompt:*").Result()
	if err != nil {
		return
	}

	newSnap := &templateSnapshot{fragments: make(map[string]map[string]*Fragment, len(keys))}
	for _, key := range keys {
		data, err := c.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var f Fragment
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		// Extract tenantID from key prefix.
		tenantID := extractTenantID(key)
		if tenantID == "" {
			continue
		}
		if newSnap.fragments[tenantID] == nil {
			newSnap.fragments[tenantID] = make(map[string]*Fragment)
		}
		newSnap.fragments[tenantID][f.Slug] = &f
	}
	c.snapshot.Store(newSnap)
}

// extractTenantID extracts tenantID from a Redis key of the form "{tenantID}:prompt:{slug}".
func extractTenantID(key string) string {
	const marker = ":prompt:"
	idx := len(key)
	for i := 0; i < len(key)-len(marker); i++ {
		if key[i:i+len(marker)] == marker {
			idx = i
			break
		}
	}
	if idx == len(key) {
		return ""
	}
	return key[:idx]
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
