package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrVersionConflict is returned when an optimistic-lock CAS fails because
// another node has already written a newer version.
var ErrVersionConflict = errors.New("checkpoint version conflict")

// PostgresCheckpointer stores checkpoints in PostgreSQL with optimistic locking.
//
// Optimistic locking: each row carries a version counter. Save increments the
// version atomically using UPDATE … WHERE version = N. If the row has been
// updated by another node since it was loaded, the UPDATE matches zero rows
// and ErrVersionConflict is returned — the caller should reload and retry.
//
// Schema (create once per database):
//
//	CREATE TABLE IF NOT EXISTS bunshin_checkpoints (
//	    id          BIGSERIAL PRIMARY KEY,
//	    workflow_id TEXT             NOT NULL,
//	    thread_id   TEXT             NOT NULL,
//	    step_index  INT              NOT NULL,
//	    step_id     TEXT             NOT NULL,
//	    state       JSONB            NOT NULL,
//	    version     INT              NOT NULL DEFAULT 0,
//	    created_at  TIMESTAMPTZ      DEFAULT NOW()
//	);
//	CREATE UNIQUE INDEX IF NOT EXISTS bunshin_checkpoints_thread
//	    ON bunshin_checkpoints (thread_id);
type PostgresCheckpointer struct {
	pool *pgxpool.Pool
}

// NewPostgresCheckpointer constructs a checkpointer backed by pool.
func NewPostgresCheckpointer(pool *pgxpool.Pool) *PostgresCheckpointer {
	return &PostgresCheckpointer{pool: pool}
}

const pgCPUpsert = `
INSERT INTO bunshin_checkpoints
    (workflow_id, thread_id, step_index, step_id, state, version, created_at)
VALUES ($1, $2, $3, $4, $5, 1, $6)
ON CONFLICT (thread_id) DO UPDATE
SET workflow_id = EXCLUDED.workflow_id,
    step_index  = EXCLUDED.step_index,
    step_id     = EXCLUDED.step_id,
    state       = EXCLUDED.state,
    created_at  = EXCLUDED.created_at,
    version     = bunshin_checkpoints.version + 1
WHERE bunshin_checkpoints.version = $7`

// Save persists cp with optimistic locking. Pass cp.Version=0 for initial insert.
// On a version conflict, ErrVersionConflict is returned; reload and retry.
func (p *PostgresCheckpointer) Save(ctx context.Context, cp *Checkpoint) error {
	tag, err := p.pool.Exec(ctx, pgCPUpsert,
		cp.WorkflowID, cp.ThreadID, cp.StepIndex, cp.StepID,
		cp.State, cp.CreatedAt, cp.Version)
	if err != nil {
		return fmt.Errorf("postgres checkpointer: save: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: thread=%q expected version %d", ErrVersionConflict, cp.ThreadID, cp.Version)
	}
	return nil
}

const pgCPLoad = `
SELECT workflow_id, thread_id, step_index, step_id, state, version, created_at
FROM bunshin_checkpoints WHERE thread_id = $1`

// Load retrieves the most recent checkpoint for threadID.
func (p *PostgresCheckpointer) Load(ctx context.Context, threadID string) (*Checkpoint, error) {
	var cp Checkpoint
	err := p.pool.QueryRow(ctx, pgCPLoad, threadID).Scan(
		&cp.WorkflowID, &cp.ThreadID, &cp.StepIndex, &cp.StepID,
		&cp.State, &cp.Version, &cp.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: thread=%q", ErrNoCheckpoint, threadID)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres checkpointer: load: %w", err)
	}
	return &cp, nil
}

const pgCPDelete = `DELETE FROM bunshin_checkpoints WHERE thread_id = $1`

// Delete removes all checkpoints for threadID.
func (p *PostgresCheckpointer) Delete(ctx context.Context, threadID string) error {
	_, err := p.pool.Exec(ctx, pgCPDelete, threadID)
	if err != nil {
		return fmt.Errorf("postgres checkpointer: delete: %w", err)
	}
	return nil
}

