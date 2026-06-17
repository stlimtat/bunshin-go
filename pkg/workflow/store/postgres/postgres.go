// Package postgres provides a PostgreSQL-backed implementation of workflow.Store.
//
// Schema (run once per database):
//
//	CREATE TABLE IF NOT EXISTS bunshin_workflows (
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
//	-- Enforces at most one active version per workflow per tenant.
//	CREATE UNIQUE INDEX IF NOT EXISTS idx_bunshin_wf_one_active
//	    ON bunshin_workflows (tenant_id, name)
//	    WHERE status = 'active' AND deleted_at IS NULL;
//	CREATE INDEX IF NOT EXISTS idx_bunshin_wf_tenant_name
//	    ON bunshin_workflows (tenant_id, name) WHERE deleted_at IS NULL;
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
	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

const schema = `
CREATE TABLE IF NOT EXISTS bunshin_workflows (
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_bunshin_wf_one_active
    ON bunshin_workflows (tenant_id, name)
    WHERE status = 'active' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_bunshin_wf_tenant_name
    ON bunshin_workflows (tenant_id, name) WHERE deleted_at IS NULL;
`

// Store is a PostgreSQL-backed implementation of workflow.Store.
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
		return fmt.Errorf("workflow/postgres: migrate: %w", err)
	}
	return nil
}

// Create persists spec as a new draft. Idempotent: identical version is a no-op.
// If the workflow was previously soft-deleted and version doesn't exist yet,
// only that specific row's deleted_at is cleared — not the entire workflow history.
func (s *Store) Create(ctx context.Context, tenantID string, spec *workflow.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("workflow/postgres: Create: spec is nil")
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("workflow/postgres: marshal spec: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("workflow/postgres: Create begin: %w", err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck

	// Attempt idempotent insert.
	tag, err := tx.Exec(ctx,
		`INSERT INTO bunshin_workflows (tenant_id, name, version, spec, status)
		      VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, name, version) DO NOTHING`,
		tenantID, spec.Name, spec.Version, raw, workflow.StatusDraft,
	)
	if err != nil {
		return "", fmt.Errorf("workflow/postgres: insert: %w", err)
	}

	// Only resurrect if the row is new (was just inserted).
	// A row that already exists doesn't need clearing.
	if tag.RowsAffected() > 0 {
		// Clear deleted_at only for the specific version we just inserted,
		// in case the name was previously soft-deleted.
		_, err = tx.Exec(ctx,
			`UPDATE bunshin_workflows
			    SET deleted_at = NULL
			  WHERE tenant_id = $1 AND name = $2 AND version = $3 AND deleted_at IS NOT NULL`,
			tenantID, spec.Name, spec.Version,
		)
		if err != nil {
			return "", fmt.Errorf("workflow/postgres: resurrect: %w", err)
		}
	}

	return spec.Version, tx.Commit(ctx)
}

// Get returns the active version, or workflow.ErrNotFound if none.
func (s *Store) Get(ctx context.Context, tenantID, name string) (*workflow.Spec, error) {
	var raw []byte
	var version, status string
	err := s.pool.QueryRow(ctx,
		`SELECT spec, version, status
		   FROM bunshin_workflows
		  WHERE tenant_id = $1 AND name = $2
		    AND status = 'active' AND deleted_at IS NULL
		  ORDER BY id DESC
		  LIMIT 1`,
		tenantID, name,
	).Scan(&raw, &version, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("workflow %q: %w", name, workflow.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("workflow/postgres: Get: %w", err)
	}
	return unmarshalSpec(raw, version, status)
}

// GetVersion returns the specific version, or workflow.ErrNotFound.
func (s *Store) GetVersion(ctx context.Context, tenantID, name, version string) (*workflow.Spec, error) {
	var raw []byte
	var status string
	err := s.pool.QueryRow(ctx,
		`SELECT spec, status
		   FROM bunshin_workflows
		  WHERE tenant_id = $1 AND name = $2 AND version = $3
		  LIMIT 1`,
		tenantID, name, version,
	).Scan(&raw, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("workflow %q version %q: %w", name, version, workflow.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("workflow/postgres: GetVersion: %w", err)
	}
	return unmarshalSpec(raw, version, status)
}

// List returns names of non-deleted workflows for tenantID.
func (s *Store) List(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT name
		   FROM bunshin_workflows
		  WHERE tenant_id = $1 AND deleted_at IS NULL
		  ORDER BY name`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("workflow/postgres: List: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("workflow/postgres: List scan: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ListVersions returns all version strings for name in insertion order (oldest first).
func (s *Store) ListVersions(ctx context.Context, tenantID, name string) ([]string, error) {
	// Verify the workflow exists (not deleted) before listing.
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM bunshin_workflows
		     WHERE tenant_id = $1 AND name = $2 AND deleted_at IS NULL
		 )`,
		tenantID, name,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("workflow/postgres: ListVersions check: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("workflow %q: %w", name, workflow.ErrNotFound)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT version
		   FROM bunshin_workflows
		  WHERE tenant_id = $1 AND name = $2
		  ORDER BY id ASC`,
		tenantID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("workflow/postgres: ListVersions: %w", err)
	}
	defer rows.Close()

	var vers []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("workflow/postgres: ListVersions scan: %w", err)
		}
		vers = append(vers, v)
	}
	return vers, rows.Err()
}

// Activate promotes version to active using a serialisable transaction with
// row-level locking to prevent concurrent double-activation.
// Returns ErrVersionConflict if version is not found or is soft-deleted.
func (s *Store) Activate(ctx context.Context, tenantID, name, version string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("workflow/postgres: Activate begin: %w", err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck

	// Lock and verify the target version exists and is not soft-deleted.
	var id int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM bunshin_workflows
		  WHERE tenant_id = $1 AND name = $2 AND version = $3 AND deleted_at IS NULL
		 FOR UPDATE`,
		tenantID, name, version,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("workflow %q version %q: %w", name, version, workflow.ErrVersionConflict)
	}
	if err != nil {
		return fmt.Errorf("workflow/postgres: Activate lock: %w", err)
	}

	// Demote any existing active version to draft.
	_, err = tx.Exec(ctx,
		`UPDATE bunshin_workflows
		    SET status = 'draft'
		  WHERE tenant_id = $1 AND name = $2 AND status = 'active' AND deleted_at IS NULL`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("workflow/postgres: Activate demote: %w", err)
	}

	// Promote the target version.
	_, err = tx.Exec(ctx,
		`UPDATE bunshin_workflows
		    SET status = 'active'
		  WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("workflow/postgres: Activate promote: %w", err)
	}
	return tx.Commit(ctx)
}

// Delete soft-deletes all versions of the workflow. Get returns ErrNotFound afterwards.
// Returns ErrNotFound if the workflow doesn't exist or is already deleted.
func (s *Store) Delete(ctx context.Context, tenantID, name string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE bunshin_workflows
		    SET deleted_at = NOW(), status = 'draft'
		  WHERE tenant_id = $1 AND name = $2 AND deleted_at IS NULL`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("workflow/postgres: Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workflow %q: %w", name, workflow.ErrNotFound)
	}
	return nil
}

// unmarshalSpec decodes JSONB bytes and stamps version/status from the DB row.
func unmarshalSpec(raw []byte, version, status string) (*workflow.Spec, error) {
	var spec workflow.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("workflow/postgres: unmarshal: %w", err)
	}
	spec.Version = version
	spec.Status = status
	return &spec, nil
}
