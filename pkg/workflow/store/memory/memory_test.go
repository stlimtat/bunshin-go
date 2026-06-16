package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/workflow"
	"github.com/stlimtat/bunshin-go/pkg/workflow/store/memory"
)

const testYAML = `
name: test-flow
nodes:
  - id: step1
    runnable: {type: custom, name: x}
`

func makeSpec(t *testing.T) *workflow.Spec {
	t.Helper()
	s, err := workflow.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_CreateAndGet_NoActive(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	ver, err := s.Create(context.Background(), "t1", spec)
	if err != nil || ver == "" {
		t.Fatalf("Create failed: %v", err)
	}
	// No active version yet.
	_, err = s.Get(context.Background(), "t1", "test-flow")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound before Activate, got %v", err)
	}
}

func TestStore_ActivateAndGet(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)
	if err := s.Activate(context.Background(), "t1", "test-flow", ver); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := s.Get(context.Background(), "t1", "test-flow")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != ver {
		t.Errorf("want version %q, got %q", ver, got.Version)
	}
	if got.Status != workflow.StatusActive {
		t.Errorf("want status active, got %q", got.Status)
	}
}

func TestStore_Create_Idempotent(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	v1, _ := s.Create(context.Background(), "t1", spec)
	v2, _ := s.Create(context.Background(), "t1", spec)
	if v1 != v2 {
		t.Errorf("idempotent create must return same version: %q vs %q", v1, v2)
	}
}

func TestStore_GetVersion(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)
	got, err := s.GetVersion(context.Background(), "t1", "test-flow", ver)
	if err != nil || got == nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if got.Version != ver {
		t.Errorf("version mismatch: want %q, got %q", ver, got.Version)
	}
}

func TestStore_GetVersion_Missing(t *testing.T) {
	s := memory.New()
	makeSpec(t)
	s.Create(context.Background(), "t1", makeSpec(t)) //nolint:errcheck
	_, err := s.GetVersion(context.Background(), "t1", "test-flow", "sha256:nonexistent")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStore_List(t *testing.T) {
	s := memory.New()
	for _, yaml := range []string{
		"name: wf-a\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n",
		"name: wf-b\nnodes:\n  - id: b\n    runnable: {type: custom, name: x}\n",
	} {
		sp, _ := workflow.Parse([]byte(yaml))
		s.Create(context.Background(), "t1", sp) //nolint:errcheck
	}
	names, err := s.List(context.Background(), "t1")
	if err != nil || len(names) != 2 {
		t.Errorf("want 2 names, got %v err %v", names, err)
	}
}

func TestStore_List_EmptyTenant(t *testing.T) {
	s := memory.New()
	names, err := s.List(context.Background(), "unknown")
	if err != nil || len(names) != 0 {
		t.Errorf("want empty list, got %v err %v", names, err)
	}
}

func TestStore_ListVersions(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	s.Create(context.Background(), "t1", spec) //nolint:errcheck
	vers, err := s.ListVersions(context.Background(), "t1", "test-flow")
	if err != nil || len(vers) != 1 {
		t.Errorf("want 1 version, got %v err %v", vers, err)
	}
}

func TestStore_Delete_HidesFromList(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	s.Create(context.Background(), "t1", spec) //nolint:errcheck
	if err := s.Delete(context.Background(), "t1", "test-flow"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	names, _ := s.List(context.Background(), "t1")
	if len(names) != 0 {
		t.Errorf("want 0 names after delete, got %v", names)
	}
}

func TestStore_Delete_GetReturnsNotFound(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)
	s.Activate(context.Background(), "t1", "test-flow", ver) //nolint:errcheck
	s.Delete(context.Background(), "t1", "test-flow")        //nolint:errcheck

	_, err := s.Get(context.Background(), "t1", "test-flow")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestStore_Activate_UnknownVersion(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	s.Create(context.Background(), "t1", spec) //nolint:errcheck
	err := s.Activate(context.Background(), "t1", "test-flow", "sha256:unknown")
	if !errors.Is(err, workflow.ErrVersionConflict) {
		t.Errorf("want ErrVersionConflict, got %v", err)
	}
}

func TestStore_Activate_UnknownWorkflow(t *testing.T) {
	s := memory.New()
	err := s.Activate(context.Background(), "t1", "missing", "sha256:x")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStore_Create_NilSpec(t *testing.T) {
	s := memory.New()
	_, err := s.Create(context.Background(), "t1", nil)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestStore_TenantIsolation(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	s.Create(context.Background(), "tenant-a", spec) //nolint:errcheck

	// tenant-b must not see tenant-a's workflows.
	names, _ := s.List(context.Background(), "tenant-b")
	if len(names) != 0 {
		t.Errorf("tenant isolation broken: tenant-b sees %v", names)
	}
}

func TestStore_Delete_Then_Create_Resurrects(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	s.Create(context.Background(), "t1", spec) //nolint:errcheck
	s.Delete(context.Background(), "t1", "test-flow")  //nolint:errcheck

	// Create again after delete should succeed.
	ver, err := s.Create(context.Background(), "t1", spec)
	if err != nil || ver == "" {
		t.Fatalf("Create after Delete failed: ver=%q err=%v", ver, err)
	}
	// Workflow should be visible in List again.
	names, _ := s.List(context.Background(), "t1")
	if len(names) != 1 {
		t.Errorf("want 1 name after resurrect, got %v", names)
	}
}

func TestStore_ListVersions_Order(t *testing.T) {
	s := memory.New()
	ctx := context.Background()

	spec1, _ := workflow.Parse([]byte("name: wf\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))

	v1, _ := s.Create(ctx, "t1", spec1)
	v2, _ := s.Create(ctx, "t1", spec2)

	vers, _ := s.ListVersions(ctx, "t1", "wf")
	if len(vers) != 2 || vers[0] != v1 || vers[1] != v2 {
		t.Errorf("expected [v1, v2] insertion order, got %v", vers)
	}
}

func TestStore_GetVersion_ClonesSpec(t *testing.T) {
	s := memory.New()
	spec := makeSpec(t)
	ver, _ := s.Create(context.Background(), "t1", spec)

	got, _ := s.GetVersion(context.Background(), "t1", "test-flow", ver)
	// Mutate the returned slice — must not affect stored state.
	got.Nodes = nil

	got2, _ := s.GetVersion(context.Background(), "t1", "test-flow", ver)
	if len(got2.Nodes) == 0 {
		t.Error("GetVersion must return independent clone; mutation leaked into store")
	}
}

func TestStore_MultipleVersions_RollForward(t *testing.T) {
	s := memory.New()
	ctx := context.Background()

	spec1, _ := workflow.Parse([]byte("name: wf\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))

	v1, _ := s.Create(ctx, "t1", spec1)
	v2, _ := s.Create(ctx, "t1", spec2)

	s.Activate(ctx, "t1", "wf", v1) //nolint:errcheck
	active, _ := s.Get(ctx, "t1", "wf")
	if active.Version != v1 {
		t.Errorf("want v1 active, got %q", active.Version)
	}

	// Roll forward to v2.
	s.Activate(ctx, "t1", "wf", v2) //nolint:errcheck
	active, _ = s.Get(ctx, "t1", "wf")
	if active.Version != v2 {
		t.Errorf("want v2 active, got %q", active.Version)
	}

	vers, _ := s.ListVersions(ctx, "t1", "wf")
	if len(vers) != 2 {
		t.Errorf("want 2 versions, got %d", len(vers))
	}
}
