// Package git provides a go-git-backed implementation of workflow.Store.
//
// All specs are stored on a single commit branch (default: "bunshin-workflows").
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

	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

const defaultBranch = "bunshin-workflows"

// committerSig is the git author/committer identity for all store commits.
var committerSig = &object.Signature{
	Name:  "bunshin-go",
	Email: "bunshin@noreply",
}

// Store is a go-git-backed workflow.Store.
// Not safe for concurrent writes from multiple processes without an external lock.
type Store struct {
	repo   *gogit.Repository
	branch string
	mu     sync.Mutex
}

// New returns a Store backed by repo on the given branch.
// If branch is empty, "bunshin-workflows" is used.
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
func (s *Store) Create(_ context.Context, tenantID string, spec *workflow.Spec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("workflow/git: Create: spec is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("workflow/git: marshal: %w", err)
	}

	wt, err := s.worktree()
	if err != nil {
		return "", err
	}

	filePath := specPath(tenantID, spec.Name, spec.Version)

	// Idempotent: skip if version already stored.
	if _, statErr := wt.Filesystem.Stat(filePath); statErr == nil {
		return spec.Version, nil
	}

	if err := wt.Filesystem.MkdirAll(tenantID+"/"+spec.Name, 0o755); err != nil {
		return "", fmt.Errorf("workflow/git: mkdir: %w", err)
	}
	if err := writeFile(wt, filePath, raw); err != nil {
		return "", err
	}
	if _, err := wt.Add(filePath); err != nil {
		return "", fmt.Errorf("workflow/git: add: %w", err)
	}

	msg := "workflow: create " + tenantID + "/" + spec.Name + "@" + truncate(spec.Version, 16)
	return spec.Version, s.commit(wt, msg)
}

// Get returns the active version, or workflow.ErrNotFound if none.
func (s *Store) Get(_ context.Context, tenantID, name string) (*workflow.Spec, error) {
	wt, err := s.worktree()
	if err != nil {
		return nil, err
	}
	version, err := s.readActive(wt, tenantID, name)
	if err != nil {
		return nil, err
	}
	return s.readSpec(wt, tenantID, name, version, workflow.StatusActive)
}

// GetVersion returns the specific version, or workflow.ErrNotFound.
func (s *Store) GetVersion(_ context.Context, tenantID, name, version string) (*workflow.Spec, error) {
	wt, err := s.worktree()
	if err != nil {
		return nil, err
	}
	return s.readSpec(wt, tenantID, name, version, workflow.StatusDraft)
}

