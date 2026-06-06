package prompt_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/prompt"
)

// ---- EmbedStore ----

func jsonFragment(f *prompt.Fragment) []byte {
	data, _ := json.Marshal(f)
	return data
}

func TestEmbedStore_GetAndList(t *testing.T) {
	fsys := fstest.MapFS{
		"prompts/greet.json": {
			Data: jsonFragment(&prompt.Fragment{
				ID:      "greet",
				Content: "Hello {{.name}}",
				Tags:    []string{"system"},
			}),
		},
		"prompts/bye.json": {
			Data: jsonFragment(&prompt.Fragment{
				ID:      "bye",
				Content: "Goodbye!",
				Tags:    []string{"user"},
			}),
		},
		"prompts/ignore.txt": {Data: []byte("not a fragment")},
	}

	store, err := prompt.NewEmbedStore(fsys, "prompts")
	if err != nil {
		t.Fatalf("NewEmbedStore: %v", err)
	}

	f, err := store.Get(context.Background(), "greet")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.Content != "Hello {{.name}}" {
		t.Fatalf("wrong content: %q", f.Content)
	}

	all, _ := store.List(context.Background())
	if len(all) != 2 {
		t.Fatalf("want 2 fragments, got %d", len(all))
	}

	sys, _ := store.List(context.Background(), "system")
	if len(sys) != 1 || sys[0].ID != "greet" {
		t.Fatalf("tag filter failed: %v", sys)
	}
}

func TestEmbedStore_WatchIsClosed(t *testing.T) {
	fsys := fstest.MapFS{}
	store, err := prompt.NewEmbedStore(fsys, ".")
	if err != nil {
		t.Fatalf("NewEmbedStore: %v", err)
	}
	ch, err := store.Watch(context.Background(), "any")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed, not send a value")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed")
	}
}

// ---- FSStore ----

func writeFragmentFile(t *testing.T, dir string, f *prompt.Fragment) {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, f.ID+".json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestFSStore_LoadsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "a", Content: "alpha", Tags: []string{"x"}})
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "b", Content: "beta", Tags: []string{"y"}})

	store, err := prompt.NewFSStore(dir)
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	defer store.Close()

	f, err := store.Get(context.Background(), "a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.Content != "alpha" {
		t.Fatalf("wrong content: %q", f.Content)
	}

	all, _ := store.List(context.Background())
	if len(all) != 2 {
		t.Fatalf("want 2 fragments, got %d", len(all))
	}
}

func TestFSStore_HotReload(t *testing.T) {
	dir := t.TempDir()
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "live", Content: "v1"})

	store, err := prompt.NewFSStore(dir)
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := store.Watch(ctx, "live")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Overwrite the file to trigger a fsnotify event.
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "live", Content: "v2"})

	select {
	case updated := <-ch:
		if updated.Content != "v2" {
			t.Fatalf("want v2, got %q", updated.Content)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for hot reload")
	}
}
