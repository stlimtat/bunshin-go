package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is a PromptBackend backed by PostgreSQL with roll-forward versioning.
// One instance serves all tenants — tenantID is passed per call.
//
// Schema migration:
//
//	CREATE TABLE IF NOT EXISTS bunshin_fragments (
//	    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
//	    slug        TEXT         NOT NULL,
//	    tenant_id   TEXT         NOT NULL,
//	    version     TEXT         NOT NULL,
//	    status      TEXT         NOT NULL DEFAULT 'draft',
//	    content     JSONB        NOT NULL,
//	    created_at  TIMESTAMPTZ  DEFAULT NOW(),
//	    UNIQUE (tenant_id, slug, version)
//	);
//	CREATE UNIQUE INDEX IF NOT EXISTS bunshin_fragments_active
//	    ON bunshin_fragments (tenant_id, slug)
//	    WHERE status = 'active';
//	CREATE INDEX IF NOT EXISTS bunshin_fragments_by_uuid
//	    ON bunshin_fragments (id);
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore constructs a store backed by pool.
// Call Migrate to apply the schema.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const pgFragSchema = `
CREATE TABLE IF NOT EXISTS bunshin_fragments (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT         NOT NULL,
    tenant_id   TEXT         NOT NULL,
    version     TEXT         NOT NULL,
    status      TEXT         NOT NULL DEFAULT 'draft',
    content     JSONB        NOT NULL,
    created_at  TIMESTAMPTZ  DEFAULT NOW(),
    UNIQUE (tenant_id, slug, version)
);
CREATE UNIQUE INDEX IF NOT EXISTS bunshin_fragments_active
    ON bunshin_fragments (tenant_id, slug)
    WHERE status = 'active';
CREATE INDEX IF NOT EXISTS bunshin_fragments_by_uuid
    ON bunshin_fragments (id);
`

// Migrate applies the schema idempotently.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, pgFragSchema); err != nil {
		return fmt.Errorf("postgres store: migrate: %w", err)
	}
	return nil
}

// Put saves a fragment as a new draft. Generates a UUIDv4 if f.ID is empty.
func (s *PostgresStore) Put(ctx context.Context, tenantID string, f *Fragment) error {
	if f.Slug == "" {
		return fmt.Errorf("postgres store: Put: fragment Slug must not be empty")
	}
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("postgres store: marshal: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO bunshin_fragments (id, slug, tenant_id, version, status, content)
		 VALUES ($1, $2, $3, $4, 'draft', $5)
		 ON CONFLICT (tenant_id, slug, version) DO NOTHING`,
		f.ID, f.Slug, tenantID, f.Version, data,
	)
	if err != nil {
		return fmt.Errorf("postgres store: put: %w", err)
	}
	return nil
}

// Get returns the active version by slug.
func (s *PostgresStore) Get(ctx context.Context, tenantID, slug string) (*Fragment, error) {
	var data []byte
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id, content FROM bunshin_fragments
		  WHERE tenant_id = $1 AND slug = $2 AND status = 'active'
		  LIMIT 1`,
		tenantID, slug,
	).Scan(&id, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("fragment slug=%q tenant=%q: not found", slug, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres store: get %q: %w", slug, err)
	}
	f, err := decodeFragment(data)
	if err != nil {
		return nil, err
	}
	f.ID = id
	return f, nil
}

// GetByID returns a fragment by its UUID.
func (s *PostgresStore) GetByID(ctx context.Context, _, id string) (*Fragment, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM bunshin_fragments
		  WHERE id = $1
		  LIMIT 1`,
		id,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("fragment id=%q: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres store: get by id %q: %w", id, err)
	}
	f, err := decodeFragment(data)
	if err != nil {
		return nil, err
	}
	f.ID = id
	return f, nil
}

