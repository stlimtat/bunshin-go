package git_test

import (
	"context"
	"errors"
	"os"
	"testing"

	gogit "github.com/go-git/go-git/v5"

	"github.com/stlimtat/bunshin-go/pkg/workflow"
	gitstore "github.com/stlimtat/bunshin-go/pkg/workflow/store/git"
)

// tempRepo initialises a bare-like git repo in a temp directory.
func tempRepo(t *testing.T) *gogit.Repository {
	t.Helper()
	dir, err := os.MkdirTemp("", "bunshin-git-test-*")
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

func store(t *testing.T) *gitstore.Store {
	t.Helper()
	return gitstore.New(tempRepo(t), "")
}

func makeSpec(t *testing.T, name string) *workflow.Spec {
	t.Helper()
	yaml := "name: " + name + "\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"
	s, err := workflow.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

// ---- Create ----

func TestGitStore_Create_ReturnsVersion(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-a")
	ver, err := s.Create(context.Background(), "t1", spec)
	if err != nil || ver == "" {
		t.Fatalf("Create: ver=%q err=%v", ver, err)
	}
	if ver != spec.Version {
		t.Errorf("want %q, got %q", spec.Version, ver)
	}
}

func TestGitStore_Create_Idempotent(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-b")
	v1, _ := s.Create(context.Background(), "t1", spec)
	v2, _ := s.Create(context.Background(), "t1", spec)
	if v1 != v2 {
		t.Errorf("idempotent create must return same version: %q vs %q", v1, v2)
	}
}

func TestGitStore_Create_NilSpec(t *testing.T) {
	s := store(t)
	_, err := s.Create(context.Background(), "t1", nil)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

// ---- Get ----

func TestGitStore_Get_NoActive(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-c")) //nolint:errcheck
	_, err := s.Get(context.Background(), "t1", "wf-c")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound before Activate, got %v", err)
	}
}

func TestGitStore_Get_AfterActivate(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-d")
	ver, _ := s.Create(context.Background(), "t1", spec)
	if err := s.Activate(context.Background(), "t1", "wf-d", ver); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := s.Get(context.Background(), "t1", "wf-d")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != ver {
		t.Errorf("want %q, got %q", ver, got.Version)
	}
	if got.Status != workflow.StatusActive {
		t.Errorf("want active, got %q", got.Status)
	}
}

// ---- GetVersion ----

func TestGitStore_GetVersion(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-e")
	ver, _ := s.Create(context.Background(), "t1", spec)
	got, err := s.GetVersion(context.Background(), "t1", "wf-e", ver)
	if err != nil || got == nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got.Name != "wf-e" {
		t.Errorf("want wf-e, got %q", got.Name)
	}
}

func TestGitStore_GetVersion_Missing(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-f")) //nolint:errcheck
	_, err := s.GetVersion(context.Background(), "t1", "wf-f", "sha256:nonexistent0000000")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- List ----

func TestGitStore_List(t *testing.T) {
	s := store(t)
	for _, name := range []string{"wf-g", "wf-h"} {
		s.Create(context.Background(), "t1", makeSpec(t, name)) //nolint:errcheck
	}
	names, err := s.List(context.Background(), "t1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("want 2 names, got %v", names)
	}
}

func TestGitStore_List_EmptyTenant(t *testing.T) {
	s := store(t)
	names, err := s.List(context.Background(), "new-tenant")
	if err != nil || len(names) != 0 {
		t.Errorf("want empty list, got %v err %v", names, err)
	}
}

// ---- ListVersions ----

func TestGitStore_ListVersions(t *testing.T) {
	s := store(t)
	spec1, _ := workflow.Parse([]byte("name: wf-v\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf-v\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	s.Create(context.Background(), "t1", spec1) //nolint:errcheck
	s.Create(context.Background(), "t1", spec2) //nolint:errcheck

	vers, err := s.ListVersions(context.Background(), "t1", "wf-v")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(vers) != 2 {
		t.Errorf("want 2 versions, got %v", vers)
	}
}

func TestGitStore_ListVersions_Missing(t *testing.T) {
	s := store(t)
	_, err := s.ListVersions(context.Background(), "t1", "nonexistent")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- Activate ----

func TestGitStore_Activate_UnknownVersion(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-av")) //nolint:errcheck
	err := s.Activate(context.Background(), "t1", "wf-av", "sha256:nonexistent0000000")
	if !errors.Is(err, workflow.ErrVersionConflict) {
		t.Errorf("want ErrVersionConflict, got %v", err)
	}
}

func TestGitStore_Activate_RollForward(t *testing.T) {
	s := store(t)
	spec1, _ := workflow.Parse([]byte("name: wf-rf\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf-rf\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	v1, _ := s.Create(context.Background(), "t1", spec1)
	v2, _ := s.Create(context.Background(), "t1", spec2)

	s.Activate(context.Background(), "t1", "wf-rf", v1) //nolint:errcheck
	got, _ := s.Get(context.Background(), "t1", "wf-rf")
	if got.Version != v1 {
		t.Errorf("want v1, got %q", got.Version)
	}

	s.Activate(context.Background(), "t1", "wf-rf", v2) //nolint:errcheck
	got, _ = s.Get(context.Background(), "t1", "wf-rf")
	if got.Version != v2 {
		t.Errorf("want v2 after roll-forward, got %q", got.Version)
	}
}

// ---- Delete ----

func TestGitStore_Delete_RemovesFromList(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-del")) //nolint:errcheck
	if err := s.Delete(context.Background(), "t1", "wf-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	names, _ := s.List(context.Background(), "t1")
	for _, n := range names {
		if n == "wf-del" {
			t.Error("deleted workflow must not appear in List")
		}
	}
}

func TestGitStore_Delete_NotFound(t *testing.T) {
	s := store(t)
	err := s.Delete(context.Background(), "t1", "nonexistent")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- Tenant isolation ----

func TestGitStore_TenantIsolation(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "tenant-a", makeSpec(t, "shared")) //nolint:errcheck
	names, _ := s.List(context.Background(), "tenant-b")
	if len(names) != 0 {
		t.Errorf("tenant isolation broken: tenant-b sees %v", names)
	}
}
