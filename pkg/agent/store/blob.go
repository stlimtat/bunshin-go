package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/agent"
	"gocloud.dev/blob"
	"gopkg.in/yaml.v3"
)

// BlobStore is a Store backed by gocloud.dev/blob, supporting cloud storage
// backends (S3, GCS, Azure Blob, local filesystem, etc.).
//
// File layout:
//   {tenantID}/{name}/{version}.yaml     — agent spec in YAML format
//   {tenantID}/{name}/active.txt         — contains the version string of the active spec
//
// Versions are discovered by listing all YAML files for {tenantID}/{name}/
type BlobStore struct {
	bucket *blob.Bucket
}

// NewBlobStore constructs a store backed by bucket.
// bucket is typically created via blob.OpenBucket(ctx, "s3://...") or similar.
func NewBlobStore(bucket *blob.Bucket) *BlobStore {
	return &BlobStore{bucket: bucket}
}

// Create persists spec as a new draft. Idempotent: same content = same version.
func (s *BlobStore) Create(ctx context.Context, tenantID string, spec *agent.AgentSpec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("blob.Store.Create: spec is nil")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("blob.Store.Create: spec.Name must not be empty")
	}

	version, err := contentHashYAML(spec)
	if err != nil {
		return "", fmt.Errorf("blob.Store: hash: %w", err)
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("blob.Store: marshal: %w", err)
	}

	key := fmt.Sprintf("%s/%s/%s.yaml", tenantID, spec.Name, version)
	w, err := s.bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return "", fmt.Errorf("blob.Store: open writer %q: %w", key, err)
	}

	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("blob.Store: write %q: %w", key, err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("blob.Store: close %q: %w", key, err)
	}

	return version, nil
}

// Get returns the active version by name.
func (s *BlobStore) Get(ctx context.Context, tenantID, name string) (*agent.AgentSpec, error) {
	activeKey := fmt.Sprintf("%s/%s/active.txt", tenantID, name)
	data, err := s.bucket.ReadAll(ctx, activeKey)
	if err != nil {
		return nil, fmt.Errorf("agent %q tenant %q: no active version", name, tenantID)
	}

	version := strings.TrimSpace(string(data))
	return s.GetVersion(ctx, tenantID, name, version)
}

// GetVersion returns a specific version.
func (s *BlobStore) GetVersion(ctx context.Context, tenantID, name, version string) (*agent.AgentSpec, error) {
	key := fmt.Sprintf("%s/%s/%s.yaml", tenantID, name, version)
	data, err := s.bucket.ReadAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
	}

	spec, err := decodeAgentSpec(data)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// List returns all agent names for tenantID with an active.txt marker.
func (s *BlobStore) List(ctx context.Context, tenantID string) ([]string, error) {
	prefix := tenantID + "/"
	iter := s.bucket.List(&blob.ListOptions{Prefix: prefix})

	nameSet := make(map[string]bool)
	for {
		attrs, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("blob.Store: list: %w", err)
		}

		// Extract name from key like "tenantID/name/active.txt" or "tenantID/name/version.yaml"
		parts := strings.Split(attrs.Key, "/")
		if len(parts) >= 3 && (strings.HasSuffix(parts[len(parts)-1], ".yaml") ||
			(strings.HasSuffix(parts[len(parts)-1], ".txt") && parts[len(parts)-1] == "active.txt")) {
			name := parts[1]
			nameSet[name] = true
		}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	return names, nil
}

// ListVersions returns all version metadata for name.
func (s *BlobStore) ListVersions(ctx context.Context, tenantID, name string) ([]AgentVersion, error) {
	prefix := fmt.Sprintf("%s/%s/", tenantID, name)

	// Read active pointer once — avoids an O(n) ReadAll inside the listing loop.
	activeKey := fmt.Sprintf("%s/%s/active.txt", tenantID, name)
	activeVersion := ""
	if data, err := s.bucket.ReadAll(ctx, activeKey); err == nil {
		activeVersion = strings.TrimSpace(string(data))
	}

	iter := s.bucket.List(&blob.ListOptions{Prefix: prefix})
	var versions []AgentVersion
	for {
		attrs, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("blob.Store: list versions: %w", err)
		}

		// Only collect .yaml files (not active.txt)
		if !strings.HasSuffix(attrs.Key, ".yaml") {
			continue
		}

		// Extract version from key like "tenantID/name/sha256:xxxxx.yaml"
		fileName := attrs.Key[len(prefix):]
		version := strings.TrimSuffix(fileName, ".yaml")

		status := "draft"
		if version == activeVersion {
			status = "active"
		}

		versions = append(versions, AgentVersion{
			Version:   version,
			Status:    status,
			CreatedAt: attrs.ModTime,
		})
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}

	// Sort newest-first by modification time
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	return versions, nil
}

// Activate marks version as active for tenantID/name.
func (s *BlobStore) Activate(ctx context.Context, tenantID, name, version string) error {
	// Verify version exists
	versionKey := fmt.Sprintf("%s/%s/%s.yaml", tenantID, name, version)
	_, err := s.bucket.Attributes(ctx, versionKey)
	if err != nil {
		return fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
	}

	// Write active.txt with the version string
	activeKey := fmt.Sprintf("%s/%s/active.txt", tenantID, name)
	w, err := s.bucket.NewWriter(ctx, activeKey, nil)
	if err != nil {
		return fmt.Errorf("blob.Store: open writer %q: %w", activeKey, err)
	}

	if _, err := w.Write([]byte(version)); err != nil {
		_ = w.Close()
		return fmt.Errorf("blob.Store: write %q: %w", activeKey, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("blob.Store: close %q: %w", activeKey, err)
	}

	return nil
}

// Delete removes all versions for tenantID/name (including active.txt).
func (s *BlobStore) Delete(ctx context.Context, tenantID, name string) error {
	// List all versions for this name
	prefix := fmt.Sprintf("%s/%s/", tenantID, name)
	iter := s.bucket.List(&blob.ListOptions{Prefix: prefix})

	foundAny := false
	for {
		attrs, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("blob.Store: list for delete: %w", err)
		}

		foundAny = true
		if err := s.bucket.Delete(ctx, attrs.Key); err != nil {
			return fmt.Errorf("blob.Store: delete %q: %w", attrs.Key, err)
		}
	}

	if !foundAny {
		return fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}

	return nil
}