// GetVersion returns a specific version by slug.
func (s *PostgresStore) GetVersion(ctx context.Context, tenantID, slug, version string) (*Fragment, error) {
	var data []byte
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id, content FROM bunshin_fragments
		  WHERE tenant_id = $1 AND slug = $2 AND version = $3
		  LIMIT 1`,
		tenantID, slug, version,
	).Scan(&id, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("fragment slug=%q version=%q: not found", slug, version)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres store: get version %q@%q: %w", slug, version, err)
	}
	f, err := decodeFragment(data)
	if err != nil {
		return nil, err
	}
	f.ID = id
	return f, nil
}

// List returns all active fragments for tenantID. Tag filtering is in-process.
func (s *PostgresStore) List(ctx context.Context, tenantID string, tags ...string) ([]*Fragment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, content FROM bunshin_fragments
		  WHERE tenant_id = $1 AND status = 'active'`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres store: list: %w", err)
	}
	defer rows.Close()

	var out []*Fragment
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("postgres store: scan: %w", err)
		}
		f, err := decodeFragment(data)
		if err != nil {
			return nil, err
		}
		f.ID = id
		if len(tags) == 0 || hasAllTags(f, tags) {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

// Rename changes the slug for all rows with the given UUID to newSlug.
func (s *PostgresStore) Rename(ctx context.Context, _, id, newSlug string) error {
	if newSlug == "" {
		return fmt.Errorf("postgres store: Rename: newSlug must not be empty")
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE bunshin_fragments SET slug = $1 WHERE id = $2`,
		newSlug, id,
	)
	if err != nil {
		return fmt.Errorf("postgres store: rename: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("fragment id=%q: not found", id)
	}
	return nil
}

// Watch is not supported — use PromptCache for live updates.
func (s *PostgresStore) Watch(_ context.Context, _, _ string) (<-chan *Fragment, error) {
	ch := make(chan *Fragment)
	close(ch)
	return ch, nil
}

// Promote transitions the newest draft to active for the given slug or UUID.
// ref may be a UUID (tries first) or a slug (fallback).
func (s *PostgresStore) Promote(ctx context.Context, tenantID, ref string) error {
	slug, err := s.resolveSlug(ctx, tenantID, ref)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE bunshin_fragments
		    SET status = 'active'
		  WHERE tenant_id = $1 AND slug = $2
		    AND version = (
		        SELECT version FROM bunshin_fragments
		         WHERE tenant_id = $1 AND slug = $2 AND status = 'draft'
		         ORDER BY created_at DESC
		         LIMIT 1
		    )`,
		tenantID, slug,
	)
	if err != nil {
		return fmt.Errorf("postgres store: promote %q: %w", ref, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("postgres store: promote %q: no draft found", ref)
	}
	return nil
}

// resolveSlug resolves ref to a slug: tries UUID parse first, then slug lookup.
func (s *PostgresStore) resolveSlug(ctx context.Context, tenantID, ref string) (string, error) {
	if _, err := uuid.Parse(ref); err == nil {
		// ref is a UUID — look up the current slug.
		var slug string
		err := s.pool.QueryRow(ctx,
			`SELECT slug FROM bunshin_fragments
			  WHERE id = $1 AND tenant_id = $2
			  LIMIT 1`,
			ref, tenantID,
		).Scan(&slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("fragment id=%q tenant=%q: not found", ref, tenantID)
		}
		if err != nil {
			return "", fmt.Errorf("postgres store: resolve uuid %q: %w", ref, err)
		}
		return slug, nil
	}
	// ref is a slug — verify it exists.
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM bunshin_fragments WHERE tenant_id = $1 AND slug = $2)`,
		tenantID, ref,
	).Scan(&exists)
	if err != nil {
		return "", fmt.Errorf("postgres store: resolve slug %q: %w", ref, err)
	}
	if !exists {
		return "", fmt.Errorf("fragment slug=%q tenant=%q: not found", ref, tenantID)
	}
	return ref, nil
}

func decodeFragment(data []byte) (*Fragment, error) {
	var f Fragment
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("postgres store: decode: %w", err)
	}
	return &f, nil
}
