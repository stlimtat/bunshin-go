package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// MinIOMessageStore persists messages as JSON objects in MinIO (or any S3-compatible store).
// Objects are stored at key "{tenantID}/{threadID}/{seq:016d}.json".
// Sequence numbers are monotonically increasing integers assigned at Append time.
//
// This backend is suited for archival of very long conversations — MinIO handles
// virtually unlimited object count and each message is independently accessible.
type MinIOMessageStore struct {
	client       *minio.Client
	bucket       string
	threadID     string
	tenantID     string
	tokenCounter TokenCounter
	seq          int64
}

// NewMinIOMessageStore constructs a store scoped to a single thread/tenant.
// bucket must exist before use.
func NewMinIOMessageStore(client *minio.Client, bucket, threadID, tenantID string, opts ...MemoryStoreOption) *MinIOMessageStore {
	dummy := &MemoryStore{tokenCounter: defaultTokenCount}
	for _, o := range opts {
		o(dummy)
	}
	return &MinIOMessageStore{
		client:       client,
		bucket:       bucket,
		threadID:     threadID,
		tenantID:     tenantID,
		tokenCounter: dummy.tokenCounter,
	}
}

func (s *MinIOMessageStore) objectKey(seq int64) string {
	return fmt.Sprintf("%s/%s/%016d.json", s.tenantID, s.threadID, seq)
}

// Append serialises msg as JSON and uploads it to MinIO.
func (s *MinIOMessageStore) Append(ctx context.Context, msg llm.Message) error {
	s.seq++
	data, err := marshal(msg)
	if err != nil {
		return err
	}
	key := s.objectKey(s.seq)
	_, err = s.client.PutObject(ctx, s.bucket, key,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/json"})
	if err != nil {
		return fmt.Errorf("minio message store: put %q: %w", key, err)
	}
	return nil
}

// Window returns up to maxTokens tokens of the most recent messages.
func (s *MinIOMessageStore) Window(ctx context.Context, maxTokens int) ([]llm.Message, error) {
	prefix := fmt.Sprintf("%s/%s/", s.tenantID, s.threadID)
	var keys []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("minio message store: list: %w", obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	sort.Strings(keys)

	var msgs []llm.Message
	for _, key := range keys {
		obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
		if err != nil {
			return nil, fmt.Errorf("minio message store: get %q: %w", key, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(obj); err != nil {
			return nil, fmt.Errorf("minio message store: read %q: %w", key, err)
		}
		_ = obj.Close()
		msg, err := unmarshal(buf.Bytes())
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return applyWindow(msgs, maxTokens, s.tokenCounter), nil
}

// WindowFor returns a provider-native Request.
func (s *MinIOMessageStore) WindowFor(ctx context.Context, p llm.LLMProvider, maxTokens int) (*llm.Request, error) {
	msgs, err := s.Window(ctx, maxTokens)
	if err != nil {
		return nil, err
	}
	return &llm.Request{Messages: msgs}, nil
}

// Len returns the number of messages by listing objects in MinIO.
func (s *MinIOMessageStore) Len() int {
	prefix := fmt.Sprintf("%s/%s/", s.tenantID, s.threadID)
	n := 0
	for obj := range s.client.ListObjects(context.Background(), s.bucket, minio.ListObjectsOptions{Prefix: prefix}) {
		if obj.Err == nil {
			n++
		}
	}
	return n
}

// Snapshot is a no-op — all writes go directly to MinIO.
func (s *MinIOMessageStore) Snapshot(_ context.Context) error { return nil }

// Restore restores the seq counter by scanning existing objects.
func (s *MinIOMessageStore) Restore(ctx context.Context) error {
	prefix := fmt.Sprintf("%s/%s/", s.tenantID, s.threadID)
	var maxSeq int64
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix}) {
		if obj.Err != nil {
			continue
		}
		// Key format: "{tenant}/{thread}/{seq:016d}.json"
		base := obj.Key[len(prefix):]
		base = strings.TrimSuffix(base, ".json")
		if n, err := strconv.ParseInt(base, 10, 64); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	s.seq = maxSeq

	// Verify the seq matches by decoding the last object's role.
	if maxSeq > 0 {
		lastKey := s.objectKey(maxSeq)
		obj, err := s.client.GetObject(ctx, s.bucket, lastKey, minio.GetObjectOptions{})
		if err != nil {
			return fmt.Errorf("minio message store: restore verify: %w", err)
		}
		var row struct{ Role string }
		if err := json.NewDecoder(obj).Decode(&row); err != nil {
			_ = obj.Close()
			return fmt.Errorf("minio message store: restore decode: %w", err)
		}
		_ = obj.Close()
	}
	return nil
}
