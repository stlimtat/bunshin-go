package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stlimtat/bunshin-go/pkg/skill"
	"github.com/stlimtat/bunshin-go/pkg/skill/store/postgres"
)

const testYAML = `
name: test-skill
description: A test skill
body: {slug: instructions}
trigger: model
`

func dbPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping Postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// pgxpool.New is lazy; Ping forces a real connection so an unreachable DB skips.
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("Postgres unreachable — skipping: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func makeSpec(t *testing.T) *skill.Spec {
	t.Helper()
	s, err := skill.Parse([]byte(testYAML))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_Migrate(t *testing.T) {
	pool := dbPool(t)
	s := postgres.New(pool)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

func TestStore_CreateAndGet_NoActive(t *testing.T) {
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
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
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
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
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
	spec := makeSpec(t)
	v1, _ := s.Create(context.Background(), "t1", spec)
	v2, _ := s.Create(context.Background(), "t1", spec)
	if v1 != v2 {
		t.Errorf("idempotent create must return same version: %q vs %q", v1, v2)
	}
}

func TestStore_GetVersion(t *testing.T) {
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
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

func TestStore_List(t *testing.T) {
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
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
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
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

func TestStore_ListVersions(t *testing.T) {
	pool := dbPool(t)
	s := postgres.New(pool)
	s.Migrate(context.Background()) //nolint:errcheck
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
