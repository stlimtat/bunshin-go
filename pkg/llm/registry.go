package llm

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const defaultPingInterval = 30 * time.Second
const pingTimeout = 5 * time.Second

// ProviderRegistry tracks LLMProvider availability via periodic pings.
// Use WithRegistry on FallbackProvider to integrate with health endpoints.
type ProviderRegistry struct {
	mu           sync.RWMutex
	providers    []LLMProvider
	available    map[ProviderID]bool
	pingInterval time.Duration
	logger       zerolog.Logger
}

// NewProviderRegistry constructs a registry with the given providers.
// Use Start() to begin background health monitoring.
// Default pingInterval is 30s; logger defaults to zerolog.Nop().
func NewProviderRegistry(providers ...LLMProvider) *ProviderRegistry {
	return &ProviderRegistry{
		providers:    providers,
		available:    make(map[ProviderID]bool),
		pingInterval: defaultPingInterval,
		logger:       zerolog.Nop(),
	}
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
// The goroutine exits when ctx is cancelled.
func (r *ProviderRegistry) Start(ctx context.Context) {
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
}

// IsAvailable reports whether the provider with the given ID passed its last ping.
func (r *ProviderRegistry) IsAvailable(id ProviderID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.available[id]
}

// Available returns providers that passed their last ping.
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
		pinger, ok := p.(Pinger)
		if !ok {
			// Non-Pinger providers are always available.
			r.mu.Lock()
			r.available[p.ID()] = true
			r.mu.Unlock()
			r.logger.Debug().Str("provider", string(p.ID())).Msg("provider does not implement Pinger; assumed available")
			continue
		}
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		err := pinger.Ping(pingCtx)
		cancel()
		r.mu.Lock()
		r.available[p.ID()] = err == nil
		r.mu.Unlock()
		if err != nil {
			r.logger.Debug().Str("provider", string(p.ID())).Err(err).Msg("ping failed; provider marked unavailable")
		} else {
			r.logger.Debug().Str("provider", string(p.ID())).Msg("ping succeeded; provider marked available")
		}
	}
}
