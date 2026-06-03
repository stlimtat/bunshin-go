package llm

import (
	"context"
	"errors"
	"fmt"
)

// FallbackProvider tries providers in order, returning the first success.
// If a ProviderRegistry is set, skips providers marked unavailable.
// Returns a combined error if all providers fail.
type FallbackProvider struct {
	id        ProviderID
	providers []LLMProvider
	registry  *ProviderRegistry // optional; nil = try all
}

// NewFallbackProvider constructs a FallbackProvider.
// id is the virtual provider ID (e.g. "fallback").
// providers is the priority-ordered list; first is primary.
// Returns an error if providers is empty.
func NewFallbackProvider(id ProviderID, providers ...LLMProvider) (*FallbackProvider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("llm: FallbackProvider requires at least one provider")
	}
	return &FallbackProvider{
		id:        id,
		providers: providers,
	}, nil
}

// ID returns the virtual provider ID.
func (f *FallbackProvider) ID() ProviderID {
	return f.id
}

// WithRegistry links a ProviderRegistry so Complete/StreamComplete skip
// providers the registry has marked unavailable.
func (f *FallbackProvider) WithRegistry(r *ProviderRegistry) *FallbackProvider {
	f.registry = r
	return f
}

// Complete tries each available provider in order.
// On each failure, logs at debug level and continues.
// Returns the first successful response, or a wrapped error listing all failures.
func (f *FallbackProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	candidates := f.candidates()
	errs := make([]error, 0, len(candidates))
	for _, p := range candidates {
		resp, err := p.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, fmt.Errorf("provider %s: %w", p.ID(), err))
	}
	return nil, fmt.Errorf("llm: all providers failed: %w", errors.Join(errs...))
}

// StreamComplete tries each provider like Complete, but for streaming.
// Returns the first provider's stream that opens without error.
// If a provider returns an error from StreamComplete, tries the next.
func (f *FallbackProvider) StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error) {
	candidates := f.candidates()
	errs := make([]error, 0, len(candidates))
	for _, p := range candidates {
		ch, err := p.StreamComplete(ctx, req)
		if err == nil {
			return ch, nil
		}
		errs = append(errs, fmt.Errorf("provider %s: %w", p.ID(), err))
	}
	return nil, fmt.Errorf("llm: all providers failed (stream): %w", errors.Join(errs...))
}

// CanTransferContext returns true only if all providers can transfer context from from.
func (f *FallbackProvider) CanTransferContext(from LLMProvider) bool {
	for _, p := range f.providers {
		if !p.CanTransferContext(from) {
			return false
		}
	}
	return true
}

// NativeMessages delegates to the first provider in the list.
func (f *FallbackProvider) NativeMessages(msgs []Message) (any, error) {
	return f.providers[0].NativeMessages(msgs)
}

// candidates returns the subset of providers to try.
// If registry is set, only returns providers the registry marks available.
// Falls back to all providers if the registry marks none available.
func (f *FallbackProvider) candidates() []LLMProvider {
	if f.registry == nil {
		return f.providers
	}
	available := make([]LLMProvider, 0, len(f.providers))
	for _, p := range f.providers {
		if f.registry.IsAvailable(p.ID()) {
			available = append(available, p)
		}
	}
	if len(available) == 0 {
		// Last-resort: try all providers when registry marks none available.
		return f.providers
	}
	return available
}
