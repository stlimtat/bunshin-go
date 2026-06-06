package memory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// PostgresMessageStore persists messages as JSONB rows in PostgreSQL.
// Tenant isolation is enforced by the tenant_id column — each store instance
// is scoped to a single (thread_id, tenant_id) pair.
//
// Schema (create once per database):
//
//	CREATE TABLE IF NOT EXISTS bunshin_messages (
//	    id          BIGSERIAL PRIMARY KEY,
//	    thread_id   TEXT      NOT NULL,
//	    tenant_id   TEXT      NOT NULL,
//	    role        TEXT      NOT NULL,
//	    content     JSONB     NOT NULL,
//	    created_at  TIMESTAMPTZ DEFAULT NOW()
//	);
//	CREATE INDEX IF NOT EXISTS bunshin_messages_thread
//	    ON bunshin_messages (tenant_id, thread_id, id);
type PostgresMessageStore struct {
	pool         *pgxpool.Pool
	threadID     string
	tenantID     string
	tokenCounter TokenCounter
}

// NewPostgresMessageStore constructs a store scoped to a single thread/tenant.
func NewPostgresMessageStore(pool *pgxpool.Pool, threadID, tenantID string, opts ...MemoryStoreOption) *PostgresMessageStore {
	s := &PostgresMessageStore{
		pool:         pool,
		threadID:     threadID,
		tenantID:     tenantID,
		tokenCounter: defaultTokenCount,
	}
	// Apply only options that target the base fields (token counter).
	dummy := &MemoryStore{tokenCounter: defaultTokenCount}
	for _, o := range opts {
		o(dummy)
	}
	s.tokenCounter = dummy.tokenCounter
	return s
}

const pgInsert = `
INSERT INTO bunshin_messages (thread_id, tenant_id, role, content)
VALUES ($1, $2, $3, $4)`

// Append persists a message to Postgres.
func (s *PostgresMessageStore) Append(ctx context.Context, msg llm.Message) error {
	data, err := marshal(msg)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, pgInsert, s.threadID, s.tenantID, string(msg.Role), data)
	if err != nil {
		return fmt.Errorf("postgres message store: append: %w", err)
	}
	return nil
}

const pgSelect = `
SELECT content FROM bunshin_messages
WHERE tenant_id = $1 AND thread_id = $2
ORDER BY id`

// Window returns up to maxTokens tokens of the most recent messages.
func (s *PostgresMessageStore) Window(ctx context.Context, maxTokens int) ([]llm.Message, error) {
	rows, err := s.pool.Query(ctx, pgSelect, s.tenantID, s.threadID)
	if err != nil {
		return nil, fmt.Errorf("postgres message store: window query: %w", err)
	}
	defer rows.Close()

	var all []llm.Message
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("postgres message store: scan: %w", err)
		}
		msg, err := unmarshal(data)
		if err != nil {
			return nil, err
		}
		all = append(all, msg)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("postgres message store: rows: %w", rows.Err())
	}

	return applyWindow(all, maxTokens, s.tokenCounter), nil
}

// WindowFor returns a provider-native Request.
func (s *PostgresMessageStore) WindowFor(ctx context.Context, p llm.LLMProvider, maxTokens int) (*llm.Request, error) {
	msgs, err := s.Window(ctx, maxTokens)
	if err != nil {
		return nil, err
	}
	return &llm.Request{Messages: msgs}, nil
}

const pgCount = `
SELECT COUNT(*) FROM bunshin_messages
WHERE tenant_id = $1 AND thread_id = $2`

// Len returns the total number of messages in the store.
func (s *PostgresMessageStore) Len() int {
	var n int
	_ = s.pool.QueryRow(context.Background(), pgCount, s.tenantID, s.threadID).Scan(&n)
	return n
}

// Snapshot is a no-op — Postgres writes are durable on Append.
func (s *PostgresMessageStore) Snapshot(_ context.Context) error { return nil }

// Restore is a no-op — messages are always read from Postgres.
func (s *PostgresMessageStore) Restore(_ context.Context) error { return nil }

// applyWindow applies a token-budget window to a slice of messages (newest-last).
func applyWindow(msgs []llm.Message, maxTokens int, counter TokenCounter) []llm.Message {
	if maxTokens <= 0 {
		return msgs
	}
	budget := maxTokens
	start := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		cost := counter(msgs[i])
		if budget-cost < 0 {
			break
		}
		budget -= cost
		start = i
	}
	return msgs[start:]
}
