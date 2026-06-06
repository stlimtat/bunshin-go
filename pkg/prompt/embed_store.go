package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
)

// EmbedStore is a read-only PromptBackend backed by an embed.FS (or any fs.FS).
// Fragments are stored as JSON files: "{root}/{id}.json".
// All fragments are loaded once at construction time — Watch always returns an
// empty, immediately-closed channel (no live updates in binary-embedded assets).
//
// Use this backend to ship prompts inside the binary for zero-dependency deployments.
type EmbedStore struct {
	mem *MemoryBackend
}

// NewEmbedStore loads all fragment JSON files from fsys under root into memory.
// Fragment files must be named "{id}.json" and contain a serialised Fragment.
func NewEmbedStore(fsys fs.FS, root string) (*EmbedStore, error) {
	mem := NewMemoryBackend()

	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("embed store: read dir %q: %w", root, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if len(e.Name()) < 5 || e.Name()[len(e.Name())-5:] != ".json" {
			continue
		}
		data, err := fs.ReadFile(fsys, root+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("embed store: read %q: %w", e.Name(), err)
		}
		var f Fragment
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("embed store: parse %q: %w", e.Name(), err)
		}
		mem.Put(&f)
	}
	return &EmbedStore{mem: mem}, nil
}

func (s *EmbedStore) Get(ctx context.Context, id string) (*Fragment, error) {
	return s.mem.Get(ctx, id)
}

func (s *EmbedStore) List(ctx context.Context, tags ...string) ([]*Fragment, error) {
	return s.mem.List(ctx, tags...)
}

func (s *EmbedStore) GetVersion(ctx context.Context, id, version string) (*Fragment, error) {
	return s.mem.GetVersion(ctx, id, version)
}

// Watch returns a closed channel — EmbedStore fragments never change at runtime.
func (s *EmbedStore) Watch(ctx context.Context, id string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment)
	close(ch)
	return ch, nil
}
