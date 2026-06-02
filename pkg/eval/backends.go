package eval

import (
	"context"
	"fmt"
	"sync"
)

// MemoryDatasetBackend stores datasets in-process.
type MemoryDatasetBackend struct {
	mu      sync.RWMutex
	sets    map[string]*Dataset
	reports []*EvalReport
}

// NewMemoryDatasetBackend constructs an empty MemoryDatasetBackend.
func NewMemoryDatasetBackend() *MemoryDatasetBackend {
	return &MemoryDatasetBackend{sets: make(map[string]*Dataset)}
}

func (b *MemoryDatasetBackend) Push(_ context.Context, ds *Dataset) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sets[ds.Name] = ds
	return nil
}

func (b *MemoryDatasetBackend) Pull(_ context.Context, name string) (*Dataset, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ds, ok := b.sets[name]
	if !ok {
		return nil, fmt.Errorf("dataset %q not found", name)
	}
	return ds, nil
}

func (b *MemoryDatasetBackend) PushResults(_ context.Context, r *EvalReport) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reports = append(b.reports, r)
	return nil
}

func (b *MemoryDatasetBackend) ListDatasets(_ context.Context) ([]*Dataset, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]*Dataset, 0, len(b.sets))
	for _, ds := range b.sets {
		out = append(out, ds)
	}
	return out, nil
}
