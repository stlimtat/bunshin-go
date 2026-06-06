package memory

import (
	"encoding/json"
	"sync/atomic"
)

var globalSeq atomic.Int64

// minioKey returns a monotonically increasing key suffix for MinIO objects.
func minioKey() int64 {
	return globalSeq.Add(1)
}

// unmarshalSentinel attempts to decode a MinIO pointer row without allocating
// a full message struct. Returns an error if data is not valid JSON.
func unmarshalSentinel(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
