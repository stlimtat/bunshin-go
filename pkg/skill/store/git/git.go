// Package git provides a go-git-backed implementation of skill.Store.
//
// All specs are stored on a single commit branch (default: "bunshin-skills").
// Each commit adds or modifies files under a path-based layout:
//
//	{tenantID}/{name}/{version}.json  — spec content
//	{tenantID}/{name}/_active         — active version pointer (plain text)
//
// The full git history is the audit trail. The _active file is the live pointer.
//
// # Single-writer contract
//
// The Store holds an in-process mutex to serialise writes within one process.
// Multi-process concurrent writers need an external lock (e.g. Redis or Postgres
// advisory lock) to prevent lost-update races on the working tree.
//
// # Repository initialisation
//
// Pass an already-initialised *git.Repository (via PlainOpen, PlainInit, or
// memory.New). The store does not call PlainInit itself.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/stlimtat/bunshin-go/pkg/skill"
)

const defaultBranch = "bunshin-skills"

// committerSig is the git author/committer identity for all store commits.
var committerSig = &object.Signature{
	Name:  "bunshin-go",
	Email: "bunshin@noreply",
}

// newSignature returns a committer signature stamped with the current time.
// go-git requires a non-zero When for a valid commit.
func newSignature() *object.Signature {
	return &object.Signature{
		Name:  committerSig.Name,
		Email: committerSig.Email,
		When:  time.Now(),
	}
}

// Store is a go-git-backed skill.Store.
// Not safe for concurrent writes from multiple processes without an external lock.
type Store struct {
	repo   *gogit.Repository
	branch string
	mu     sync.Mutex
}

// New returns a Store backed by repo on the given branch.
// If branch is empty, "bunshin-skills" is used.
func New(repo *gogit.Repository, branch string) *Store {
	if branch == "" {
		branch = defaultBranch
	}
	return &Store{repo: repo, branch: branch}
}

func (s *Store) refName() plumbing.ReferenceName {
	return plumbing.NewBranchReferenceName(s.branch)
}

func specPath(tenantID, name, version string) string {
	return tenantID + "/" + name + "/" + version + ".json"
}

func activePath(tenantID, name string) string {
	return tenantID + "/" + name + "/_active"
}

// Create persists spec as a new draft commit. Idempotent by content-hash version.
func (s *Store) Create(_ context.Context, tenantID string, spec *skill.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("skill/git: Create: spec is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("skill/git: marshal: %w", err)
	}

	path := specPath(tenantID, spec.Name, spec.Version)

	wt, err := s.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("skill/git: worktree: %w", err)
	}

	// Check if the file already exists to skip the commit.
	_, err = wt.Filesystem.Stat(path)
	if err == nil {
		return spec.Version, nil // Already committed.
	}

	if err := s.checkoutBranch(wt); err != nil {
		return "", fmt.Errorf("skill/git: checkout: %w", err)
	}

	// Write spec file.
	f, err := wt.Filesystem.Create(path)
	if err != nil {
		return "", fmt.Errorf("skill/git: create file: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return "", fmt.Errorf("skill/git: write file: %w", err)
	}
	f.Close()

	if _, err := wt.Add(path); err != nil {
		return "", fmt.Errorf("skill/git: add: %w", err)
	}

	hash, err := wt.Commit(
		fmt.Sprintf("skill: create %s version %s", spec.Name, spec.Version),
		&gogit.CommitOptions{Author: newSignature()},
	)
	if err != nil {
		return "", fmt.Errorf("skill/git: commit: %w", err)
	}

	if err := s.updateRef(wt, hash); err != nil {
		return "", fmt.Errorf("skill/git: update ref: %w", err)
	}
	return spec.Version, nil
}

// Get returns the active version, or skill.ErrNotFound if none.
func (s *Store) Get(_ context.Context, tenantID, name string) (*skill.Spec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	activeVersion, err := s.readActivePointer(tenantID, name)
	if err != nil {
		return nil, err
	}
	return s.getVersionLocked(tenantID, name, activeVersion)
}

// GetVersion returns the specific version, or skill.ErrNotFound if absent.
func (s *Store) GetVersion(_ context.Context, tenantID, name, version string) (*skill.Spec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getVersionLocked(tenantID, name, version)
}

// getVersionLocked retrieves a spec without acquiring the mutex (must be held by caller).
func (s *Store) getVersionLocked(tenantID, name, version string) (*skill.Spec, error) {
	path := specPath(tenantID, name, version)
	wt, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("skill/git: worktree: %w", err)
	}

	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrNotFound)
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("skill/git: read: %w", err)
	}

	var spec skill.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("skill/git: unmarshal: %w", err)
	}
	spec.Version = version
	spec.Status = skill.StatusDraft

	// If this is the active version, mark it as such.
	activeVer, err := s.readActivePointer(tenantID, name)
	if err == nil && activeVer == version {
		spec.Status = skill.StatusActive
	}
	return &spec, nil
}