// List returns names of workflows for tenantID that have at least one version.
func (s *Store) List(_ context.Context, tenantID string) ([]string, error) {
	wt, err := s.worktree()
	if err != nil {
		return nil, err
	}
	dir := tenantID
	entries, err := wt.Filesystem.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist yet — no workflows.
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ListVersions returns version strings in insertion order (oldest first).
func (s *Store) ListVersions(_ context.Context, tenantID, name string) ([]string, error) {
	wt, err := s.worktree()
	if err != nil {
		return nil, err
	}
	dir := tenantID + "/" + name
	if _, statErr := wt.Filesystem.Stat(dir); statErr != nil {
		return nil, fmt.Errorf("workflow %q: %w", name, workflow.ErrNotFound)
	}
	entries, err := wt.Filesystem.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("workflow/git: readdir: %w", err)
	}
	var vers []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			vers = append(vers, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return vers, nil
}

// Activate writes the _active pointer and commits.
func (s *Store) Activate(_ context.Context, tenantID, name, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := s.worktree()
	if err != nil {
		return err
	}

	if _, statErr := wt.Filesystem.Stat(specPath(tenantID, name, version)); statErr != nil {
		return fmt.Errorf("workflow %q version %q: %w", name, version, workflow.ErrVersionConflict)
	}

	aPath := activePath(tenantID, name)
	if err := writeFile(wt, aPath, []byte(version)); err != nil {
		return err
	}
	if _, err := wt.Add(aPath); err != nil {
		return fmt.Errorf("workflow/git: add _active: %w", err)
	}
	msg := "workflow: activate " + tenantID + "/" + name + "@" + truncate(version, 16)
	return s.commit(wt, msg)
}

// Delete removes the workflow directory from the working tree and commits.
func (s *Store) Delete(_ context.Context, tenantID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := s.worktree()
	if err != nil {
		return err
	}
	dir := tenantID + "/" + name
	if _, statErr := wt.Filesystem.Stat(dir); statErr != nil {
		return fmt.Errorf("workflow %q: %w", name, workflow.ErrNotFound)
	}

	if _, err := wt.Remove(dir); err != nil {
		return fmt.Errorf("workflow/git: remove: %w", err)
	}
	msg := "workflow: delete " + tenantID + "/" + name
	return s.commit(wt, msg)
}

// ---- helpers ----

func (s *Store) worktree() (*gogit.Worktree, error) {
	wt, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("workflow/git: worktree: %w", err)
	}
	ref := s.refName()

	// Fast path: branch already exists.
	if _, refErr := s.repo.Reference(ref, true); refErr == nil {
		if err := wt.Checkout(&gogit.CheckoutOptions{Branch: ref}); err != nil {
			return nil, fmt.Errorf("workflow/git: checkout %s: %w", ref, err)
		}
		return wt, nil
	}

	// Branch doesn't exist. If the repo has no commits yet (empty repo),
	// create an initial empty commit so we can create the branch.
	if _, headErr := s.repo.Head(); headErr != nil {
		sig := &object.Signature{
			Name:  committerSig.Name,
			Email: committerSig.Email,
			When:  time.Now(),
		}
		if _, initErr := wt.Commit("init: bunshin-go workflow store", &gogit.CommitOptions{
			Author:            sig,
			Committer:         sig,
			AllowEmptyCommits: true,
		}); initErr != nil {
			return nil, fmt.Errorf("workflow/git: init commit: %w", initErr)
		}
	}

	// Now create the branch.
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: ref, Create: true}); err != nil {
		return nil, fmt.Errorf("workflow/git: create branch %s: %w", ref, err)
	}
	return wt, nil
}

func (s *Store) commit(wt *gogit.Worktree, msg string) error {
	sig := &object.Signature{
		Name:  committerSig.Name,
		Email: committerSig.Email,
		When:  time.Now(),
	}
	_, err := wt.Commit(msg, &gogit.CommitOptions{
		Author:    sig,
		Committer: sig,
		AllowEmptyCommits: false,
	})
	if err != nil && !strings.Contains(err.Error(), "nothing to commit") {
		return fmt.Errorf("workflow/git: commit: %w", err)
	}
	return nil
}

func (s *Store) readActive(wt *gogit.Worktree, tenantID, name string) (string, error) {
	data, err := readFile(wt, activePath(tenantID, name))
	if err != nil {
		return "", fmt.Errorf("workflow %q: no active version: %w", name, workflow.ErrNotFound)
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf("workflow %q: empty active pointer: %w", name, workflow.ErrNotFound)
	}
	return version, nil
}

func (s *Store) readSpec(wt *gogit.Worktree, tenantID, name, version, status string) (*workflow.Spec, error) {
	data, err := readFile(wt, specPath(tenantID, name, version))
	if err != nil {
		return nil, fmt.Errorf("workflow %q version %q: %w", name, version, workflow.ErrNotFound)
	}
	var spec workflow.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("workflow/git: unmarshal: %w", err)
	}
	spec.Version = version
	spec.Status = status
	return &spec, nil
}

func writeFile(wt *gogit.Worktree, path string, data []byte) error {
	f, err := wt.Filesystem.Create(path)
	if err != nil {
		return fmt.Errorf("workflow/git: create %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("workflow/git: write %s: %w", path, err)
	}
	return nil
}

func readFile(wt *gogit.Worktree, path string) ([]byte, error) {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
