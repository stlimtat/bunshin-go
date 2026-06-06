package prompt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is a PromptBackend backed by PostgreSQL with roll-forward versioning.
//
// Each fragment has a status: "draft" or "active". Promote transitions the latest
// draft to active for that fragment ID. This is the only allowed state transition
// (roll-forward only — no rollback).
//
// Schema (create once per database):
//
//	CREATE TABLE IF NOT EXISTS bunshin_fragments (
//	    id          BIGSERIAL PRIMARY KEY,
//	    fragment_id TEXT   NOT NULL,
//	    tenant_id   TEXT   NOT NULL,
//	    version     TEXT   NOT NULL,
//	    status      TEXT   NOT NULL DEFAULT 'draft',
//	    content     JSONB  NOT NULL,
//	    created_at  TIMESTAMPTZ DEFAULT NOW()
//	);
//	CREATE UNIQUE INDEX IF NOT EXISTS bunshin_fragments_active
//	    ON bunshin_fragments (tenant_id, fragment_id)
//	    WHERE status = 'active';
//	CREATE INDEX IF NOT EXISTS bunshin_fragments_lookup
//	    ON bunshin_fragments (tenant_id, fragment_id, version);
type PostgresStore struct {
	pool     *pgxpool.Pool
	tenantID string
}

// NewPostgresStore constructs a store scoped to a tenant.
func NewPostgresStore(pool *pgxpool.Pool, tenantID string) *PostgresStore {
	return &PostgresStore{pool: pool, tenantID: tenantID}
}

const pgFragInsert = `
INSERT INTO bunshin_fragments (fragment_id, tenant_id, version, status, content)
VALUES ($1, $2, $3, 'draft', $4)`

// Put saves a fragment as a new draft. Does not affect the active version.
func (s *PostgresStore) Put(ctx context.Context, f *Fragment) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("postgres store: marshal: %w", err)
	}
	_, err = s.pool.Exec(ctx, pgFragInsert, f.ID, s.tenantID, f.Version, data)
	if err != nil {
		return fmt.Errorf("postgres store: put: %w", err)
	}
	return nil
}

const pgFragGetActive = `
SELECT content FROM bunshin_fragments
WHERE tenant_id = $1 AND fragment_id = $2 AND status = 'active'
LIMIT 1`

// Get returns the active version of a fragment.
func (s *PostgresStore) Get(ctx context.Context, id string) (*Fragment, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, pgFragGetActive, s.tenantID, id).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("postgres store: get %q: %w", id, err)
	}
	return decodeFragment(data)
}

const pgFragGetVersion = `
SELECT content FROM bunshin_fragments
WHERE tenant_id = $1 AND fragment_id = $2 AND version = $3
LIMIT 1`

// GetVersion returns a specific version (any status).
func (s *PostgresStore) GetVersion(ctx context.Context, id, version string) (*Fragment, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, pgFragGetVersion, s.tenantID, id, version).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("postgres store: get version %q@%q: %w", id, version, err)
	}
	return decodeFragment(data)
}

const pgFragList = `
SELECT content FROM bunshin_fragments
WHERE tenant_id = $1 AND status = 'active'`

// List returns all active fragments for the tenant.
// Tag filtering is applied in-process.
func (s *PostgresStore) List(ctx context.Context, tags ...string) ([]*Fragment, error) {
	rows, err := s.pool.Query(ctx, pgFragList, s.tenantID)
	if err != nil {
		return nil, fmt.Errorf("postgres store: list: %w", err)
	}
	defer rows.Close()

	var out []*Fragment
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("postgres store: scan: %w", err)
		}
		f, err := decodeFragment(data)
		if err != nil {
			return nil, err
		}
		if len(tags) == 0 || hasAllTags(f, tags) {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

// Watch is not supported by PostgresStore — use PromptCache for live updates.
// Returns a closed channel.
func (s *PostgresStore) Watch(_ context.Context, _ string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment)
	close(ch)
	return ch, nil
}

const pgFragPromote = `
UPDATE bunshin_fragments
SET status = 'active'
WHERE tenant_id = $1 AND fragment_id = $2
  AND version = (
      SELECT version FROM bunshin_fragments
      WHERE tenant_id = $1 AND fragment_id = $2 AND status = 'draft'
      ORDER BY created_at DESC
      LIMIT 1
  )`

// Promote transitions the newest draft to active (roll-forward only).
// Returns an error if no draft exists.
func (s *PostgresStore) Promote(ctx context.Context, fragmentID string) error {
	cmd, err := s.pool.Exec(ctx, pgFragPromote, s.tenantID, fragmentID)
	if err != nil {
		return fmt.Errorf("postgres store: promote %q: %w", fragmentID, err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("postgres store: promote %q: no draft found", fragmentID)
	}
	return nil
}

func decodeFragment(data []byte) (*Fragment, error) {
	var f Fragment
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("postgres store: decode: %w", err)
	}
	return &f, nil
}
