// Package postgres provides a PostgreSQL-backed implementation of skill.Store.
//
// Schema (run once per database):
//
//	CREATE TABLE IF NOT EXISTS bunshin_skills (
//	    id         BIGSERIAL    PRIMARY KEY,
//	    tenant_id  TEXT         NOT NULL,
//	    name       TEXT         NOT NULL,
//	    version    TEXT         NOT NULL,
//	    spec       JSONB        NOT NULL,
//	    status     TEXT         NOT NULL DEFAULT 'draft',
//	    deleted_at TIMESTAMPTZ,
//	    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
//	    UNIQUE (tenant_id, name, version)
//	);
//	CREATE INDEX IF NOT EXISTS idx_bunshin_skills_tenant_name
//	    ON bunshin_skills (tenant_id, name) WHERE deleted_at IS NULL;
//
// Use Migrate() to apply the schema automatically.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stlimtat/bunshin-go/pkg/skill"
)

const schema = `
CREATE TABLE IF NOT EXISTS bunshin_skills (
    id         BIGSERIAL    PRIMARY KEY,
    tenant_id  TEXT         NOT NULL,
    name       TEXT         NOT NULL,
    version    TEXT         NOT NULL,
    spec       JSONB        NOT NULL,
    status     TEXT         NOT NULL DEFAULT 'draft',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name, version)
);
CREATE INDEX IF NOT EXISTS idx_bunshin_skills_tenant_name
    ON bunshin_skills (tenant_id, name) WHERE deleted_at IS NULL;
`

// Store is a PostgreSQL-backed implementation of skill.Store.
// Multiple Store instances may share the same *pgxpool.Pool.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store backed by pool. Call Migrate to ensure the schema exists.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Migrate applies the table and index DDL idempotently.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("skill/postgres: migrate: %w", err)
	}
	return nil
}

// Create persists spec as a new draft. Idempotent: identical version is a no-op.
// Resurrects soft-deleted entries (clears deleted_at).
func (s *Store) Create(ctx context.Context, tenantID string, spec *skill.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("skill/postgres: Create: spec is nil")
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("skill/postgres: marshal spec: %w", err)
	}

	// Resurrect any soft-deleted rows for this name first.
	_, err = s.pool.Exec(ctx,
		`UPDATE bunshin_skills
		    SET deleted_at = NULL
		  WHERE tenant_id = $1 AND name = $2 AND deleted_at IS NOT NULL`,
		tenantID, spec.Name,
	)
	if err != nil {
		return "", fmt.Errorf("skill/postgres: resurrect: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO bunshin_skills (tenant_id, name, version, spec, status)
		      VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, name, version) DO NOTHING`,
		tenantID, spec.Name, spec.Version, raw, skill.StatusDraft,
	)
	if err != nil {
		return "", fmt.Errorf("skill/postgres: insert: %w", err)
	}
	return spec.Version, nil
}

// Get returns the active version, or skill.ErrNotFound if none.
func (s *Store) Get(ctx context.Context, tenantID, name string) (*skill.Spec, error) {
	var raw []byte
	var version, status string
	err := s.pool.QueryRow(ctx,
		`SELECT spec, version, status
		   FROM bunshin_skills
		  WHERE tenant_id = $1 AND name = $2 AND status = 'active' AND deleted_at IS NULL
		  LIMIT 1`,
		tenantID, name,
	).Scan(&raw, &version, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("skill/postgres: Get: %w", err)
	}
	return unmarshalSpec(raw, version, status)
}

// GetVersion returns the specific version, or skill.ErrNotFound.
func (s *Store) GetVersion(ctx context.Context, tenantID, name, version string) (*skill.Spec, error) {
	var raw []byte
	var status string
	err := s.pool.QueryRow(ctx,
		`SELECT spec, status
		   FROM bunshin_skills
		  WHERE tenant_id = $1 AND name = $2 AND version = $3
		  LIMIT 1`,
		tenantID, name, version,
	).Scan(&raw, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("skill/postgres: GetVersion: %w", err)
	}
	return unmarshalSpec(raw, version, status)
}

// List returns names of non-deleted skills for tenantID.
func (s *Store) List(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT name
		   FROM bunshin_skills
		  WHERE tenant_id = $1 AND deleted_at IS NULL
		  ORDER BY name`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("skill/postgres: List: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("skill/postgres: List scan: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ListVersions returns all version strings for name in insertion order (oldest first).
func (s *Store) ListVersions(ctx context.Context, tenantID, name string) ([]string, error) {
	if _, err := s.findName(ctx, tenantID, name); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT version
		   FROM bunshin_skills
		  WHERE tenant_id = $1 AND name = $2
		  ORDER BY id ASC`,
		tenantID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("skill/postgres: ListVersions: %w", err)
	}
	defer rows.Close()

	var vers []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("skill/postgres: ListVersions scan: %w", err)
		}
		vers = append(vers, v)
	}
	return vers, rows.Err()
}

// Activate promotes version to active using an optimistic-lock transaction.
// Returns skill.ErrVersionConflict if version is not found.
func (s *Store) Activate(ctx context.Context, tenantID, name, version string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("skill/postgres: Activate begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify the target version exists.
	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM bunshin_skills
		     WHERE tenant_id = $1 AND name = $2 AND version = $3
		 )`,
		tenantID, name, version,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("skill/postgres: Activate check: %w", err)
	}
	if !exists {
		return fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrVersionConflict)
	}

	// Demote existing active version to draft.
	_, err = tx.Exec(ctx,
		`UPDATE bunshin_skills
		    SET status = 'draft'
		  WHERE tenant_id = $1 AND name = $2 AND status = 'active'`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("skill/postgres: Activate demote: %w", err)
	}

	// Promote the target version.
	_, err = tx.Exec(ctx,
		`UPDATE bunshin_skills
		    SET status = 'active'
		  WHERE tenant_id = $1 AND name = $2 AND version = $3`,
		tenantID, name, version,
	)
	if err != nil {
		return fmt.Errorf("skill/postgres: Activate promote: %w", err)
	}
	return tx.Commit(ctx)
}

// Delete soft-deletes the skill. Get returns skill.ErrNotFound afterwards.
func (s *Store) Delete(ctx context.Context, tenantID, name string) error {
	if _, err := s.findName(ctx, tenantID, name); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE bunshin_skills
		    SET deleted_at = NOW(), status = 'draft'
		  WHERE tenant_id = $1 AND name = $2 AND deleted_at IS NULL`,
		tenantID, name,
	)
	return err
}

// findName checks whether a non-deleted skill named name exists.
func (s *Store) findName(ctx context.Context, tenantID, name string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM bunshin_skills
		     WHERE tenant_id = $1 AND name = $2 AND deleted_at IS NULL
		 )`,
		tenantID, name,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("skill/postgres: findName: %w", err)
	}
	if !exists {
		return false, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	return true, nil
}

// unmarshalSpec decodes JSONB bytes and stamps version/status from the DB row.
func unmarshalSpec(raw []byte, version, status string) (*skill.Spec, error) {
	var spec skill.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("skill/postgres: unmarshal: %w", err)
	}
	spec.Version = version
	spec.Status = status
	return &spec, nil
}
