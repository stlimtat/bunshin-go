package llm

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/stlimtat/bunshin-go/internal/telemetry"
)

const defaultPingInterval = 30 * time.Second
const pingTimeout = 5 * time.Second

// providerEntry holds a provider with its associated tags.
type providerEntry struct {
	provider LLMProvider
	tags     Tags
}

// ProviderRegistry tracks LLMProvider availability via periodic pings and
// supports tag-based selection. Register named instances with tags; Select
// resolves matching available providers at call time.
//
// Use WithRegistry on FallbackProvider to integrate with health endpoints.
// Start must be called at most once; duplicate calls are no-ops.
// Available() returns providers sorted by ascending ping latency.
type ProviderRegistry struct {
	mu           sync.RWMutex
	startOnce    sync.Once
	providers    []LLMProvider
	entries      map[ProviderID]providerEntry // tag-registered entries
	available    map[ProviderID]bool
	latency      map[ProviderID]time.Duration // last successful ping RTT; math.MaxInt64 if no successful ping
	pingInterval time.Duration
	logger       zerolog.Logger
}

// NewProviderRegistry constructs a registry with the given providers.
// Use Start() to begin background health monitoring.
// Default pingInterval is 30s; logger defaults to zerolog.Nop().
// For tag-based selection use Register after construction.
func NewProviderRegistry(providers ...LLMProvider) *ProviderRegistry {
	lat := make(map[ProviderID]time.Duration, len(providers))
	for _, p := range providers {
		lat[p.ID()] = time.Duration(math.MaxInt64)
	}
	return &ProviderRegistry{
		providers:    providers,
		entries:      make(map[ProviderID]providerEntry),
		available:    make(map[ProviderID]bool),
		latency:      lat,
		pingInterval: defaultPingInterval,
		logger:       zerolog.Nop(),
	}
}

// Register adds a named provider instance with tags for tag-based selection.
// The provider is also added to the health-monitoring pool.
// Tags encode both selection criteria (vendor, tier, budget) and resource
// configuration metadata. This is the key differentiator of bunshin-go:
// multiple instances of the same vendor coexist with different keys or budgets.
func (r *ProviderRegistry) Register(id ProviderID, p LLMProvider, tags Tags) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[id] = providerEntry{provider: p, tags: tags}
	// Add to health-monitoring pool if not already present.
	for _, existing := range r.providers {
		if existing.ID() == p.ID() {
			return
		}
	}
	r.providers = append(r.providers, p)
	r.latency[p.ID()] = time.Duration(math.MaxInt64)
}

// Select returns available providers whose tags contain all key-value pairs in
// each of the provided tag maps. Tags from multiple Tag() calls are ANDed.
// If no providers match, returns nil.
//
// Example:
//
//	registry.Select(llm.Tag("vendor", "openai"), llm.Tag("budget", "high"))
func (r *ProviderRegistry) Select(filters ...Tags) []LLMProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []LLMProvider
	for _, entry := range r.entries {
		if !r.available[entry.provider.ID()] {
			continue
		}
		if tagsMatch(entry.tags, filters) {
			result = append(result, entry.provider)
		}
	}
	// Sort by latency for deterministic ordering.
	sort.Slice(result, func(i, j int) bool {
		return r.latency[result[i].ID()] < r.latency[result[j].ID()]
	})
	return result
}

// Get returns the provider registered under the given instance ID, if any.
func (r *ProviderRegistry) Get(id ProviderID) (LLMProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, false
	}
	return e.provider, true
}

// tagsMatch reports whether entry tags contain all key-value pairs in all filters.
func tagsMatch(entryTags Tags, filters []Tags) bool {
	for _, filter := range filters {
		for k, v := range filter {
			if entryTags[k] != v {
				return false
			}
		}
	}
	return true
}

// WithLogger sets the logger used for ping result debug output.
func (r *ProviderRegistry) WithLogger(logger zerolog.Logger) *ProviderRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = logger
	return r
}

// WithPingInterval overrides the default 30-second health-check interval.
func (r *ProviderRegistry) WithPingInterval(d time.Duration) *ProviderRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pingInterval = d
	return r
}

// Start begins background health monitoring. It immediately pings all providers
// to populate initial availability, then pings on each interval tick.
// The goroutine exits when ctx is cancelled. Duplicate calls are no-ops.
func (r *ProviderRegistry) Start(ctx context.Context) {
	r.startOnce.Do(func() {
		r.pingAll(ctx)
		go func() {
			r.mu.RLock()
			interval := r.pingInterval
			r.mu.RUnlock()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					r.pingAll(ctx)
				}
			}
		}()
	})
}

// IsAvailable reports whether the provider with the given ID passed its last ping.
func (r *ProviderRegistry) IsAvailable(id ProviderID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.available[id]
}

// Available returns providers that passed their last ping, sorted by ascending
// ping latency so callers naturally prefer the fastest live provider.
// If no providers are available, returns all providers as a last resort.
func (r *ProviderRegistry) Available() []LLMProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]LLMProvider, 0, len(r.providers))
	for _, p := range r.providers {
		if r.available[p.ID()] {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		// Last-resort: return all providers when none passed their ping.
		return r.providers
	}
	sort.Slice(result, func(i, j int) bool {
		return r.latency[result[i].ID()] < r.latency[result[j].ID()]
	})
	return result
}

// Status returns a snapshot of provider availability keyed by ProviderID.
func (r *ProviderRegistry) Status() map[ProviderID]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot := make(map[ProviderID]bool, len(r.available))
	for k, v := range r.available {
		snapshot[k] = v
	}
	return snapshot
}

// pingAll pings every provider that implements Pinger.
// Providers that don't implement Pinger are assumed available.
func (r *ProviderRegistry) pingAll(ctx context.Context) {
	for _, p := range r.providers {
		pinger, ok := p.(telemetry.Pinger)
		if !ok {
			// Providers with no Ping method are always available.
			r.mu.Lock()
			r.available[p.ID()] = true
			r.mu.Unlock()
			r.logger.Debug().Str("provider", string(p.ID())).Msg("provider has no Ping method; assumed available")
			continue
		}
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		start := time.Now()
		err := pinger.Ping(pingCtx)
		rtt := time.Since(start)
		cancel()
		r.mu.Lock()
		r.available[p.ID()] = err == nil
		if err == nil {
			r.latency[p.ID()] = rtt
		}
		r.mu.Unlock()
		if err != nil {
			r.logger.Debug().Str("provider", string(p.ID())).Err(err).Msg("ping failed; provider marked unavailable")
		} else {
			r.logger.Debug().Str("provider", string(p.ID())).Dur("rtt", rtt).Msg("ping succeeded; provider marked available")
		}
	}
}