// List returns non-deleted skill names for the tenant.
func (s *Store) List(_ context.Context, tenantID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("skill/git: worktree: %w", err)
	}

	skillNames := make(map[string]struct{})

	entries, err := wt.Filesystem.ReadDir(tenantID)
	if err != nil {
		return nil, nil // Tenant not found.
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillNames[entry.Name()] = struct{}{}
		}
	}

	result := make([]string, 0, len(skillNames))
	for name := range skillNames {
		result = append(result, name)
	}
	return result, nil
}

// ListVersions returns all version strings for name in insertion order (oldest first).
func (s *Store) ListVersions(_ context.Context, tenantID, name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("skill/git: worktree: %w", err)
	}

	dir := tenantID + "/" + name
	entries, err := wt.Filesystem.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}

	versions := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") && !strings.HasPrefix(entry.Name(), "_") {
			version := strings.TrimSuffix(entry.Name(), ".json")
			versions = append(versions, version)
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	return versions, nil
}

// Activate promotes version to active. Returns skill.ErrVersionConflict if absent.
func (s *Store) Activate(_ context.Context, tenantID, name, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("skill/git: worktree: %w", err)
	}

	// Verify the target version exists.
	path := specPath(tenantID, name, version)
	_, err = wt.Filesystem.Stat(path)
	if err != nil {
		return fmt.Errorf("skill %q version %q: %w", name, version, skill.ErrVersionConflict)
	}

	if err := s.checkoutBranch(wt); err != nil {
		return fmt.Errorf("skill/git: checkout: %w", err)
	}

	// Write active pointer.
	activePath := activePath(tenantID, name)
	f, err := wt.Filesystem.Create(activePath)
	if err != nil {
		return fmt.Errorf("skill/git: create active pointer: %w", err)
	}
	if _, err := f.Write([]byte(version)); err != nil {
		f.Close()
		return fmt.Errorf("skill/git: write active pointer: %w", err)
	}
	f.Close()

	if _, err := wt.Add(activePath); err != nil {
		return fmt.Errorf("skill/git: add active: %w", err)
	}

	hash, err := wt.Commit(
		fmt.Sprintf("skill: activate %s %s", name, version),
		&gogit.CommitOptions{Author: newSignature()},
	)
	if err != nil {
		return fmt.Errorf("skill/git: commit active: %w", err)
	}

	return s.updateRef(wt, hash)
}

// Delete soft-deletes the skill by removing the active pointer.
func (s *Store) Delete(_ context.Context, tenantID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("skill/git: worktree: %w", err)
	}

	if err := s.checkoutBranch(wt); err != nil {
		return fmt.Errorf("skill/git: checkout: %w", err)
	}

	activePath := activePath(tenantID, name)
	if err := wt.Filesystem.Remove(activePath); err != nil {
		return fmt.Errorf("skill/git: remove active: %w", err)
	}

	if _, err := wt.Remove(activePath); err != nil {
		return fmt.Errorf("skill/git: remove from index: %w", err)
	}

	hash, err := wt.Commit(
		fmt.Sprintf("skill: delete %s", name),
		&gogit.CommitOptions{Author: newSignature()},
	)
	if err != nil {
		return fmt.Errorf("skill/git: commit delete: %w", err)
	}

	return s.updateRef(wt, hash)
}

// checkoutBranch ensures the worktree is on the target branch, creating it if needed.
// On an empty repository (no HEAD yet), it first writes an initial empty commit so
// the branch can be created.
func (s *Store) checkoutBranch(wt *gogit.Worktree) error {
	refName := s.refName()

	// Fast path: branch already exists.
	if _, refErr := s.repo.Reference(refName, true); refErr == nil {
		return wt.Checkout(&gogit.CheckoutOptions{Branch: refName})
	}

	// Branch doesn't exist. If the repo has no commits yet, seed an empty commit
	// so HEAD exists and the branch can be created.
	if _, headErr := s.repo.Head(); headErr != nil {
		if _, initErr := wt.Commit("init: bunshin-go skill store", &gogit.CommitOptions{
			Author:            newSignature(),
			AllowEmptyCommits: true,
		}); initErr != nil {
			return fmt.Errorf("skill/git: init commit: %w", initErr)
		}
	}

	// Create the branch from the current HEAD.
	return wt.Checkout(&gogit.CheckoutOptions{Branch: refName, Create: true})
}

// updateRef updates the branch reference to point to the new commit.
func (s *Store) updateRef(wt *gogit.Worktree, hash plumbing.Hash) error {
	refName := s.refName()
	ref := plumbing.NewHashReference(refName, hash)
	return s.repo.Storer.SetReference(ref)
}

// readActivePointer reads the active version string from the _active file.
// Must be called with the mutex held.
func (s *Store) readActivePointer(tenantID, name string) (string, error) {
	path := activePath(tenantID, name)
	wt, err := s.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("skill/git: worktree: %w", err)
	}

	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return "", fmt.Errorf("skill %q: %w", name, skill.ErrNotFound)
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("skill/git: read active: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