// History returns all saved checkpoints for threadID, oldest first.
// With the current schema (one row per thread_id) this returns at most one entry.
// To store full history, remove the UNIQUE INDEX and remove ON CONFLICT DO UPDATE.
func (p *PostgresCheckpointer) History(ctx context.Context, threadID string) ([]*Checkpoint, error) {
	cp, err := p.Load(ctx, threadID)
	if errors.Is(err, ErrNoCheckpoint) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []*Checkpoint{cp}, nil
}

// PostgresJournal persists step journal entries in PostgreSQL.
//
// Schema (create once per database):
//
//	CREATE TABLE IF NOT EXISTS bunshin_journal (
//	    id          BIGSERIAL PRIMARY KEY,
//	    workflow_id TEXT          NOT NULL,
//	    thread_id   TEXT          NOT NULL,
//	    step_index  INT           NOT NULL,
//	    step_id     TEXT          NOT NULL,
//	    inputs      JSONB,
//	    outputs     JSONB,
//	    error       TEXT,
//	    start_time  TIMESTAMPTZ,
//	    end_time    TIMESTAMPTZ
//	);
//	CREATE INDEX IF NOT EXISTS bunshin_journal_thread
//	    ON bunshin_journal (thread_id, step_index);
type PostgresJournal struct {
	pool *pgxpool.Pool
}

// NewPostgresJournal constructs a journal backed by pool.
func NewPostgresJournal(pool *pgxpool.Pool) *PostgresJournal {
	return &PostgresJournal{pool: pool}
}

const pgJInsert = `
INSERT INTO bunshin_journal
    (workflow_id, thread_id, step_index, step_id, inputs, outputs, error, start_time, end_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

// Append writes a journal entry.
func (j *PostgresJournal) Append(ctx context.Context, e *JournalEntry) error {
	_, err := j.pool.Exec(ctx, pgJInsert,
		e.WorkflowID, e.ThreadID, e.StepIndex, e.StepID,
		mapToJSON(e.Inputs), mapToJSON(e.Outputs),
		e.Err, e.StartTime, e.EndTime)
	if err != nil {
		return fmt.Errorf("postgres journal: append: %w", err)
	}
	return nil
}

const pgJSelect = `
SELECT workflow_id, thread_id, step_index, step_id, inputs, outputs, error, start_time, end_time
FROM bunshin_journal WHERE thread_id = $1 ORDER BY step_index, id`

// Entries returns all journal entries for threadID in execution order.
func (j *PostgresJournal) Entries(ctx context.Context, threadID string) ([]*JournalEntry, error) {
	rows, err := j.pool.Query(ctx, pgJSelect, threadID)
	if err != nil {
		return nil, fmt.Errorf("postgres journal: query: %w", err)
	}
	defer rows.Close()

	var out []*JournalEntry
	for rows.Next() {
		var e JournalEntry
		var inputsJSON, outputsJSON []byte
		var startTime, endTime *time.Time
		if err := rows.Scan(
			&e.WorkflowID, &e.ThreadID, &e.StepIndex, &e.StepID,
			&inputsJSON, &outputsJSON, &e.Err, &startTime, &endTime,
		); err != nil {
			return nil, fmt.Errorf("postgres journal: scan: %w", err)
		}
		if startTime != nil {
			e.StartTime = *startTime
		}
		if endTime != nil {
			e.EndTime = *endTime
		}
		_ = jsonToMap(inputsJSON, &e.Inputs)
		_ = jsonToMap(outputsJSON, &e.Outputs)
		out = append(out, &e)
	}
	return out, rows.Err()
}

const pgJClear = `DELETE FROM bunshin_journal WHERE thread_id = $1`

// Clear removes all journal entries for threadID.
func (j *PostgresJournal) Clear(ctx context.Context, threadID string) error {
	_, err := j.pool.Exec(ctx, pgJClear, threadID)
	if err != nil {
		return fmt.Errorf("postgres journal: clear: %w", err)
	}
	return nil
}
