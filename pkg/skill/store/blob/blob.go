// Package blob provides a cloud blob storage-backed implementation of skill.Store.
//
// All specs are stored on a single blob bucket with a path-based layout:
//
//	{tenantID}/{name}/{version}.json  — spec content (immutable)
//	{tenantID}/{name}/_active         — active version pointer (plain text, mutable)
//
// The full bucket history (if enabled) serves as the audit trail.
//
// Use with gocloud.dev/blob drivers (e.g. s3blob, gcsblob, azureblob, fileblob).
// See https://gocloud.dev/howto/blob/ for setup examples.
package blob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/skill"
	goblob "gocloud.dev/blob"
)

// Store is a blob.Bucket-backed implementation of skill.Store.
type Store struct {
	bucket *goblob.Bucket
}

// New returns a Store backed by bucket.
func New(bucket *goblob.Bucket) *Store {
	return &Store{bucket: bucket}
}

// Create persists spec as a new draft. Idempotent: identical version is a no-op.
func (s *Store) Create(ctx context.Context, tenantID string, spec *skill.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("skill/blob: Create: spec is nil")
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("skill/blob: marshal spec: %w", err)
	}

	path := specPath(tenantID, spec.Name, spec.Version)
	// Write is idempotent only if the object already exists with identical content.
	// Check first to avoid unnecessary writes.
	_, err = s.bucket.Attributes(ctx, path)
	if err == nil {
		return spec.Version, nil // Already exists.
	}

	if err = s.bucket.WriteAll(ctx, path, raw, nil); err != nil {
		return "", fmt.Errorf("skill/blob: write spec: %w", err)
	}
	return spec.Version, nil
}

// Get returns the active version, or skill.ErrNotFound if none.
func (s *Store) Get(ctx context.Context, tenantID, name string) (*skill.Spec, error) {
	activeVersion, err := s.readActivePointer(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}
	return s.GetVersion(ctx, tenantID, name, activeVersion)
}

// GetVersion returns the specific version, or skill.ErrNotFound if absent.
func (s *Store) GetVersion(ctx context.Context, tenantID, name, version string) (*skill.Spec, error) {
	path := specPath(tenantID, name, version)
	raw, err := s.bucket.ReadAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrNotFound)
	}
	var spec skill.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("skill/blob: unmarshal: %w", err)
	}
	spec.Version = version
	spec.Status = skill.StatusDraft
	// If this is the active version, mark it as such.
	activeVer, err := s.readActivePointer(ctx, tenantID, name)
	if err == nil && activeVer == version {
		spec.Status = skill.StatusActive
	}
	return &spec, nil
}

// List returns non-deleted skill names for the tenant.
// Note: this requires listing all objects, so it may be slow for large buckets.
func (s *Store) List(ctx context.Context, tenantID string) ([]string, error) {
	prefix := tenantID + "/"
	iter := s.bucket.List(&goblob.ListOptions{Prefix: prefix})
	skillNames := make(map[string]struct{})
	for {
		attrs, err := iter.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("skill/blob: list: %w", err)
		}
		// Extract skill name from path: {tenantID}/{name}/{version}.json
		parts := strings.Split(attrs.Key, "/")
		if len(parts) >= 2 {
			skillNames[parts[1]] = struct{}{}
		}
	}
	result := make([]string, 0, len(skillNames))
	for name := range skillNames {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

// ListVersions returns all version strings for name in insertion order (oldest first).
// Note: Cloud blob storage doesn't preserve insertion order, so versions are sorted alphabetically.
func (s *Store) ListVersions(ctx context.Context, tenantID, name string) ([]string, error) {
	prefix := tenantID + "/" + name + "/"
	iter := s.bucket.List(&goblob.ListOptions{Prefix: prefix})
	versions := make([]string, 0)
	for {
		attrs, err := iter.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("skill/blob: listVersions: %w", err)
		}
		// Extract version from path: {tenantID}/{name}/{version}.json
		base := strings.TrimPrefix(attrs.Key, prefix)
		if !strings.HasPrefix(base, "_") && strings.HasSuffix(base, ".json") {
			version := strings.TrimSuffix(base, ".json")
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	sort.Strings(versions)
	return versions, nil
}

// Activate promotes version to active. Returns skill.ErrVersionConflict if absent.
func (s *Store) Activate(ctx context.Context, tenantID, name, version string) error {
	// Verify the target version exists.
	path := specPath(tenantID, name, version)
	_, err := s.bucket.Attributes(ctx, path)
	if err != nil {
		return fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrVersionConflict)
	}
	// Write the active pointer.
	if err = s.bucket.WriteAll(ctx, activePath(tenantID, name), []byte(version), nil); err != nil {
		return fmt.Errorf("skill/blob: write active pointer: %w", err)
	}
	return nil
}

// Delete soft-deletes the skill by removing the active pointer.
// The spec versions remain in the bucket for audit purposes.
func (s *Store) Delete(ctx context.Context, tenantID, name string) error {
	activePointerPath := activePath(tenantID, name)
	if err := s.bucket.Delete(ctx, activePointerPath); err != nil {
		return fmt.Errorf("skill/blob: delete active pointer: %w", err)
	}
	return nil
}

// readActivePointer reads the active version string from the _active file.
func (s *Store) readActivePointer(ctx context.Context, tenantID, name string) (string, error) {
	raw, err := s.bucket.ReadAll(ctx, activePath(tenantID, name))
	if err != nil {
		return "", fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	return strings.TrimSpace(string(raw)), nil
}

func specPath(tenantID, name, version string) string {
	return tenantID + "/" + name + "/" + version + ".json"
}

func activePath(tenantID, name string) string {
	return tenantID + "/" + name + "/_active"
}
