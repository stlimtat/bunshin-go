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
	backend  PromptBackend
	tenantID string
	engine   TemplateEngine
}

// NewPromptComposer constructs a PromptComposer backed by backend for tenantID.
func NewPromptComposer(backend PromptBackend, tenantID string) *PromptComposer {
	return &PromptComposer{backend: backend, tenantID: tenantID, engine: &GoTemplateEngine{}}
}

// WithEngine replaces the default GoTemplateEngine.
func (c *PromptComposer) WithEngine(e TemplateEngine) *PromptComposer {
	c.engine = e
	return c
}

type fragWork struct {
	frag    *Fragment
	merged  map[string]any
	refSlug string
}

// Render assembles and renders a PromptTemplate with the given variables.
func (c *PromptComposer) Render(ctx context.Context, t PromptTemplate, vars map[string]any) (string, error) {
	sep := t.Separator
	if sep == "" {
		sep = "\n\n"
	}

	// Phase 1: evaluate conditions to determine qualifying fragments.
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
				return "", fmt.Errorf("fragment %q condition: %w", ref.Slug, err)
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

	// Phase 2: fetch qualifying fragments concurrently by slug.
	work := make([]fragWork, len(pending))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for i, p := range pending {
		i, p := i, p
		g.Go(func() error {
			frag, err := c.backend.Get(gctx, c.tenantID, p.ref.Slug)
			if err != nil {
				return fmt.Errorf("fragment %q: %w", p.ref.Slug, err)
			}
			if err := frag.Validate(p.merged); err != nil {
				return err
			}
			mu.Lock()
			work[i] = fragWork{frag: frag, merged: p.merged, refSlug: p.ref.Slug}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", err
	}

	// Phase 3: render in original order.
	parts := make([]string, 0, len(work))
	for _, w := range work {
		rendered, err := c.engine.Render(w.frag.Content, w.merged)
		if err != nil {
			return "", fmt.Errorf("fragment %q render: %w", w.refSlug, err)
		}
		parts = append(parts, rendered)
	}

	return strings.Join(parts, sep), nil
}
