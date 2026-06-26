// Package hash provides a shared content-hash function used by spec versioning.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
)

// Bytes returns "sha256:" + the first 32 hex characters of the SHA-256 digest.
func Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])[:32]
}
