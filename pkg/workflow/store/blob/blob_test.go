package blob_test

import (
	"context"
	"errors"
	"os"
	"testing"

	goblob "gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob" // local filesystem driver for tests

	"github.com/stlimtat/bunshin-go/pkg/workflow"
	blobstore "github.com/stlimtat/bunshin-go/pkg/workflow/store/blob"
)

// tempBucket opens a fileblob bucket backed by a temp directory.
func tempBucket(t *testing.T) *goblob.Bucket {
	t.Helper()
	dir, err := os.MkdirTemp("", "bunshin-blob-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	bucket, err := goblob.OpenBucket(context.Background(), "file://"+dir)
	if err != nil {
		t.Fatalf("OpenBucket: %v", err)
	}
	t.Cleanup(func() { bucket.Close() })
	return bucket
}

func store(t *testing.T) *blobstore.Store {
	t.Helper()
	return blobstore.New(tempBucket(t), "test/")
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

func TestBlobStore_Create_ReturnsVersion(t *testing.T) {
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

func TestBlobStore_Create_Idempotent(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-b")
	v1, _ := s.Create(context.Background(), "t1", spec)
	v2, _ := s.Create(context.Background(), "t1", spec)
	if v1 != v2 {
		t.Errorf("idempotent create must return same version: %q vs %q", v1, v2)
	}
}

func TestBlobStore_Create_NilSpec(t *testing.T) {
	s := store(t)
	_, err := s.Create(context.Background(), "t1", nil)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

// ---- Get ----

func TestBlobStore_Get_NoActive(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-c")) //nolint:errcheck
	_, err := s.Get(context.Background(), "t1", "wf-c")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound before Activate, got %v", err)
	}
}

func TestBlobStore_Get_AfterActivate(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-d")
	ver, _ := s.Create(context.Background(), "t1", spec)
	s.Activate(context.Background(), "t1", "wf-d", ver) //nolint:errcheck

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

func TestBlobStore_Get_Deleted_NotFound(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-e")
	ver, _ := s.Create(context.Background(), "t1", spec)
	s.Activate(context.Background(), "t1", "wf-e", ver)  //nolint:errcheck
	s.Delete(context.Background(), "t1", "wf-e")          //nolint:errcheck

	_, err := s.Get(context.Background(), "t1", "wf-e")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

// ---- GetVersion ----

func TestBlobStore_GetVersion(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-f")
	ver, _ := s.Create(context.Background(), "t1", spec)
	got, err := s.GetVersion(context.Background(), "t1", "wf-f", ver)
	if err != nil || got == nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got.Name != "wf-f" {
		t.Errorf("want wf-f, got %q", got.Name)
	}
}

func TestBlobStore_GetVersion_Missing(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-g")) //nolint:errcheck
	_, err := s.GetVersion(context.Background(), "t1", "wf-g", "sha256:nonexistent0000000")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- List ----

func TestBlobStore_List(t *testing.T) {
	s := store(t)
	for _, name := range []string{"wf-h", "wf-i"} {
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

func TestBlobStore_List_ExcludesDeleted(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-j"))   //nolint:errcheck
	s.Create(context.Background(), "t1", makeSpec(t, "wf-keep")) //nolint:errcheck
	s.Delete(context.Background(), "t1", "wf-j")                  //nolint:errcheck

	names, _ := s.List(context.Background(), "t1")
	for _, n := range names {
		if n == "wf-j" {
			t.Error("deleted workflow must not appear in List")
		}
	}
}

func TestBlobStore_List_EmptyTenant(t *testing.T) {
	s := store(t)
	names, err := s.List(context.Background(), "unknown-tenant")
	if err != nil || len(names) != 0 {
		t.Errorf("want empty list, got %v err %v", names, err)
	}
}

// ---- ListVersions ----

func TestBlobStore_ListVersions_Order(t *testing.T) {
	s := store(t)
	spec1, _ := workflow.Parse([]byte("name: wf-v\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf-v\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	v1, _ := s.Create(context.Background(), "t1", spec1)
	v2, _ := s.Create(context.Background(), "t1", spec2)

	vers, err := s.ListVersions(context.Background(), "t1", "wf-v")
	if err != nil || len(vers) != 2 {
		t.Fatalf("want 2 versions, got %v err %v", vers, err)
	}
	if vers[0] != v1 || vers[1] != v2 {
		t.Errorf("want insertion order [v1, v2], got %v", vers)
	}
}

func TestBlobStore_ListVersions_Deleted_NotFound(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-lv")) //nolint:errcheck
	s.Delete(context.Background(), "t1", "wf-lv")               //nolint:errcheck
	_, err := s.ListVersions(context.Background(), "t1", "wf-lv")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

// ---- Activate ----

func TestBlobStore_Activate_UnknownVersion(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "t1", makeSpec(t, "wf-av")) //nolint:errcheck
	err := s.Activate(context.Background(), "t1", "wf-av", "sha256:nonexistent0000000")
	if !errors.Is(err, workflow.ErrVersionConflict) {
		t.Errorf("want ErrVersionConflict, got %v", err)
	}
}

func TestBlobStore_Activate_RollForward(t *testing.T) {
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

// ---- Delete + Resurrect ----

func TestBlobStore_Delete_Then_Create_Resurrects(t *testing.T) {
	s := store(t)
	spec := makeSpec(t, "wf-res")
	s.Create(context.Background(), "t1", spec)   //nolint:errcheck
	s.Delete(context.Background(), "t1", "wf-res") //nolint:errcheck

	_, err := s.Create(context.Background(), "t1", spec)
	if err != nil {
		t.Fatalf("resurrect Create: %v", err)
	}
	names, _ := s.List(context.Background(), "t1")
	found := false
	for _, n := range names {
		if n == "wf-res" {
			found = true
		}
	}
	if !found {
		t.Error("resurrected workflow must appear in List")
	}
}

// ---- Tenant isolation ----

func TestBlobStore_TenantIsolation(t *testing.T) {
	s := store(t)
	s.Create(context.Background(), "tenant-a", makeSpec(t, "shared-wf")) //nolint:errcheck
	names, _ := s.List(context.Background(), "tenant-b")
	if len(names) != 0 {
		t.Errorf("tenant isolation broken: tenant-b sees %v", names)
	}
}
