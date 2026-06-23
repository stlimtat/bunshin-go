package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FSStore is a read-only PromptBackend backed by JSON files on the local filesystem.
// Files are named "{root}/{slug}.json". A background goroutine uses fsnotify to
// watch for changes and push updates to Watch subscribers.
// Fragment.ID is UUIDv5(bunshinNS, tenantID+":"+slug) — stable across restarts.
//
// Call Close to stop the watcher goroutine.
type FSStore struct {
	mu       sync.RWMutex
	root     string
	tenantID string
	mem      *MemoryBackend
	watcher  *fsnotify.Watcher
	// (tenantID, slug) → watcher channels (separate from MemoryBackend to support
	// hot-reload notifications from fsnotify)
	watchers map[string][]chan *Fragment
}

// NewFSStore creates an FSStore rooted at root scoped to tenantID.
// All existing fragment files are loaded immediately.
func NewFSStore(root, tenantID string) (*FSStore, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fs store: watcher: %w", err)
	}
	if err := w.Add(root); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("fs store: watch %q: %w", root, err)
	}

	s := &FSStore{
		root:     root,
		tenantID: tenantID,
		mem:      NewMemoryBackend(),
		watcher:  w,
		watchers: make(map[string][]chan *Fragment),
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("fs store: read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, e.Name())
		if f, err := s.loadFile(path); err == nil {
			_ = s.mem.Put(context.Background(), tenantID, f)
		}
	}

	go s.watchLoop()
	return s, nil
}

func (s *FSStore) loadFile(path string) (*Fragment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Fragment
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("fs store: parse %q: %w", path, err)
	}
	// Derive slug from filename when not set in JSON.
	if f.Slug == "" {
		base := filepath.Base(path)
		f.Slug = strings.TrimSuffix(base, ".json")
	}
	// Deterministic UUIDv5.
	f.ID = slugUUID(s.tenantID, f.Slug)
	return &f, nil
}

func (s *FSStore) watchLoop() {
	for {
		select {
		case ev, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(ev.Name, ".json") {
				continue
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) {
				f, err := s.loadFile(ev.Name)
				if err != nil {
					continue
				}
				_ = s.mem.Put(context.Background(), s.tenantID, f)
				s.mu.RLock()
				for _, ch := range s.watchers[f.Slug] {
					select {
					case ch <- f:
					default:
					}
				}
				s.mu.RUnlock()
			}
		case _, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// Close stops the fsnotify watcher goroutine.
func (s *FSStore) Close() error {
	return s.watcher.Close()
}

func (s *FSStore) Put(_ context.Context, _ string, _ *Fragment) error {
	return ErrNotSupported
}

func (s *FSStore) Promote(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

func (s *FSStore) Get(ctx context.Context, tenantID, slug string) (*Fragment, error) {
	return s.mem.Get(ctx, tenantID, slug)
}

func (s *FSStore) GetByID(ctx context.Context, tenantID, id string) (*Fragment, error) {
	return s.mem.GetByID(ctx, tenantID, id)
}

func (s *FSStore) GetVersion(ctx context.Context, tenantID, slug, version string) (*Fragment, error) {
	return s.mem.GetVersion(ctx, tenantID, slug, version)
}

func (s *FSStore) List(ctx context.Context, tenantID string, tags ...string) ([]*Fragment, error) {
	return s.mem.List(ctx, tenantID, tags...)
}

func (s *FSStore) Delete(_ context.Context, _, _ string) error {
	return ErrNotSupported
}

func (s *FSStore) Rename(_ context.Context, _, _, _ string) error {
	return ErrNotSupported
}

func (s *FSStore) Watch(ctx context.Context, _, slug string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment, 4)
	s.mu.Lock()
	s.watchers[slug] = append(s.watchers[slug], ch)
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		ws := s.watchers[slug]
		for i, w := range ws {
			if w == ch {
				s.watchers[slug] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		close(ch)
	}()
	return ch, nil
}
