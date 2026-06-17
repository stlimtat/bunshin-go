package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stlimtat/bunshin-go/pkg/workflow"
	wfpostgres "github.com/stlimtat/bunshin-go/pkg/workflow/store/postgres"
)

// pool returns a *pgxpool.Pool connected to DATABASE_URL, or skips the test.
func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping Postgres integration tests")
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func migrate(t *testing.T, s *wfpostgres.Store) {
	t.Helper()
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
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

// cleanup removes test rows after each test.
func cleanup(t *testing.T, p *pgxpool.Pool, tenantID string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = p.Exec(context.Background(),
			`DELETE FROM bunshin_workflows WHERE tenant_id = $1`, tenantID)
	})
}

func TestPostgresStore_CreateAndGet_NoActive(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t1")

	spec := makeSpec(t, "wf-a")
	ver, err := s.Create(context.Background(), "pg-t1", spec)
	if err != nil || ver == "" {
		t.Fatalf("Create: ver=%q err=%v", ver, err)
	}
	_, err = s.Get(context.Background(), "pg-t1", "wf-a")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound before Activate, got %v", err)
	}
}

func TestPostgresStore_ActivateAndGet(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t2")

	spec := makeSpec(t, "wf-b")
	ver, _ := s.Create(context.Background(), "pg-t2", spec)
	if err := s.Activate(context.Background(), "pg-t2", "wf-b", ver); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, err := s.Get(context.Background(), "pg-t2", "wf-b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != ver {
		t.Errorf("want version %q, got %q", ver, got.Version)
	}
	if got.Status != workflow.StatusActive {
		t.Errorf("want active, got %q", got.Status)
	}
}

func TestPostgresStore_Create_Idempotent(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t3")

	spec := makeSpec(t, "wf-c")
	v1, _ := s.Create(context.Background(), "pg-t3", spec)
	v2, _ := s.Create(context.Background(), "pg-t3", spec)
	if v1 != v2 {
		t.Errorf("idempotent create must return same version: %q vs %q", v1, v2)
	}
}

func TestPostgresStore_GetVersion(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t4")

	spec := makeSpec(t, "wf-d")
	ver, _ := s.Create(context.Background(), "pg-t4", spec)
	got, err := s.GetVersion(context.Background(), "pg-t4", "wf-d", ver)
	if err != nil || got == nil {
		t.Fatalf("GetVersion: %v", err)
	}
}

func TestPostgresStore_GetVersion_Missing(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t5")

	s.Create(context.Background(), "pg-t5", makeSpec(t, "wf-e")) //nolint:errcheck
	_, err := s.GetVersion(context.Background(), "pg-t5", "wf-e", "sha256:nonexistent00000")
	if !errors.Is(err, workflow.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestPostgresStore_List(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t6")

	for _, name := range []string{"wf-x", "wf-y"} {
		s.Create(context.Background(), "pg-t6", makeSpec(t, name)) //nolint:errcheck
	}
	names, err := s.List(context.Background(), "pg-t6")
	if err != nil || len(names) != 2 {
		t.Errorf("want 2 names, got %v err %v", names, err)
	}
}

func TestPostgresStore_ListVersions_Order(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t7")

	spec1, _ := workflow.Parse([]byte("name: wf-v\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf-v\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))

	v1, _ := s.Create(context.Background(), "pg-t7", spec1)
	v2, _ := s.Create(context.Background(), "pg-t7", spec2)

	vers, err := s.ListVersions(context.Background(), "pg-t7", "wf-v")
	if err != nil || len(vers) != 2 {
		t.Fatalf("want 2 versions, got %v err %v", vers, err)
	}
	if vers[0] != v1 || vers[1] != v2 {
		t.Errorf("expected insertion order [v1, v2], got %v", vers)
	}
}

func TestPostgresStore_Delete_HidesFromList(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t8")

	s.Create(context.Background(), "pg-t8", makeSpec(t, "wf-del")) //nolint:errcheck
	if err := s.Delete(context.Background(), "pg-t8", "wf-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	names, _ := s.List(context.Background(), "pg-t8")
	if len(names) != 0 {
		t.Errorf("want empty after delete, got %v", names)
	}
}

func TestPostgresStore_Delete_Then_Create_Resurrects(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t9")

	spec := makeSpec(t, "wf-res")
	s.Create(context.Background(), "pg-t9", spec)  //nolint:errcheck
	s.Delete(context.Background(), "pg-t9", "wf-res") //nolint:errcheck

	ver, err := s.Create(context.Background(), "pg-t9", spec)
	if err != nil || ver == "" {
		t.Fatalf("resurrect Create: %v", err)
	}
	names, _ := s.List(context.Background(), "pg-t9")
	if len(names) != 1 {
		t.Errorf("want 1 after resurrect, got %v", names)
	}
}

func TestPostgresStore_Activate_UnknownVersion(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-t10")

	s.Create(context.Background(), "pg-t10", makeSpec(t, "wf-av")) //nolint:errcheck
	err := s.Activate(context.Background(), "pg-t10", "wf-av", "sha256:unknown00000000")
	if !errors.Is(err, workflow.ErrVersionConflict) {
		t.Errorf("want ErrVersionConflict, got %v", err)
	}
}

func TestPostgresStore_Activate_UnknownWorkflow(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)

	err := s.Activate(context.Background(), "pg-t11", "nonexistent", "sha256:x")
	if !errors.Is(err, workflow.ErrVersionConflict) {
		t.Errorf("want ErrVersionConflict, got %v", err)
	}
}

func TestPostgresStore_TenantIsolation(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-iso-a")
	cleanup(t, p, "pg-iso-b")

	s.Create(context.Background(), "pg-iso-a", makeSpec(t, "wf-shared")) //nolint:errcheck
	names, _ := s.List(context.Background(), "pg-iso-b")
	if len(names) != 0 {
		t.Errorf("tenant isolation broken: iso-b sees %v", names)
	}
}

func TestPostgresStore_RollForward(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)
	cleanup(t, p, "pg-rf")

	spec1, _ := workflow.Parse([]byte("name: wf-rf\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	spec2, _ := workflow.Parse([]byte("name: wf-rf\ndescription: v2\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))

	v1, _ := s.Create(context.Background(), "pg-rf", spec1)
	v2, _ := s.Create(context.Background(), "pg-rf", spec2)
	s.Activate(context.Background(), "pg-rf", "wf-rf", v1) //nolint:errcheck

	got, _ := s.Get(context.Background(), "pg-rf", "wf-rf")
	if got.Version != v1 {
		t.Errorf("want v1 active, got %q", got.Version)
	}
	s.Activate(context.Background(), "pg-rf", "wf-rf", v2) //nolint:errcheck
	got, _ = s.Get(context.Background(), "pg-rf", "wf-rf")
	if got.Version != v2 {
		t.Errorf("want v2 active after roll-forward, got %q", got.Version)
	}
}

func TestPostgresStore_Create_NilSpec(t *testing.T) {
	p := pool(t)
	s := wfpostgres.New(p)
	migrate(t, s)

	_, err := s.Create(context.Background(), "t", nil)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}
