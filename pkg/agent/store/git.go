package store

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"gopkg.in/yaml.v3"
)

// GitStore is a Store backed by a git repository, treating agent specs as version-controlled documents.
//
// Branch layout:
//   agents/{tenantID}/{name}/draft     — contains the most recent draft spec
//   agents/{tenantID}/{name}/main      — contains the active spec
//
// File layout within each branch:
//   spec.yaml     — the agent spec
//
// Commits are made with:
//   Author: "bunshin agent service <agent@bunshin.dev>"
//   Message: "version={sha256:...}"
type GitStore struct {
	repo *git.Repository
}

// NewGitStore constructs a store backed by repo.
// repo must be an open *git.Repository, typically obtained via git.PlainOpen(path).
func NewGitStore(repo *git.Repository) *GitStore {
	return &GitStore{repo: repo}
}

const (
	gitAuthorName  = "bunshin agent service"
	gitAuthorEmail = "agent@bunshin.dev"
)

// Create persists spec as a new draft. Idempotent: same content = same version.
func (s *GitStore) Create(ctx context.Context, tenantID string, spec *AgentSpec) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("git.Store.Create: spec is nil")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("git.Store.Create: spec.Name must not be empty")
	}

	version, err := contentHashYAML(spec)
	if err != nil {
		return "", fmt.Errorf("git.Store: hash: %w", err)
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("git.Store: marshal: %w", err)
	}

	branchName := fmt.Sprintf("agents/%s/%s/draft", tenantID, spec.Name)
	return version, s.writeSpecToBranch(ctx, branchName, data, version)
}

