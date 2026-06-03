package prompt

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
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

// fragWork holds one resolved fragment ready for rendering.
type fragWork struct {
	frag   *Fragment
	merged map[string]any
	refID  string
}

// Render assembles and renders a PromptTemplate with the given variables.
// Conditions are evaluated sequentially; qualifying fragments are fetched in
// parallel, then rendered in original order.
func (c *PromptComposer) Render(ctx context.Context, t PromptTemplate, vars map[string]any) (string, error) {
	sep := t.Separator
	if sep == "" {
		sep = "\n\n"
	}

	// Phase 1: evaluate conditions sequentially to determine which fragments to fetch.
	type pendingFetch struct {
		idx    int
		ref    FragmentRef
		merged map[string]any
	}
	var pending []pendingFetch
	for i, ref := range t.Fragments {
		if ref.Condition != "" {
			result, err := c.engine.RenderLenient(ref.Condition, vars)
			if err != nil {
				return "", fmt.Errorf("fragment %q condition: %w", ref.ID, err)
			}
			if strings.TrimSpace(result) == "" || strings.TrimSpace(result) == "false" {
				continue
			}
		}

		merged := make(map[string]any, len(vars)+len(ref.Overrides))
		for k, v := range vars {
			merged[k] = v
		}
		for k, v := range ref.Overrides {
			merged[k] = v
		}
		pending = append(pending, pendingFetch{idx: i, ref: ref, merged: merged})
	}

	if len(pending) == 0 {
		return "", nil
	}

	// Phase 2: fetch all qualifying fragments concurrently.
	work := make([]fragWork, len(pending))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for i, p := range pending {
		i, p := i, p
		g.Go(func() error {
			frag, err := c.backend.Get(gctx, p.ref.ID)
			if err != nil {
				return fmt.Errorf("fragment %q: %w", p.ref.ID, err)
			}
			if err := frag.Validate(p.merged); err != nil {
				return err
			}
			mu.Lock()
			work[i] = fragWork{frag: frag, merged: p.merged, refID: p.ref.ID}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}

	// Phase 3: render fragments in original order (engine may not be goroutine-safe).
	parts := make([]string, 0, len(work))
	for _, w := range work {
		rendered, err := c.engine.Render(w.frag.Content, w.merged)
		if err != nil {
			return "", fmt.Errorf("fragment %q render: %w", w.refID, err)
		}
		parts = append(parts, rendered)
	}

	return strings.Join(parts, sep), nil
}
