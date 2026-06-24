package git_test

import (
	"context"
	"errors"
	"os"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stlimtat/bunshin-go/pkg/skill"
	"github.com/stlimtat/bunshin-go/pkg/skill/store/git"
)

const testYAML = `
name: test-skill
description: A test skill
body: {slug: instructions}
trigger: model
`

func newRepo(t *testing.T) *gogit.Repository {
	t.Helper()
	dir, err := os.MkdirTemp("", "bunshin-skill-git-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return repo
}

func makeSpec(t *testing.T) *skill.Spec {
	t.Helper()
	s, err := skill.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_CreateAndGet_NoActive(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	ver, err := s.Create(context.Background(), "t1", spec)
	if err != nil || ver == "" {
		t.Fatalf("Create failed: %v", err)
	}
	// No active version yet.
	_, err = s.Get(context.Background(), "t1", "test-skill")
	if !errors.Is(err, skill.ErrNotFound) {
		t.Errorf("want ErrNotFound before Activate, got %v", err)
	}
}

func TestStore_ActivateAndGet(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)
	if err := s.Activate(context.Background(), "t1", "test-skill", ver); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := s.Get(context.Background(), "t1", "test-skill")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != ver {
		t.Errorf("want version %q, got %q", ver, got.Version)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("want status active, got %q", got.Status)
	}
}

func TestStore_Create_Idempotent(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	v1, _ := s.Create(context.Background(), "t1", spec)
	v2, _ := s.Create(context.Background(), "t1", spec)
	if v1 != v2 {
		t.Errorf("idempotent create must return same version: %q vs %q", v1, v2)
	}
}

func TestStore_GetVersion(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)
	got, err := s.GetVersion(context.Background(), "t1", "test-skill", ver)
	if err != nil || got == nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got.Version != ver {
		t.Errorf("version mismatch: want %q, got %q", ver, got.Version)
	}
}

func TestStore_GetVersion_Missing(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	makeSpec(t)
	s.Create(context.Background(), "t1", makeSpec(t)) //nolint:errcheck
	_, err := s.GetVersion(context.Background(), "t1", "test-skill", "sha256:nonexistent")
	if !errors.Is(err, skill.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStore_List(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	for _, yaml := range []string{
		"name: skill-a\nbody: {slug: a}\ntrigger: model\n",
		"name: skill-b\nbody: {slug: b}\ntrigger: condition\n",
	} {
		sp, _ := skill.Parse([]byte(yaml))
		s.Create(context.Background(), "t1", sp) //nolint:errcheck
	}
	names, err := s.List(context.Background(), "t1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("want 2 skills, got %d", len(names))
	}
}

func TestStore_Delete(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)
	s.Activate(context.Background(), "t1", "test-skill", ver) //nolint:errcheck
	if err := s.Delete(context.Background(), "t1", "test-skill"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(context.Background(), "t1", "test-skill")
	if !errors.Is(err, skill.ErrNotFound) {
		t.Errorf("want ErrNotFound after Delete, got %v", err)
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	err := s.Delete(context.Background(), "t1", "nonexistent")
	if err == nil {
		t.Errorf("want error for delete nonexistent")
	}
}

func TestStore_Activate_InvalidVersion(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	s.Create(context.Background(), "t1", spec) //nolint:errcheck
	err := s.Activate(context.Background(), "t1", "test-skill", "sha256:badversion")
	if !errors.Is(err, skill.ErrVersionConflict) {
		t.Errorf("want ErrVersionConflict, got %v", err)
	}
}

func TestStore_ListVersions(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	v1, _ := s.Create(context.Background(), "t1", spec)
	versions, err := s.ListVersions(context.Background(), "t1", "test-skill")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0] != v1 {
		t.Errorf("want [%q], got %v", v1, versions)
	}
}

func TestStore_TenantIsolation(t *testing.T) {
	repo := newRepo(t)
	s := git.New(repo, "")
	spec := makeSpec(t)
	ver1, _ := s.Create(context.Background(), "t1", spec)
	ver2, _ := s.Create(context.Background(), "t2", spec)
	if ver1 != ver2 {
		t.Errorf("same content should yield same version")
	}
	s.Activate(context.Background(), "t1", "test-skill", ver1) //nolint:errcheck
	// t2 should not see t1's activation.
	_, err := s.Get(context.Background(), "t2", "test-skill")
	if !errors.Is(err, skill.ErrNotFound) {
		t.Errorf("t2 should not see t1's data, got %v", err)
	}
}
