package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
)

// EmbedStore is a read-only PromptBackend backed by an embed.FS (or any fs.FS).
// Fragments are stored as JSON files: "{root}/{slug}.json".
// Fragment.ID is computed as UUIDv5(bunshinNS, tenantID+":"+slug) at load time —
// stable across binary rebuilds as long as the slug doesn't change.
// Watch always returns an immediately-closed channel (no live updates).
type EmbedStore struct {
	mem *MemoryBackend
	// tenantID used when loading (EmbedStore is single-tenant by design).
	tenantID string
}

// NewEmbedStore loads all fragment JSON files from fsys under root.
// Files must be named "{slug}.json" and contain a serialised Fragment.
// tenantID scopes the loaded fragments.
func NewEmbedStore(fsys fs.FS, root, tenantID string) (*EmbedStore, error) {
	mem := NewMemoryBackend()

	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("embed store: read dir %q: %w", root, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < 5 || name[len(name)-5:] != ".json" {
			continue
		}
		data, err := fs.ReadFile(fsys, root+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("embed store: read %q: %w", name, err)
		}
		var f Fragment
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("embed store: parse %q: %w", name, err)
		}
		// Derive slug from filename when not set in JSON.
		if f.Slug == "" {
			f.Slug = name[:len(name)-5]
		}
		// Assign deterministic UUIDv5 from tenantID+slug.
		f.ID = slugUUID(tenantID, f.Slug)
		if err := mem.Put(context.Background(), tenantID, &f); err != nil {
			return nil, fmt.Errorf("embed store: put %q: %w", f.Slug, err)
		}
	}
	return &EmbedStore{mem: mem, tenantID: tenantID}, nil
}

func (s *EmbedStore) Put(_ context.Context, _ string, _ *Fragment) error {
	return ErrNotSupported
}

func (s *EmbedStore) Promote(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

func (s *EmbedStore) Get(ctx context.Context, tenantID, slug string) (*Fragment, error) {
	return s.mem.Get(ctx, tenantID, slug)
}

func (s *EmbedStore) GetByID(ctx context.Context, tenantID, id string) (*Fragment, error) {
	return s.mem.GetByID(ctx, tenantID, id)
}

func (s *EmbedStore) GetVersion(ctx context.Context, tenantID, slug, version string) (*Fragment, error) {
	return s.mem.GetVersion(ctx, tenantID, slug, version)
}

func (s *EmbedStore) List(ctx context.Context, tenantID string, tags ...string) ([]*Fragment, error) {
	return s.mem.List(ctx, tenantID, tags...)
}

func (s *EmbedStore) Delete(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

func (s *EmbedStore) Rename(_ context.Context, _, _, _ string) error {
	return ErrNotSupported
}

// Watch returns a closed channel — embedded fragments never change at runtime.
func (s *EmbedStore) Watch(_ context.Context, _, _ string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment)
	close(ch)
	return ch, nil
}
