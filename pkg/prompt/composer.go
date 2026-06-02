package prompt

import (
	"context"
	"fmt"
)

// PromptComposer assembles PromptTemplates into rendered strings.
type PromptComposer struct {
	backend PromptBackend
	engine  TemplateEngine
}

// NewPromptComposer constructs a PromptComposer backed by backend.
// Uses GoTemplateEngine by default.
func NewPromptComposer(backend PromptBackend) *PromptComposer {
	return &PromptComposer{backend: backend, engine: &GoTemplateEngine{}}
}

// WithEngine replaces the default GoTemplateEngine.
func (c *PromptComposer) WithEngine(e TemplateEngine) *PromptComposer {
	c.engine = e
	return c
}

// Render assembles and renders a PromptTemplate with the given variables.
// Fragment-level Overrides are merged on top of vars for each fragment.
func (c *PromptComposer) Render(ctx context.Context, t PromptTemplate, vars map[string]any) (string, error) {
	sep := t.Separator
	if sep == "" {
		sep = "\n\n"
	}

	var parts []string
	for _, ref := range t.Fragments {
		if ref.Condition != "" {
			result, err := c.engine.Render(ref.Condition, vars)
			if err != nil {
				return "", fmt.Errorf("fragment %q condition: %w", ref.ID, err)
			}
			if result == "" || result == "false" {
				continue
			}
		}

		frag, err := c.backend.Get(ctx, ref.ID)
		if err != nil {
			return "", fmt.Errorf("fragment %q: %w", ref.ID, err)
		}

		merged := make(map[string]any, len(vars)+len(ref.Overrides))
		for k, v := range vars {
			merged[k] = v
		}
		for k, v := range ref.Overrides {
			merged[k] = v
		}

		if err := frag.Validate(merged); err != nil {
			return "", err
		}

		rendered, err := c.engine.Render(frag.Content, merged)
		if err != nil {
			return "", fmt.Errorf("fragment %q render: %w", ref.ID, err)
		}
		parts = append(parts, rendered)
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result, nil
}
