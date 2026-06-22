package prompt

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

var (
	_ PromptBackend = (*MemoryBackend)(nil)
	_ PromptBackend = (*EmbedStore)(nil)
	_ PromptBackend = (*FSStore)(nil)
	_ PromptBackend = (*PostgresStore)(nil)
	_ PromptBackend = (*PromptCache)(nil)

	_ PromptActivator = (*EmbedStore)(nil)
	_ PromptActivator = (*FSStore)(nil)
	_ PromptActivator = (*PostgresStore)(nil)
)

// MemoryBackend stores fragments in-process. Used in tests and for
// programmatically registered fragments.
type MemoryBackend struct {
	mu sync.RWMutex
	// (tenantID, slug) → active fragment
	bySlug map[string]map[string]*Fragment
	// uuid → fragment (cross-tenant UUID lookup)
	byUUID map[string]*Fragment
	// (tenantID, slug, version) → fragment
	byVersion map[string]map[string]map[string]*Fragment
	// (tenantID, slug) → watcher channels
	watchers map[string]map[string][]chan *Fragment
}

// NewMemoryBackend constructs an empty MemoryBackend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		bySlug:    make(map[string]map[string]*Fragment),
		byUUID:    make(map[string]*Fragment),
		byVersion: make(map[string]map[string]map[string]*Fragment),
		watchers:  make(map[string]map[string][]chan *Fragment),
	}
}

// Put registers or updates a fragment. Generates a UUIDv4 if f.ID is empty.
func (b *MemoryBackend) Put(_ context.Context, tenantID string, f *Fragment) error {
	if f.Slug == "" {
		return fmt.Errorf("memory backend: Put: fragment Slug must not be empty")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if f.ID == "" {
		f.ID = uuid.New().String()
	}

	if b.bySlug[tenantID] == nil {
		b.bySlug[tenantID] = make(map[string]*Fragment)
	}
	b.bySlug[tenantID][f.Slug] = f
	b.byUUID[f.ID] = f

	if f.Version != "" {
		if b.byVersion[tenantID] == nil {
			b.byVersion[tenantID] = make(map[string]map[string]*Fragment)
		}
		if b.byVersion[tenantID][f.Slug] == nil {
			b.byVersion[tenantID][f.Slug] = make(map[string]*Fragment)
		}
		b.byVersion[tenantID][f.Slug][f.Version] = f
	}

	// Notify watchers.
	if tm, ok := b.watchers[tenantID]; ok {
		for _, ch := range tm[f.Slug] {
			select {
			case ch <- f:
			default:
			}
		}
	}
	return nil
}

func (b *MemoryBackend) Get(_ context.Context, tenantID, slug string) (*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if tm, ok := b.bySlug[tenantID]; ok {
		if f, ok := tm[slug]; ok {
			return f, nil
		}
	}
	return nil, fmt.Errorf("fragment slug=%q tenant=%q: not found", slug, tenantID)
}

func (b *MemoryBackend) GetByID(_ context.Context, _, id string) (*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if f, ok := b.byUUID[id]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("fragment id=%q: not found", id)
}

func (b *MemoryBackend) GetVersion(_ context.Context, tenantID, slug, version string) (*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	tm, ok := b.byVersion[tenantID]
	if !ok {
		return nil, fmt.Errorf("fragment slug=%q tenant=%q: not found", slug, tenantID)
	}
	sv, ok := tm[slug]
	if !ok {
		return nil, fmt.Errorf("fragment slug=%q tenant=%q: not found", slug, tenantID)
	}
	f, ok := sv[version]
	if !ok {
		return nil, fmt.Errorf("fragment slug=%q tenant=%q version=%q: not found", slug, tenantID, version)
	}
	return f, nil
}

func (b *MemoryBackend) List(_ context.Context, tenantID string, tags ...string) ([]*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []*Fragment
	for _, f := range b.bySlug[tenantID] {
		if len(tags) == 0 || hasAllTags(f, tags) {
			out = append(out, f)
		}
	}
	return out, nil
}

func (b *MemoryBackend) Rename(_ context.Context, tenantID, id, newSlug string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, ok := b.byUUID[id]
	if !ok {
		return fmt.Errorf("fragment id=%q: not found", id)
	}
	tm := b.bySlug[tenantID]
	if tm == nil {
		return fmt.Errorf("fragment id=%q tenant=%q: not found", id, tenantID)
	}
	oldSlug := f.Slug
	delete(tm, oldSlug)
	f.Slug = newSlug
	tm[newSlug] = f

	// Move version index.
	if bv, ok := b.byVersion[tenantID]; ok {
		if versions, ok := bv[oldSlug]; ok {
			bv[newSlug] = versions
			delete(bv, oldSlug)
		}
	}
	return nil
}

// Watch returns a channel that receives fragment updates for (tenantID, slug).
func (b *MemoryBackend) Watch(ctx context.Context, tenantID, slug string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment, 4)
	b.mu.Lock()
	if b.watchers[tenantID] == nil {
		b.watchers[tenantID] = make(map[string][]chan *Fragment)
	}
	b.watchers[tenantID][slug] = append(b.watchers[tenantID][slug], ch)
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()
		if tm, ok := b.watchers[tenantID]; ok {
			ws := tm[slug]
			for i, w := range ws {
				if w == ch {
					tm[slug] = append(ws[:i], ws[i+1:]...)
					break
				}
			}
		}
		close(ch)
	}()
	return ch, nil
}

// hasAllTags returns true if fragment f has every tag in required.
func hasAllTags(f *Fragment, required []string) bool {
	tagSet := make(map[string]bool, len(f.Tags))
	for _, t := range f.Tags {
		tagSet[t] = true
	}
	for _, r := range required {
		if !tagSet[r] {
			return false
		}
	}
	return true
}