// Get returns the active spec for tenantID/name.
func (s *GitStore) Get(ctx context.Context, tenantID, name string) (*AgentSpec, error) {
	branchName := fmt.Sprintf("agents/%s/%s/main", tenantID, name)
	data, err := s.readSpecFromBranch(ctx, branchName)
	if err != nil {
		return nil, fmt.Errorf("agent %q tenant %q: no active version", name, tenantID)
	}

	spec, err := decodeAgentSpec(data)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// GetVersion returns a specific version. For git-backed storage, version lookup
// requires scanning history; for efficiency, only the latest active and draft are reliable.
func (s *GitStore) GetVersion(ctx context.Context, tenantID, name, version string) (*AgentSpec, error) {
	// Try draft first, then main
	branchName := fmt.Sprintf("agents/%s/%s/draft", tenantID, name)
	data, err := s.readSpecFromBranch(ctx, branchName)
	if err == nil {
		spec, err := decodeAgentSpec(data)
		if err == nil && matchesVersion(spec, version) {
			return spec, nil
		}
	}

	branchName = fmt.Sprintf("agents/%s/%s/main", tenantID, name)
	data, err = s.readSpecFromBranch(ctx, branchName)
	if err == nil {
		spec, err := decodeAgentSpec(data)
		if err == nil && matchesVersion(spec, version) {
			return spec, nil
		}
	}

	return nil, fmt.Errorf("agent %q version %q tenant %q: not found", name, version, tenantID)
}

// List returns all agent names with either a draft or main branch.
func (s *GitStore) List(ctx context.Context, tenantID string) ([]string, error) {
	refIter, err := s.repo.References()
	if err != nil {
		return nil, fmt.Errorf("git.Store: list refs: %w", err)
	}
	defer refIter.Close()

	nameSet := make(map[string]bool)
	prefix := fmt.Sprintf("refs/heads/agents/%s/", tenantID)

	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if !strings.HasPrefix(name, prefix) {
			return nil
		}
		// Extract name from refs/heads/agents/{tenantID}/{name}/draft or /main
		parts := strings.Split(name[len(prefix):], "/")
		if len(parts) >= 2 {
			agentName := parts[0]
			nameSet[agentName] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("git.Store: foreach refs: %w", err)
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	return names, nil
}

// ListVersions returns version history by scanning the draft and main branches.
// For a true version history, you would need to walk the commit log.
// This simplified version returns only the current draft and main.
func (s *GitStore) ListVersions(ctx context.Context, tenantID, name string) ([]AgentVersion, error) {
	var versions []AgentVersion

	// Try main (active)
	mainBranch := fmt.Sprintf("agents/%s/%s/main", tenantID, name)
	if data, err := s.readSpecFromBranch(ctx, mainBranch); err == nil {
		if spec, err := decodeAgentSpec(data); err == nil {
			version, _ := contentHashYAML(spec)
			versions = append(versions, AgentVersion{
				Version:   version,
				Status:    "active",
				CreatedAt: time.Now(),
			})
		}
	}

	// Try draft
	draftBranch := fmt.Sprintf("agents/%s/%s/draft", tenantID, name)
	if data, err := s.readSpecFromBranch(ctx, draftBranch); err == nil {
		if spec, err := decodeAgentSpec(data); err == nil {
			version, _ := contentHashYAML(spec)
			// Only add if different from active
			if len(versions) == 0 || versions[0].Version != version {
				versions = append(versions, AgentVersion{
					Version:   version,
					Status:    "draft",
					CreatedAt: time.Now(),
				})
			}
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}

	return versions, nil
}

// Activate promotes the draft spec to main (active).
func (s *GitStore) Activate(ctx context.Context, tenantID, name, version string) error {
	draftBranch := fmt.Sprintf("agents/%s/%s/draft", tenantID, name)
	mainBranch := fmt.Sprintf("agents/%s/%s/main", tenantID, name)

	// Read draft
	data, err := s.readSpecFromBranch(ctx, draftBranch)
	if err != nil {
		return fmt.Errorf("agent %q: no draft to activate", name)
	}

	// Verify the version matches
	spec, err := decodeAgentSpec(data)
	if err != nil {
		return fmt.Errorf("agent %q: decode draft: %w", name, err)
	}

	specVersion, err := contentHashYAML(spec)
	if err != nil {
		return fmt.Errorf("agent %q: hash: %w", name, err)
	}

	if specVersion != version {
		return fmt.Errorf("agent %q version %q: draft has version %q", name, version, specVersion)
	}

	// Copy draft to main
	return s.writeSpecToBranch(ctx, mainBranch, data, version)
}

// Delete removes all branches for tenantID/name (draft and main).
func (s *GitStore) Delete(ctx context.Context, tenantID, name string) error {
	draftBranch := fmt.Sprintf("agents/%s/%s/draft", tenantID, name)
	mainBranch := fmt.Sprintf("agents/%s/%s/main", tenantID, name)

	foundAny := false
	for _, branch := range []string{draftBranch, mainBranch} {
		ref := plumbing.NewBranchReferenceName(branch)
		err := s.repo.Storer.RemoveReference(ref)
		if err == nil {
			foundAny = true
		}
	}

	if !foundAny {
		return fmt.Errorf("agent %q tenant %q: not found", name, tenantID)
	}

	return nil
}

// writeSpecToBranch writes data to branchName with a commit message containing the version.
// For now, this is a simplified implementation that stores to worktree only.
// A full implementation would use go-git's low-level plumbing API for branch management.
func (s *GitStore) writeSpecToBranch(ctx context.Context, branchName string, data []byte, version string) error {
	wt, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("git.Store: get worktree: %w", err)
	}

	// Create the file in the worktree filesystem using billy.File interface
	f, err := wt.Filesystem.Create("spec.yaml")
	if err != nil {
		return fmt.Errorf("git.Store: create file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("git.Store: write file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("git.Store: close file: %w", err)
	}

	// Add to index
	if _, err := wt.Add("spec.yaml"); err != nil {
		return fmt.Errorf("git.Store: add: %w", err)
	}

	// Commit with message containing version
	commitHash, err := wt.Commit(fmt.Sprintf("version=%s", version), &git.CommitOptions{
		Author: &object.Signature{
			Name:  gitAuthorName,
			Email: gitAuthorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git.Store: commit: %w", err)
	}

	// Create a branch reference pointing to this commit
	ref := plumbing.NewBranchReferenceName(branchName)
	hashRef := plumbing.NewHashReference(ref, commitHash)
	return s.repo.Storer.SetReference(hashRef)
}

// readSpecFromBranch reads spec.yaml from branchName.
func (s *GitStore) readSpecFromBranch(ctx context.Context, branchName string) ([]byte, error) {
	ref := plumbing.NewBranchReferenceName(branchName)
	hash, err := s.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, fmt.Errorf("git.Store: resolve %q: %w", branchName, err)
	}

	commit, err := s.repo.CommitObject(*hash)
	if err != nil {
		return nil, fmt.Errorf("git.Store: get commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("git.Store: get tree: %w", err)
	}

	// Get the file from the tree
	file, err := tree.File("spec.yaml")
	if err != nil {
		return nil, fmt.Errorf("git.Store: get file: %w", err)
	}

	reader, err := file.Reader()
	if err != nil {
		return nil, fmt.Errorf("git.Store: file reader: %w", err)
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// matchesVersion checks if spec's content hash matches version.
func matchesVersion(spec *AgentSpec, version string) bool {
	computed, err := contentHashYAML(spec)
	if err != nil {
		return false
	}
	return computed == version
}
