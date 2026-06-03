package prompt

import (
	"context"
	"fmt"
	"sync"
)

// MemoryBackend stores fragments in-process. Used in tests and for
// programmatically registered fragments.
type MemoryBackend struct {
	mu        sync.RWMutex
	fragments map[string]*Fragment // id → latest version
	versions  map[string]map[string]*Fragment // id → version → fragment
	watchers  map[string][]chan *Fragment
}

// NewMemoryBackend constructs an empty MemoryBackend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		fragments: make(map[string]*Fragment),
		versions:  make(map[string]map[string]*Fragment),
		watchers:  make(map[string][]chan *Fragment),
	}
}

// Put registers or updates a fragment. If the fragment has a Version set,
// it is stored under that version key as well.
func (b *MemoryBackend) Put(f *Fragment) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fragments[f.ID] = f
	if f.Version != "" {
		if b.versions[f.ID] == nil {
			b.versions[f.ID] = make(map[string]*Fragment)
		}
		b.versions[f.ID][f.Version] = f
	}
	// Notify watchers.
	for _, ch := range b.watchers[f.ID] {
		select {
		case ch <- f:
		default: // drop if watcher is not consuming
		}
	}
}

func (b *MemoryBackend) Get(_ context.Context, id string) (*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	f, ok := b.fragments[id]
	if !ok {
		return nil, fmt.Errorf("fragment %q not found", id)
	}
	return f, nil
}

func (b *MemoryBackend) List(_ context.Context, tags ...string) ([]*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []*Fragment
	for _, f := range b.fragments {
		if len(tags) == 0 {
			out = append(out, f)
			continue
		}
		if hasAllTags(f, tags) {
			out = append(out, f)
		}
	}
	return out, nil
}

func (b *MemoryBackend) GetVersion(_ context.Context, id, version string) (*Fragment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.versions[id]
	if !ok {
		return nil, fmt.Errorf("fragment %q not found", id)
	}
	f, ok := v[version]
	if !ok {
		return nil, fmt.Errorf("fragment %q version %q not found", id, version)
	}
	return f, nil
}

// Watch returns a channel that receives fragment updates.
// The channel is buffered (capacity 4); updates are dropped when the buffer is full.
// Consumers must drain promptly or risk missing updates.
func (b *MemoryBackend) Watch(ctx context.Context, id string) (<-chan *Fragment, error) {
	b.mu.Lock()
	ch := make(chan *Fragment, 4)
	b.watchers[id] = append(b.watchers[id], ch)
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()
		ws := b.watchers[id]
		for i, w := range ws {
			if w == ch {
				b.watchers[id] = append(ws[:i], ws[i+1:]...)
				break
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
