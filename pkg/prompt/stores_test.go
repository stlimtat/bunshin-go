package prompt_test

import (
	"context"
	"encoding/json"
	"errors"
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

	store, err := prompt.NewEmbedStore(fsys, "prompts", testTenant)
	if err != nil {
		t.Fatalf("NewEmbedStore: %v", err)
	}

	f, err := store.Get(context.Background(), testTenant, "greet")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.Content != "Hello {{.name}}" {
		t.Fatalf("wrong content: %q", f.Content)
	}

	all, _ := store.List(context.Background(), testTenant)
	if len(all) != 2 {
		t.Fatalf("want 2 fragments, got %d", len(all))
	}

	sys, _ := store.List(context.Background(), testTenant, "system")
	if len(sys) != 1 || sys[0].Slug != "greet" {
		t.Fatalf("tag filter failed: %v", sys)
	}
}

func TestEmbedStore_WatchIsClosed(t *testing.T) {
	fsys := fstest.MapFS{}
	store, err := prompt.NewEmbedStore(fsys, ".", testTenant)
	if err != nil {
		t.Fatalf("NewEmbedStore: %v", err)
	}
	ch, err := store.Watch(context.Background(), testTenant, "any")
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

	store, err := prompt.NewFSStore(dir, testTenant)
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	defer store.Close()

	f, err := store.Get(context.Background(), testTenant, "a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.Content != "alpha" {
		t.Fatalf("wrong content: %q", f.Content)
	}

	all, _ := store.List(context.Background(), testTenant)
	if len(all) != 2 {
		t.Fatalf("want 2 fragments, got %d", len(all))
	}
}

func TestFSStore_HotReload(t *testing.T) {
	dir := t.TempDir()
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "live", Content: "v1"})

	store, err := prompt.NewFSStore(dir, testTenant)
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := store.Watch(ctx, testTenant, "live")
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

// ---- EmbedStore ErrNotSupported ----

func TestEmbedStore_ReadOnly(t *testing.T) {
	fsys := fstest.MapFS{
		"prompts/greet.json": {
			Data: jsonFragment(&prompt.Fragment{Content: "hello"}),
		},
	}
	store, err := prompt.NewEmbedStore(fsys, "prompts", testTenant)
	if err != nil {
		t.Fatalf("NewEmbedStore: %v", err)
	}

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Put returns ErrNotSupported",
			fn:   func() error { return store.Put(context.Background(), testTenant, &prompt.Fragment{Slug: "x"}) },
		},
		{
			name: "Rename returns ErrNotSupported",
			fn:   func() error { return store.Rename(context.Background(), testTenant, "some-id", "new-slug") },
		},
		{
			name: "Promote returns ErrNotSupported",
			fn:   func() error { return store.Promote(context.Background(), testTenant, "some-id") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); !errors.Is(err, prompt.ErrNotSupported) {
				t.Errorf("want ErrNotSupported, got %v", err)
			}
		})
	}
}

func TestEmbedStore_GetByID(t *testing.T) {
	fsys := fstest.MapFS{
		"prompts/hero.json": {
			Data: jsonFragment(&prompt.Fragment{Content: "hero content"}),
		},
	}
	store, err := prompt.NewEmbedStore(fsys, "prompts", testTenant)
	if err != nil {
		t.Fatalf("NewEmbedStore: %v", err)
	}

	// Resolve the UUID via Get first.
	f, err := store.Get(context.Background(), testTenant, "hero")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := store.GetByID(context.Background(), testTenant, f.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Slug != "hero" {
		t.Errorf("want slug=hero, got %q", got.Slug)
	}
}

// ---- FSStore ErrNotSupported ----

func TestFSStore_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "r", Content: "read-only"})

	store, err := prompt.NewFSStore(dir, testTenant)
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Put returns ErrNotSupported",
			fn:   func() error { return store.Put(context.Background(), testTenant, &prompt.Fragment{Slug: "x"}) },
		},
		{
			name: "Rename returns ErrNotSupported",
			fn:   func() error { return store.Rename(context.Background(), testTenant, "some-id", "new-slug") },
		},
		{
			name: "Promote returns ErrNotSupported",
			fn:   func() error { return store.Promote(context.Background(), testTenant, "some-id") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); !errors.Is(err, prompt.ErrNotSupported) {
				t.Errorf("want ErrNotSupported, got %v", err)
			}
		})
	}
}

func TestFSStore_GetByID(t *testing.T) {
	dir := t.TempDir()
	writeFragmentFile(t, dir, &prompt.Fragment{ID: "byid", Content: "by id content"})

	store, err := prompt.NewFSStore(dir, testTenant)
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	defer store.Close()

	// Resolve the UUID via Get first.
	f, err := store.Get(context.Background(), testTenant, "byid")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := store.GetByID(context.Background(), testTenant, f.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Slug != "byid" {
		t.Errorf("want slug=byid, got %q", got.Slug)
	}
}
