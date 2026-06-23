package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"gopkg.in/yaml.v3"
)

// ErrNotFound is returned when a skill or version is absent.
var ErrNotFound = errors.New("skill: not found")

// ErrVersionConflict is returned when Activate references an unknown version.
var ErrVersionConflict = errors.New("skill: version not found")

// StatusDraft marks a spec that has not yet been activated.
const StatusDraft = "draft"

// StatusActive marks the live version of a skill.
const StatusActive = "active"

// Spec is the canonical in-memory representation of a YAML skill definition.
type Spec struct {
	// Name identifies the skill within a tenant.
	Name string `yaml:"name"`
	// Description is optional human-readable documentation.
	Description string `yaml:"description,omitempty"`
	// Body is a reference to the Fragment containing the skill's instructions.
	Body prompt.FragmentRef `yaml:"body"`
	// Files are optional bundled scripts and documentation.
	Files []FileRef `yaml:"files,omitempty"`
	// Trigger is either "model" or "condition".
	// "model" means the skill is advertised as a load_skill_<name> tool.
	// "condition" means it's optional with a Condition expression.
	Trigger string `yaml:"trigger"`

	// Version is the content-hash version string (set by Parse, not parsed from YAML).
	Version string `yaml:"-"`
	// Status is "draft" or "active" (set by Store, not parsed from YAML).
	Status string `yaml:"-"`
}

// FileRef is a bundled file within a skill.
type FileRef struct {
	// Name is the file identifier (e.g. "script.py", "README.md").
	Name string `yaml:"name"`
	// MediaRef is a reference to the file content (inline or URL).
	MediaRef *llm.MediaRef `yaml:"media_ref"`
}

// Parse decodes YAML bytes into a Spec, validates it, and sets its Version field.
func Parse(data []byte) (*Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("skill: parse: %w", err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("skill: parse: name is required")
	}
	if s.Body.Slug == "" {
		return nil, fmt.Errorf("skill: parse: body.slug is required")
	}
	if s.Trigger != "model" && s.Trigger != "condition" {
		return nil, fmt.Errorf("skill: parse: trigger must be 'model' or 'condition', got %q", s.Trigger)
	}

	canonical, err := canonicalJSON(&s)
	if err != nil {
		return nil, fmt.Errorf("skill: parse: canonicalise: %w", err)
	}
	s.Version = contentHash(canonical)
	return &s, nil
}

// canonicalJSON marshals the stable fields of spec to JSON with sorted map keys.
// JSON encoding in Go sorts map keys alphabetically, making it deterministic
// across backends regardless of map iteration order.
func canonicalJSON(s *Spec) ([]byte, error) {
	type canonical struct {
		Name        string             `json:"name"`
		Description string             `json:"description,omitempty"`
		Body        prompt.FragmentRef `json:"body"`
		Files       []FileRef          `json:"files,omitempty"`
		Trigger     string             `json:"trigger"`
	}
	return json.Marshal(canonical{
		Name:        s.Name,
		Description: s.Description,
		Body:        s.Body,
		Files:       s.Files,
		Trigger:     s.Trigger,
	})
}

// contentHash returns "sha256:" + first 32 hex chars of the SHA-256 digest.
// 32 hex chars = 128 bits, providing strong collision resistance.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])[:32]
}
