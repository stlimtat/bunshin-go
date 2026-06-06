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

// FSStore is a PromptBackend backed by JSON files on the local file system.
// Files are named "{root}/{id}.json". A background goroutine uses fsnotify to
// watch for changes and push updates to active Watch subscribers.
//
// Call Close to stop the watcher goroutine.
type FSStore struct {
	mu       sync.RWMutex
	root     string
	mem      *MemoryBackend
	watcher  *fsnotify.Watcher
	watchers map[string][]chan *Fragment
}

// NewFSStore creates an FSStore rooted at root. All existing fragment files are
// loaded immediately. An fsnotify watcher is started in the background.
func NewFSStore(root string) (*FSStore, error) {
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
		mem:      NewMemoryBackend(),
		watcher:  w,
		watchers: make(map[string][]chan *Fragment),
	}

	// Load all existing fragments.
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
			s.mem.Put(f)
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
				s.mem.Put(f)
				s.mu.RLock()
				for _, ch := range s.watchers[f.ID] {
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

func (s *FSStore) Get(ctx context.Context, id string) (*Fragment, error) {
	return s.mem.Get(ctx, id)
}

func (s *FSStore) List(ctx context.Context, tags ...string) ([]*Fragment, error) {
	return s.mem.List(ctx, tags...)
}

func (s *FSStore) GetVersion(ctx context.Context, id, version string) (*Fragment, error) {
	return s.mem.GetVersion(ctx, id, version)
}

func (s *FSStore) Watch(ctx context.Context, id string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment, 4)
	s.mu.Lock()
	s.watchers[id] = append(s.watchers[id], ch)
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		ws := s.watchers[id]
		for i, w := range ws {
			if w == ch {
				s.watchers[id] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		close(ch)
	}()
	return ch, nil
}
