// Package blob provides a gocloud.dev/blob-backed implementation of workflow.Store.
//
// Specs are stored as JSON objects under the prefix:
//
//	{prefix}workflows/{tenantID}/{name}/{version}.json  — spec content
//	{prefix}workflows/{tenantID}/{name}/_active          — active version string
//	{prefix}workflows/{tenantID}/{name}/_deleted         — soft-delete marker
//	{prefix}workflows/{tenantID}/{name}/_versions        — newline-delimited insertion order
//
// The store is safe for multiple readers but NOT safe for concurrent writers
// targeting the same (tenantID, name) — it uses optimistic read-modify-write
// on the _active and _versions objects. Use the Postgres store for multi-writer
// deployments. Blob storage is ideal for single-writer CI/CD pipelines or
// GitOps deployments.
//
// # Backend selection
//
// Open a *blob.Bucket with any gocloud.dev driver:
//
//	import _ "gocloud.dev/blob/s3blob"     // AWS S3
//	import _ "gocloud.dev/blob/gcsblob"    // Google Cloud Storage
//	import _ "gocloud.dev/blob/azureblob"  // Azure Blob Storage
//	import _ "gocloud.dev/blob/fileblob"   // local filesystem (dev/test)
//
//	bucket, _ := blob.OpenBucket(ctx, "s3://my-bucket?region=us-east-1")
//	store := blobstore.New(bucket, "bunshin/")
package blob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gocloud.dev/blob"
	goblob "gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// Store is a gocloud.dev/blob-backed implementation of workflow.Store.
// Not safe for concurrent writes to the same (tenantID, name) pair.
type Store struct {
	bucket *goblob.Bucket
	prefix string // e.g. "bunshin/" — must end with "/" or be ""
}

// New returns a Store backed by bucket under the given key prefix.
// prefix must end with "/" or be empty.
func New(bucket *blob.Bucket, prefix string) *Store {
	return &Store{bucket: bucket, prefix: prefix}
}

func (s *Store) key(tenantID, name, file string) string {
	return s.prefix + "workflows/" + tenantID + "/" + name + "/" + file
}

// Create persists spec as a new draft. Idempotent by content-hash version.
// Resurrects soft-deleted workflows (removes _deleted marker).
func (s *Store) Create(ctx context.Context, tenantID string, spec *workflow.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("workflow/blob: Create: spec is nil")
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("workflow/blob: marshal spec: %w", err)
	}

	versionKey := s.key(tenantID, spec.Name, spec.Version+".json")
	deletedKey := s.key(tenantID, spec.Name, "_deleted")
	versionsKey := s.key(tenantID, spec.Name, "_versions")

	// Idempotent: skip write if version already exists.
	if exists, _ := s.bucket.Exists(ctx, versionKey); !exists {
		if err := s.bucket.WriteAll(ctx, versionKey, raw, nil); err != nil {
			return "", fmt.Errorf("workflow/blob: write spec: %w", err)
		}
		// Append version to insertion-order log.
		if err := s.appendVersion(ctx, versionsKey, spec.Version); err != nil {
			return "", fmt.Errorf("workflow/blob: update versions log: %w", err)
		}
	}

	// Resurrect soft-deleted workflow.
	if exists, _ := s.bucket.Exists(ctx, deletedKey); exists {
		if err := s.bucket.Delete(ctx, deletedKey); err != nil {
			return "", fmt.Errorf("workflow/blob: remove deleted marker: %w", err)
		}
	}

	return spec.Version, nil
}

// Get returns the active version, or workflow.ErrNotFound if none.
func (s *Store) Get(ctx context.Context, tenantID, name string) (*workflow.Spec, error) {
	if err := s.checkNotDeleted(ctx, tenantID, name); err != nil {
		return nil, err
	}
	activeVersion, err := s.readActive(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}
	return s.readSpec(ctx, tenantID, name, activeVersion, workflow.StatusActive)
}

// GetVersion returns the specific version, or workflow.ErrNotFound.
func (s *Store) GetVersion(ctx context.Context, tenantID, name, version string) (*workflow.Spec, error) {
	return s.readSpec(ctx, tenantID, name, version, workflow.StatusDraft)
}

