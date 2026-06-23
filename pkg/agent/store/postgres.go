package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stlimtat/bunshin-go/pkg/agent"
	"gopkg.in/yaml.v3"
)

// PostgresStore is a Store backed by PostgreSQL with content-hash versioning.
// One instance serves all tenants — tenantID is passed per call.
//
// Schema migration:
//
//	CREATE TABLE IF NOT EXISTS bunshin_agents (
//	    id         TEXT         PRIMARY KEY DEFAULT gen_random_uuid()::text,
//	    tenant_id  TEXT         NOT NULL,
//	    name       TEXT         NOT NULL,
//	    version    TEXT         NOT NULL,
//	    content    BYTEA        NOT NULL,
//	    status     TEXT         NOT NULL DEFAULT 'draft',
//	    created_at TIMESTAMPTZ  DEFAULT now(),
//	    UNIQUE (tenant_id, name, version)
//	);
//	CREATE INDEX IF NOT EXISTS idx_agents_tenant_name_status
//	    ON bunshin_agents(tenant_id, name, status);
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore constructs a store backed by pool.
// Call Migrate to apply the schema.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const pgAgentSchema = `
CREATE TABLE IF NOT EXISTS bunshin_agents (
    id         TEXT         PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id  TEXT         NOT NULL,
    name       TEXT         NOT NULL,
    version    TEXT         NOT NULL,
    content    BYTEA        NOT NULL,
    status     TEXT         NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ  DEFAULT now(),
    UNIQUE (tenant_id, name, version)
);
CREATE INDEX IF NOT EXISTS idx_agents_tenant_name_status
    ON bunshin_agents(tenant_id, name, status);
`

// Migrate applies the schema idempotently.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, pgAgentSchema); err != nil {
		return fmt.Errorf("agent/postgres: migrate: %w", err)
	}
	return nil
}

// Create persists spec as a new draft. Idempotent: same content = same version.
// If the agent was previously soft-deleted, Create resurrects it.
func (s *PostgresStore) Create(ctx context.Context, tenantID string, spec *agent.AgentSpec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("agent/postgres: Create: spec is nil")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("agent/postgres: Create: spec.Name must not be empty")
	}

	version, err := contentHashYAML(spec)
	if err != nil {
		return "", fmt.Errorf("agent/postgres: hash: %w", err)
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("agent/postgres: marshal: %w", err)
	}

	// Resurrect any soft-deleted rows for this name first.
	_, err = s.pool.Exec(ctx,
		`UPDATE bunshin_agents
		    SET status = 'draft'
		  WHERE tenant_id = $1 AND name = $2 AND status = 'deleted'`,
		tenantID, spec.Name,
	)
	if err != nil {
		return "", fmt.Errorf("agent/postgres: resurrect: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO bunshin_agents (tenant_id, name, version, content, status)
		      VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, name, version) DO NOTHING`,
		tenantID, spec.Name, version, data, "draft",
	)
	if err != nil {
		return "", fmt.Errorf("agent/postgres: insert: %w", err)
	}
	return version, nil
}

// Get returns the active version by name, or an error if none exists.
func (s *PostgresStore) Get(ctx context.Context, tenantID, name string) (*agent.AgentSpec, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM bunshin_agents
		  WHERE tenant_id = $1 AND name = $2 AND status = 'active'
		  LIMIT 1`,
		tenantID, name,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("agent %q tenant %q: no active version", name, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("agent/postgres: get %q: %w", name, err)
	}
	spec, err := decodeAgentSpec(data)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// GetVersion returns a specific version by name, or an error if absent.
func (s *PostgresStore) GetVersion(ctx context.Context, tenantID, name, version string) (*agent.AgentSpec, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM bunshin_agents
		  WHERE tenant_id = $1 AND name = $2 AND version = $3
		  LIMIT 1`,
		tenantID, name, version,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("agent/postgres: get version %q@%q: %w", name, version, err)
	}
	spec, err := decodeAgentSpec(data)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// List returns all non-deleted agent names for tenantID.
func (s *PostgresStore) List(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT name FROM bunshin_agents
		  WHERE tenant_id = $1 AND status != 'deleted'`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("agent/postgres: list: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("agent/postgres: scan: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ListVersions returns all versions of the agent for tenantID/name, newest-first.
func (s *PostgresStore) ListVersions(ctx context.Context, tenantID, name string) ([]AgentVersion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT version, status, created_at FROM bunshin_agents
		  WHERE tenant_id = $1 AND name = $2
		  ORDER BY created_at DESC`,
		tenantID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("agent/postgres: list versions %q: %w", name, err)
	}
	defer rows.Close()

	var out []AgentVersion
	for rows.Next() {
		var version, status string
		var createdAt time.Time
		if err := rows.Scan(&version, &status, &createdAt); err != nil {
			return nil, fmt.Errorf("agent/postgres: scan version: %w", err)
		}
		out = append(out, AgentVersion{
			Version:   version,
			Status:    status,
			CreatedAt: createdAt,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}
	return out, rows.Err()
}

// Activate promotes a draft version to active for tenantID/name.
func (s *PostgresStore) Activate(ctx context.Context, tenantID, name, version string) error {
	// Verify the version exists.
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM bunshin_agents
		   WHERE tenant_id = $1 AND name = $2 AND version = $3)`,
		tenantID, name, version,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("agent/postgres: activate check: %w", err)
	}
	if !exists {
		return fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
	}

	// Mark any previous active as draft.
	_, err = s.pool.Exec(ctx,
		`UPDATE bunshin_agents
		    SET status = 'draft'
		  WHERE tenant_id = $1 AND name = $2 AND status = 'active'`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("agent/postgres: deactivate old: %w", err)
	}

	// Activate the new version.
	tag, err := s.pool.Exec(ctx,
		`UPDATE bunshin_agents
		    SET status = 'active'
		  WHERE tenant_id = $1 AND name = $2 AND version = $3`,
		tenantID, name, version,
	)
	if err != nil {
		return fmt.Errorf("agent/postgres: activate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("agent %q version %q tenant %q: activate failed", name, version, tenantID)
	}
	return nil
}

// Delete soft-deletes all versions for tenantID/name.
func (s *PostgresStore) Delete(ctx context.Context, tenantID, name string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE bunshin_agents SET status = 'deleted' WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("agent/postgres: delete %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}
	return nil
}

func decodeAgentSpec(data []byte) (*agent.AgentSpec, error) {
	var spec agent.AgentSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("agent/postgres: decode yaml: %w", err)
	}
	return &spec, nil
}

