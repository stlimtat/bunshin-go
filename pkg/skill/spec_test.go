package skill_test

import (
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/skill"
)

func TestParse_Valid(t *testing.T) {
	yaml := `
name: investigation-skill
description: Skill for conducting investigations
body: {slug: investigate-fragment, condition: "model.role == 'analyst'"}
files:
  - name: script.py
    media_ref: {url: "https://example.com/script.py"}
  - name: README.md
    media_ref: {url: "https://example.com/README.md"}
trigger: model
`
	spec, err := skill.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if spec.Name != "investigation-skill" {
		t.Errorf("want name investigation-skill, got %q", spec.Name)
	}
	if spec.Trigger != "model" {
		t.Errorf("want trigger model, got %q", spec.Trigger)
	}
	if spec.Body.Slug != "investigate-fragment" {
		t.Errorf("want body.slug investigate-fragment, got %q", spec.Body.Slug)
	}
	if len(spec.Files) != 2 {
		t.Errorf("want 2 files, got %d", len(spec.Files))
	}
	if spec.Version == "" {
		t.Errorf("Parse should set Version")
	}
}

func TestParse_MissingName(t *testing.T) {
	yaml := `
body: {slug: frag}
trigger: model
`
	_, err := skill.Parse([]byte(yaml))
	if err == nil {
		t.Errorf("Parse should error on missing name")
	}
}

func TestParse_MissingBody(t *testing.T) {
	yaml := `
name: test
trigger: model
`
	_, err := skill.Parse([]byte(yaml))
	if err == nil {
		t.Errorf("Parse should error on missing body.slug")
	}
}

func TestParse_InvalidTrigger(t *testing.T) {
	yaml := `
name: test
body: {slug: frag}
trigger: invalid
`
	_, err := skill.Parse([]byte(yaml))
	if err == nil {
		t.Errorf("Parse should error on invalid trigger")
	}
}

func TestParse_Idempotent(t *testing.T) {
	yaml := `
name: test-skill
body: {slug: instructions}
trigger: model
`
	spec1, _ := skill.Parse([]byte(yaml))
	spec2, _ := skill.Parse([]byte(yaml))
	if spec1.Version != spec2.Version {
		t.Errorf("same YAML should yield same version")
	}
}

func TestParse_VersionDeterministic(t *testing.T) {
	yaml := `
name: test-skill
description: A test skill
body: {slug: instructions}
trigger: model
files:
  - name: a.py
    media_ref: {url: "https://example.com/a.py"}
  - name: b.py
    media_ref: {url: "https://example.com/b.py"}
`
	spec1, _ := skill.Parse([]byte(yaml))
	spec2, _ := skill.Parse([]byte(yaml))
	if spec1.Version != spec2.Version {
		t.Errorf("want same version for identical YAML, got %q vs %q", spec1.Version, spec2.Version)
	}
	if !spec1.Version[:7] == "sha256:" {
		t.Errorf("version should start with sha256:, got %q", spec1.Version)
	}
}

func TestParse_Condition_Trigger(t *testing.T) {
	yaml := `
name: conditional-skill
body: {slug: frag, condition: "user_role == 'admin'"}
trigger: condition
`
	spec, err := skill.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if spec.Trigger != "condition" {
		t.Errorf("want trigger condition, got %q", spec.Trigger)
	}
}