// List returns names of non-deleted workflows for tenantID.
func (s *Store) List(ctx context.Context, tenantID string) ([]string, error) {
	prefix := s.prefix + "workflows/" + tenantID + "/"
	iter := s.bucket.List(&goblob.ListOptions{
		Prefix:    prefix,
		Delimiter: "/",
	})
	var names []string
	for {
		obj, err := iter.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("workflow/blob: List: %w", err)
		}
		if !obj.IsDir {
			continue
		}
		name := strings.TrimPrefix(obj.Key, prefix)
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}
		// Skip soft-deleted workflows.
		if exists, _ := s.bucket.Exists(ctx, s.key(tenantID, name, "_deleted")); exists {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// ListVersions returns all version strings for name in insertion order (oldest first).
func (s *Store) ListVersions(ctx context.Context, tenantID, name string) ([]string, error) {
	if err := s.checkNotDeleted(ctx, tenantID, name); err != nil {
		return nil, err
	}
	return s.readVersions(ctx, tenantID, name)
}

// Activate promotes version to active. Returns ErrVersionConflict if absent.
func (s *Store) Activate(ctx context.Context, tenantID, name, version string) error {
	if err := s.checkNotDeleted(ctx, tenantID, name); err != nil {
		return workflow.ErrVersionConflict
	}
	versionKey := s.key(tenantID, name, version+".json")
	if exists, _ := s.bucket.Exists(ctx, versionKey); !exists {
		return fmt.Errorf("workflow %q version %q: %w", name, version, workflow.ErrVersionConflict)
	}
	activeKey := s.key(tenantID, name, "_active")
	if err := s.bucket.WriteAll(ctx, activeKey, []byte(version), nil); err != nil {
		return fmt.Errorf("workflow/blob: write active pointer: %w", err)
	}
	return nil
}

// Delete soft-deletes the workflow. Get returns ErrNotFound afterwards.
func (s *Store) Delete(ctx context.Context, tenantID, name string) error {
	if err := s.checkNotDeleted(ctx, tenantID, name); err != nil {
		return err
	}
	deletedKey := s.key(tenantID, name, "_deleted")
	if err := s.bucket.WriteAll(ctx, deletedKey, []byte("deleted"), nil); err != nil {
		return fmt.Errorf("workflow/blob: write deleted marker: %w", err)
	}
	return nil
}

// ---- helpers ----

func (s *Store) checkNotDeleted(ctx context.Context, tenantID, name string) error {
	deletedKey := s.key(tenantID, name, "_deleted")
	if exists, _ := s.bucket.Exists(ctx, deletedKey); exists {
		return fmt.Errorf("workflow %q: %w", name, workflow.ErrNotFound)
	}
	return nil
}

func (s *Store) readActive(ctx context.Context, tenantID, name string) (string, error) {
	activeKey := s.key(tenantID, name, "_active")
	data, err := s.bucket.ReadAll(ctx, activeKey)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return "", fmt.Errorf("workflow %q: no active version: %w", name, workflow.ErrNotFound)
		}
		return "", fmt.Errorf("workflow/blob: read active: %w", err)
	}
	return string(bytes.TrimSpace(data)), nil
}

func (s *Store) readSpec(ctx context.Context, tenantID, name, version, status string) (*workflow.Spec, error) {
	versionKey := s.key(tenantID, name, version+".json")
	data, err := s.bucket.ReadAll(ctx, versionKey)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return nil, fmt.Errorf("workflow %q version %q: %w", name, version, workflow.ErrNotFound)
		}
		return nil, fmt.Errorf("workflow/blob: read spec: %w", err)
	}
	var spec workflow.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("workflow/blob: unmarshal: %w", err)
	}
	spec.Version = version
	spec.Status = status
	return &spec, nil
}

func (s *Store) readVersions(ctx context.Context, tenantID, name string) ([]string, error) {
	versionsKey := s.key(tenantID, name, "_versions")
	data, err := s.bucket.ReadAll(ctx, versionsKey)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("workflow/blob: read versions: %w", err)
	}
	var vers []string
	for _, line := range strings.Split(string(data), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			vers = append(vers, v)
		}
	}
	return vers, nil
}

func (s *Store) appendVersion(ctx context.Context, versionsKey, version string) error {
	existing, err := s.bucket.ReadAll(ctx, versionsKey)
	if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		return err
	}
	updated := string(existing) + version + "\n"
	return s.bucket.WriteAll(ctx, versionsKey, []byte(updated), nil)
}
