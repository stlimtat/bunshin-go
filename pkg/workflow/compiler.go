package workflow

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// Registries holds all runtime dependencies needed to compile a Spec.
type Registries struct {
	// LLM resolves provider instances for llm nodes.
	LLM *llm.ProviderRegistry
	// Tools resolves tool instances for tool nodes.
	Tools *tools.ToolRegistry
	// Custom resolves Go-defined Runnables for custom nodes and custom routers.
	Custom *RunnableRegistry
	// Prompts resolves Fragment templates for llm node prompts.
	Prompts prompt.PromptBackend
	// Routers resolves EIP router factories by type name.
	Routers *RouterRegistry
}

// Compile translates a Spec into a core.Runnable backed by a graph.Graph.
// Missing registry entries surface as errors at compile time, not at invocation.
// ctx is used for any blocking I/O during compilation (e.g. remote prompt fetch).
func Compile(spec *Spec, regs Registries) (core.Runnable, error) {
	return CompileContext(context.Background(), spec, regs)
}

// CompileContext is like Compile but accepts a context for blocking compile-time I/O.
func CompileContext(ctx context.Context, spec *Spec, regs Registries) (core.Runnable, error) {
	if spec == nil {
		return nil, fmt.Errorf("workflow.Compile: spec is nil")
	}
	if len(spec.Nodes) == 0 {
		return nil, fmt.Errorf("workflow.Compile: spec %q has no nodes", spec.Name)
	}
	nodes, err := buildNodes(ctx, spec, regs)
	if err != nil {
		return nil, err
	}
	g := graph.New[map[string]any](spec.Name)
	for _, n := range nodes {
		g.AddNode(n)
	}
	g.SetEntry(nodes[0].ID)
	return g.AsRunnable(), nil
}

// buildNodes converts NodeSpecs to graph.Node[map[string]any].
func buildNodes(ctx context.Context, spec *Spec, regs Registries) ([]graph.Node[map[string]any], error) {
	nodeCount := len(spec.Nodes)
	nodes := make([]graph.Node[map[string]any], 0, nodeCount)

	for i, ns := range spec.Nodes {
		runnable, err := buildRunnable(ctx, ns, regs)
		if err != nil {
			return nil, fmt.Errorf("workflow: node %q: %w", ns.ID, err)
		}

		router, err := buildRouter(ns, i, nodeCount, spec, regs)
		if err != nil {
			return nil, fmt.Errorf("workflow: node %q: router: %w", ns.ID, err)
		}

		nodes = append(nodes, graph.Node[map[string]any]{
			ID:       ns.ID,
			Runnable: runnable,
			Router:   router,
		})
	}
	return nodes, nil
}

// buildRunnable constructs the TypedRunnable for one NodeSpec.
func buildRunnable(ctx context.Context, ns NodeSpec, regs Registries) (core.TypedRunnable[core.State[map[string]any], core.State[map[string]any]], error) {
	switch ns.Runnable.Type {
	case "llm":
		return buildLLMRunnable(ctx, ns, regs)
	case "tool":
		return buildToolRunnable(ns, regs)
	case "custom":
		return buildCustomRunnable(ns, regs)
	default:
		return nil, fmt.Errorf("unknown runnable type %q (want llm|tool|custom)", ns.Runnable.Type)
	}
}

