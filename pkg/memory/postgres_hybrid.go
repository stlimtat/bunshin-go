package memory

import (
	"bytes"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// PostgresHybridMessageStore stores text messages inline in Postgres and routes
// media-bearing messages to MinIO.
//
// Inline rows store the full JSONB payload. Media rows store a sentinel JSONB
// with {"minio_key":"<key>"} so the reader can fetch the blob on demand.
// This keeps the Postgres table small while supporting arbitrarily large images
// or documents in the conversation history.
type PostgresHybridMessageStore struct {
	pool         *pgxpool.Pool
	minioClient  *minio.Client
	minioBucket  string
	threadID     string
	tenantID     string
	tokenCounter TokenCounter
}

// NewPostgresHybridMessageStore constructs a hybrid store.
// minioClient and minioBucket are used for media-bearing messages.
func NewPostgresHybridMessageStore(
	pool *pgxpool.Pool,
	minioClient *minio.Client, minioBucket string,
	threadID, tenantID string,
	opts ...MemoryStoreOption,
) *PostgresHybridMessageStore {
	dummy := &MemoryStore{tokenCounter: defaultTokenCount}
	for _, o := range opts {
		o(dummy)
	}
	return &PostgresHybridMessageStore{
		pool:         pool,
		minioClient:  minioClient,
		minioBucket:  minioBucket,
		threadID:     threadID,
		tenantID:     tenantID,
		tokenCounter: dummy.tokenCounter,
	}
}

// hasMedia reports whether msg contains any non-text parts.
func hasMedia(msg llm.Message) bool {
	for _, p := range msg.Parts {
		if p.Type != llm.PartTypeText && p.Type != llm.PartTypeToolCall && p.Type != llm.PartTypeToolResult {
			return true
		}
	}
	return false
}

// Append stores the message. Media messages go to MinIO; others are inline.
func (s *PostgresHybridMessageStore) Append(ctx context.Context, msg llm.Message) error {
	var payload []byte
	var err error

	if hasMedia(msg) {
		data, err := marshal(msg)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%s/media/%d.json", s.tenantID, s.threadID, minioKey())
		if _, err = s.minioClient.PutObject(ctx, s.minioBucket, key,
			bytes.NewReader(data), int64(len(data)),
			minio.PutObjectOptions{ContentType: "application/json"}); err != nil {
			return fmt.Errorf("hybrid store: minio put: %w", err)
		}
		payload = []byte(fmt.Sprintf(`{"minio_key":%q}`, key))
	} else {
		payload, err = marshal(msg)
		if err != nil {
			return err
		}
	}

	_, err = s.pool.Exec(ctx, pgInsert, s.threadID, s.tenantID, string(msg.Role), payload)
	if err != nil {
		return fmt.Errorf("hybrid store: pg insert: %w", err)
	}
	return nil
}

// Window returns up to maxTokens tokens of the most recent messages.
func (s *PostgresHybridMessageStore) Window(ctx context.Context, maxTokens int) ([]llm.Message, error) {
	rows, err := s.pool.Query(ctx, pgSelect, s.tenantID, s.threadID)
	if err != nil {
		return nil, fmt.Errorf("hybrid store: query: %w", err)
	}
	defer rows.Close()

	var all []llm.Message
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("hybrid store: scan: %w", err)
		}
		// Detect MinIO pointer rows.
		var sentinel struct{ MinioKey string `json:"minio_key"` }
		if err := unmarshalSentinel(data, &sentinel); err == nil && sentinel.MinioKey != "" {
			data, err = s.fetchMinIO(ctx, sentinel.MinioKey)
			if err != nil {
				return nil, err
			}
		}
		msg, err := unmarshal(data)
		if err != nil {
			return nil, err
		}
		all = append(all, msg)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("hybrid store: rows: %w", rows.Err())
	}
	return applyWindow(all, maxTokens, s.tokenCounter), nil
}

// WindowFor returns a provider-native Request.
func (s *PostgresHybridMessageStore) WindowFor(ctx context.Context, p llm.LLMProvider, maxTokens int) (*llm.Request, error) {
	msgs, err := s.Window(ctx, maxTokens)
	if err != nil {
		return nil, err
	}
	return &llm.Request{Messages: msgs}, nil
}

// Len returns the total number of messages.
func (s *PostgresHybridMessageStore) Len() int {
	var n int
	_ = s.pool.QueryRow(context.Background(), pgCount, s.tenantID, s.threadID).Scan(&n)
	return n
}

func (s *PostgresHybridMessageStore) Snapshot(_ context.Context) error { return nil }
func (s *PostgresHybridMessageStore) Restore(_ context.Context) error  { return nil }

func (s *PostgresHybridMessageStore) fetchMinIO(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.minioClient.GetObject(ctx, s.minioBucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("hybrid store: minio get %q: %w", key, err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(obj); err != nil {
		return nil, fmt.Errorf("hybrid store: minio read %q: %w", key, err)
	}
	_ = obj.Close()
	return buf.Bytes(), nil
}