func buildLLMRunnable(ctx context.Context, ns NodeSpec, regs Registries) (core.TypedRunnable[core.State[map[string]any], core.State[map[string]any]], error) {
	if regs.LLM == nil {
		return nil, fmt.Errorf("llm node requires LLM registry")
	}

	// Resolve provider.
	var provider llm.LLMProvider
	ref := ns.Runnable
	switch {
	case ref.ProviderTier != "":
		providers := regs.LLM.Select(llm.Tag("tier", ref.ProviderTier))
		if len(providers) == 0 {
			return nil, fmt.Errorf("llm node: no provider with tier=%q", ref.ProviderTier)
		}
		provider = providers[0]
	case ref.ProviderID != "":
		p, ok := regs.LLM.Get(llm.ProviderID(ref.ProviderID))
		if !ok {
			return nil, fmt.Errorf("llm node: provider %q not found", ref.ProviderID)
		}
		provider = p
	default:
		return nil, fmt.Errorf("llm node requires provider_tier or provider_id")
	}

	// Resolve prompt template at compile time using provided ctx.
	// Optional — when absent, an empty user message is sent.
	var tmpl *template.Template
	if ref.Prompt != "" {
		if regs.Prompts == nil {
			return nil, fmt.Errorf("llm node: prompt %q specified but no Prompts registry", ref.Prompt)
		}
		frag, err := regs.Prompts.Get(ctx, ref.Prompt)
		if err != nil {
			return nil, fmt.Errorf("llm node: prompt fragment %q: %w", ref.Prompt, err)
		}
		t, err := template.New(ref.Prompt).Parse(frag.Content)
		if err != nil {
			return nil, fmt.Errorf("llm node: parse prompt %q: %w", ref.Prompt, err)
		}
		tmpl = t
	}

	inputKey := ref.InputKey
	outputKey := ref.OutputKey

	return core.TypedFunc(func(ctx context.Context, state core.State[map[string]any]) (core.State[map[string]any], error) {
		// Render prompt template with full state.Data.
		// When InputKey is set, the specific value is also bound as ".Input"
		// so templates can reference it by name without repeating the key.
		var userMsg string
		if tmpl != nil {
			var buf bytes.Buffer
			vars := map[string]any{}
			for k, v := range state.Data {
				vars[k] = v
			}
			if inputKey != "" {
				vars["Input"] = state.Data[inputKey]
			}
			if err := tmpl.Execute(&buf, vars); err != nil {
				return state, fmt.Errorf("workflow llm node %q: render prompt: %w", ns.ID, err)
			}
			userMsg = buf.String()
		}

		req := &llm.Request{
			Messages: []llm.Message{{
				Role:  llm.RoleUser,
				Parts: []llm.ContentPart{llm.NewTextPart(userMsg)},
			}},
		}
		resp, err := provider.Complete(ctx, req)
		if err != nil {
			return state, fmt.Errorf("workflow llm node %q: %w", ns.ID, err)
		}

		if outputKey != "" {
			if state.Data == nil {
				state.Data = make(map[string]any)
			}
			state.Data[outputKey] = resp.Content
		}
		return state, nil
	}), nil
}

func buildToolRunnable(ns NodeSpec, regs Registries) (core.TypedRunnable[core.State[map[string]any], core.State[map[string]any]], error) {
	if regs.Tools == nil {
		return nil, fmt.Errorf("tool node requires Tools registry")
	}
	ref := ns.Runnable
	if ref.Name == "" {
		return nil, fmt.Errorf("tool node missing name")
	}
	tool, err := regs.Tools.Get(ref.Name)
	if err != nil {
		return nil, fmt.Errorf("tool node: %w", err)
	}

	inputKey := ref.InputKey
	outputKey := ref.OutputKey

	return core.TypedFunc(func(ctx context.Context, state core.State[map[string]any]) (core.State[map[string]any], error) {
		var input any = state.Data
		if inputKey != "" {
			input = state.Data[inputKey]
		}

		out, err := tool.Invoke(ctx, input)
		if err != nil {
			return state, fmt.Errorf("workflow tool node %q: %w", ns.ID, err)
		}

		if outputKey != "" {
			if state.Data == nil {
				state.Data = make(map[string]any)
			}
			state.Data[outputKey] = out
		}
		return state, nil
	}), nil
}

func buildCustomRunnable(ns NodeSpec, regs Registries) (core.TypedRunnable[core.State[map[string]any], core.State[map[string]any]], error) {
	if regs.Custom == nil {
		return nil, fmt.Errorf("custom node requires Custom registry")
	}
	ref := ns.Runnable
	if ref.Name == "" {
		return nil, fmt.Errorf("custom node missing name")
	}
	r, err := regs.Custom.Get(ref.Name)
	if err != nil {
		return nil, fmt.Errorf("custom node: %w", err)
	}

	return core.TypedFunc(func(ctx context.Context, state core.State[map[string]any]) (core.State[map[string]any], error) {
		out, err := r.Invoke(ctx, state)
		if err != nil {
			return state, fmt.Errorf("workflow custom node %q: %w", ns.ID, err)
		}
		s, ok := out.(core.State[map[string]any])
		if !ok {
			return state, fmt.Errorf("workflow custom node %q: returned %T, want State[map[string]any]", ns.ID, out)
		}
		return s, nil
	}), nil
}

// buildRouter returns the graph.Router for a node. Linear position-based wiring
// is used when neither Next nor Router is declared.
func buildRouter(
	ns NodeSpec,
	idx, total int,
	spec *Spec,
	regs Registries,
) (graph.Router[map[string]any], error) {
	// Explicit EIP / custom router takes priority.
	if ns.Router != nil {
		if regs.Routers == nil {
			return nil, fmt.Errorf("router declared but no RouterRegistry provided")
		}
		return regs.Routers.Build(ns.Router, regs.Custom)
	}

	// Explicit next hop.
	next := ns.Next
	if next == "" {
		// Position-based: last node → END, others → next node id.
		if idx == total-1 {
			next = graph.END
		} else {
			next = spec.Nodes[idx+1].ID
		}
	}
	fixed := next
	return func(_ context.Context, _ core.State[map[string]any]) (string, error) {
		return fixed, nil
	}, nil
}
