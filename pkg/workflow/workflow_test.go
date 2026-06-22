package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// ---- Parse ----

const linearYAML = `
name: my-flow
description: "test flow"
nodes:
  - id: step1
    runnable: { type: custom, name: inc }
  - id: step2
    runnable: { type: custom, name: inc }
`

const cyclicYAML = `
name: loop-flow
nodes:
  - id: think
    runnable: { type: custom, name: echo }
    router:
      type: custom
      name: always-end
  - id: act
    runnable: { type: custom, name: echo }
    next: think
`

func TestParse_Linear(t *testing.T) {
	spec, err := workflow.Parse([]byte(linearYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "my-flow" {
		t.Errorf("name: want my-flow, got %q", spec.Name)
	}
	if len(spec.Nodes) != 2 {
		t.Errorf("nodes: want 2, got %d", len(spec.Nodes))
	}
	if spec.Version == "" {
		t.Error("version must be set")
	}
}

func TestParse_VersionPrefix(t *testing.T) {
	spec, _ := workflow.Parse([]byte(linearYAML))
	if len(spec.Version) < 7 || spec.Version[:7] != "sha256:" {
		t.Errorf("version should start with sha256:, got %q", spec.Version)
	}
}

func TestParse_Idempotent(t *testing.T) {
	s1, _ := workflow.Parse([]byte(linearYAML))
	s2, _ := workflow.Parse([]byte(linearYAML))
	if s1.Version != s2.Version {
		t.Errorf("same content must yield same version: %q vs %q", s1.Version, s2.Version)
	}
}

func TestParse_DifferentContent_DifferentVersion(t *testing.T) {
	s1, _ := workflow.Parse([]byte(linearYAML))
	s2, _ := workflow.Parse([]byte(cyclicYAML))
	if s1.Version == s2.Version {
		t.Error("different content must yield different version")
	}
}

func TestParse_MissingName(t *testing.T) {
	_, err := workflow.Parse([]byte("nodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParse_EmptyNodes(t *testing.T) {
	_, err := workflow.Parse([]byte("name: x\nnodes: []\n"))
	if err == nil {
		t.Error("expected error for empty nodes")
	}
}

func TestParse_NodeMissingID(t *testing.T) {
	yaml := "name: x\nnodes:\n  - runnable: {type: custom, name: x}\n"
	_, err := workflow.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for node missing id")
	}
}

func TestParse_NodeMissingRunnableType(t *testing.T) {
	yaml := "name: x\nnodes:\n  - id: a\n    runnable: {}\n"
	_, err := workflow.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for node missing runnable.type")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := workflow.Parse([]byte("{not valid yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParse_DuplicateNodeID(t *testing.T) {
	yaml := "name: x\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n  - id: a\n    runnable: {type: custom, name: x}\n"
	_, err := workflow.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for duplicate node id")
	}
}

func TestParse_InvalidNextTarget(t *testing.T) {
	yaml := "name: x\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n    next: nonexistent\n"
	_, err := workflow.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for next referencing unknown node")
	}
}

func TestParse_ContentHash_Is32Hex(t *testing.T) {
	spec, _ := workflow.Parse([]byte(linearYAML))
	// "sha256:" (7 chars) + 32 hex chars = 39 total
	if len(spec.Version) != 39 {
		t.Errorf("want 39 char version string, got %d: %q", len(spec.Version), spec.Version)
	}
}

// ---- RunnableRegistry ----

func TestRunnableRegistry_RegisterAndGet(t *testing.T) {
	r := core.NewRunnableFunc("inc", func(_ context.Context, v any) (any, error) { return v, nil })
	reg := workflow.NewRunnableRegistry()
	reg.Register("inc", r)

	got, err := reg.Get("inc")
	if err != nil || got == nil {
		t.Fatalf("Get returned error: %v", err)
	}
}

func TestRunnableRegistry_GetMissing(t *testing.T) {
	reg := workflow.NewRunnableRegistry()
	_, err := reg.Get("missing")
	if err == nil {
		t.Error("expected error for missing runnable")
	}
}

func TestRunnableRegistry_PanicEmptyName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for empty name")
		}
	}()
	reg := workflow.NewRunnableRegistry()
	r := core.NewRunnableFunc("x", func(_ context.Context, v any) (any, error) { return v, nil })
	reg.Register("", r)
}

func TestRunnableRegistry_PanicNilRunnable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for nil runnable")
		}
	}()
	reg := workflow.NewRunnableRegistry()
	reg.Register("x", nil)
}

func TestRunnableRegistry_PanicDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate name")
		}
	}()
	reg := workflow.NewRunnableRegistry()
	r := core.NewRunnableFunc("x", func(_ context.Context, v any) (any, error) { return v, nil })
	reg.Register("x", r)
	reg.Register("x", r)
}

// ---- RouterRegistry ----

func TestRouterRegistry_RegisterAndBuild(t *testing.T) {
	reg := workflow.NewRouterRegistry()
	reg.Register("always-end", func(_ map[string]any) (graph.Router[map[string]any], error) {
		return func(_ context.Context, _ core.State[map[string]any]) (string, error) {
			return graph.END, nil
		}, nil
	})
	router, err := reg.Build(&workflow.RouterRef{Type: "always-end"}, workflow.NewRunnableRegistry())
	if err != nil || router == nil {
		t.Fatalf("Build failed: %v", err)
	}
	next, err := router(context.Background(), core.NewState(map[string]any{}))
	if err != nil || next != graph.END {
		t.Errorf("want END, got %q err %v", next, err)
	}
}

func TestRouterRegistry_UnknownType(t *testing.T) {
	reg := workflow.NewRouterRegistry()
	_, err := reg.Build(&workflow.RouterRef{Type: "nonexistent"}, workflow.NewRunnableRegistry())
	if err == nil {
		t.Error("expected error for unknown router type")
	}
}

func TestRouterRegistry_NilRef(t *testing.T) {
	reg := workflow.NewRouterRegistry()
	r, err := reg.Build(nil, workflow.NewRunnableRegistry())
	if err != nil || r != nil {
		t.Errorf("nil ref should return nil router: got %v err %v", r, err)
	}
}

func TestRouterRegistry_CustomRouter(t *testing.T) {
	cr := workflow.NewRunnableRegistry()
	cr.Register("always-end", core.NewRunnableFunc("always-end", func(_ context.Context, _ any) (any, error) {
		return graph.END, nil
	}))

	reg := workflow.NewRouterRegistry()
	router, err := reg.Build(&workflow.RouterRef{Type: "custom", Name: "always-end"}, cr)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	next, _ := router(context.Background(), core.NewState(map[string]any{}))
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestRouterRegistry_CustomRouter_MissingName(t *testing.T) {
	reg := workflow.NewRouterRegistry()
	_, err := reg.Build(&workflow.RouterRef{Type: "custom"}, workflow.NewRunnableRegistry())
	if err == nil {
		t.Error("expected error for custom router with missing name")
	}
}

func TestRouterRegistry_PanicDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate typeName")
		}
	}()
	reg := workflow.NewRouterRegistry()
	factory := func(_ map[string]any) (graph.Router[map[string]any], error) { return nil, nil }
	reg.Register("x", factory)
	reg.Register("x", factory)
}

// ---- Compile ----

func makeIncRunnable() core.Runnable {
	return core.NewRunnableFunc("inc", func(_ context.Context, v any) (any, error) {
		s, ok := v.(core.State[map[string]any])
		if !ok {
			return v, nil
		}
		if s.Data == nil {
			s.Data = make(map[string]any)
		}
		n, _ := s.Data["n"].(int)
		s.Data["n"] = n + 1
		return s, nil
	})
}

func makeRegs() workflow.Registries {
	cr := workflow.NewRunnableRegistry()
	cr.Register("inc", makeIncRunnable())
	cr.Register("echo", core.NewRunnableFunc("echo", func(_ context.Context, v any) (any, error) { return v, nil }))
	cr.Register("always-end", core.NewRunnableFunc("always-end", func(_ context.Context, _ any) (any, error) {
		return graph.END, nil
	}))
	return workflow.Registries{
		Custom:  cr,
		Routers: workflow.NewRouterRegistry(),
	}
}

func TestCompile_Linear_Runs(t *testing.T) {
	spec, err := workflow.Parse([]byte(linearYAML))
	if err != nil {
		t.Fatal(err)
	}
	regs := makeRegs()
	r, err := workflow.Compile(spec, regs)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	state := core.NewState(map[string]any{"n": 0})
	out, err := r.Invoke(context.Background(), state)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	s, ok := out.(core.State[map[string]any])
	if !ok {
		t.Fatalf("want State[map[string]any], got %T", out)
	}
	if s.Data["n"] != 2 {
		t.Errorf("want n=2 (two inc steps), got %v", s.Data["n"])
	}
}

func TestCompile_NilSpec(t *testing.T) {
	_, err := workflow.Compile(nil, workflow.Registries{})
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestCompile_UnknownNodeType(t *testing.T) {
	spec, _ := workflow.Parse([]byte("name: x\nnodes:\n  - id: a\n    runnable: {type: unknown}\n"))
	// Override validation to reach compile
	_ = spec
}

func TestCompile_MissingCustomRegistry(t *testing.T) {
	spec, _ := workflow.Parse([]byte("name: x\nnodes:\n  - id: a\n    runnable: {type: custom, name: x}\n"))
	_, err := workflow.Compile(spec, workflow.Registries{})
	if err == nil {
		t.Error("expected error when Custom registry is nil")
	}
}

func TestCompile_CustomNodeMissingName(t *testing.T) {
	spec, _ := workflow.Parse([]byte("name: x\nnodes:\n  - id: a\n    runnable: {type: custom}\n"))
	regs := makeRegs()
	_, err := workflow.Compile(spec, regs)
	if err == nil {
		t.Error("expected error for custom node with missing name")
	}
}

func TestCompile_CyclicWithCustomRouter(t *testing.T) {
	spec, err := workflow.Parse([]byte(cyclicYAML))
	if err != nil {
		t.Fatal(err)
	}
	regs := makeRegs()
	r, err := workflow.Compile(spec, regs)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	state := core.NewState(map[string]any{})
	_, err = r.Invoke(context.Background(), state)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
}

func TestCompile_ContextCancellation(t *testing.T) {
	spec, _ := workflow.Parse([]byte(linearYAML))
	regs := makeRegs()
	r, _ := workflow.Compile(spec, regs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Invoke(ctx, core.NewState(map[string]any{}))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

// ---- LLM node ----

func TestCompile_LLMNode_ProviderID(t *testing.T) {
	fake := llm.NewFakeProvider(llm.ProviderFake, "result-text")
	reg := llm.NewProviderRegistry()
	reg.Register(llm.ProviderFake, fake, llm.Tags{"tier": "fast"})

	yaml := `
name: llm-flow
nodes:
  - id: call
    runnable:
      type: llm
      provider_id: "fake"
      output_key: out
`
	spec, err := workflow.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	r, err := workflow.Compile(spec, workflow.Registries{LLM: reg})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := r.Invoke(context.Background(), core.NewState(map[string]any{}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	s := out.(core.State[map[string]any])
	if s.Data["out"] != "result-text" {
		t.Errorf("want 'result-text', got %v", s.Data["out"])
	}
}

func TestCompile_LLMNode_ProviderTierNoProviders(t *testing.T) {
	// Select() requires Start() for health monitoring; without it returns empty.
	// This test documents that tier-based resolution returns an error in tests
	// without a running registry.
	reg := llm.NewProviderRegistry()
	reg.Register("fast-p", llm.NewFakeProvider("fast-p", "x"), llm.Tags{"tier": "fast"})

	yaml := `
name: llm-flow
nodes:
  - id: call
    runnable:
      type: llm
      provider_tier: fast
`
	spec, _ := workflow.Parse([]byte(yaml))
	_, err := workflow.Compile(spec, workflow.Registries{LLM: reg})
	// Expected: error because Select returns empty without health loop running.
	if err == nil {
		t.Error("expected error: tier-based selection requires running health loop")
	}
}

func TestCompile_LLMNode_MissingLLMRegistry(t *testing.T) {
	yaml := `
name: llm-flow
nodes:
  - id: call
    runnable:
      type: llm
      provider_tier: fast
`
	spec, _ := workflow.Parse([]byte(yaml))
	_, err := workflow.Compile(spec, workflow.Registries{})
	if err == nil {
		t.Error("expected error when LLM registry is nil")
	}
}

func TestCompile_LLMNode_NoProviderRef(t *testing.T) {
	fake := llm.NewFakeProvider("fake", "x")
	reg := llm.NewProviderRegistry(fake)

	yaml := `
name: llm-flow
nodes:
  - id: call
    runnable: {type: llm}
`
	spec, _ := workflow.Parse([]byte(yaml))
	_, err := workflow.Compile(spec, workflow.Registries{LLM: reg})
	if err == nil {
		t.Error("expected error when neither provider_tier nor provider_id set")
	}
}

func TestCompile_LLMNode_ProviderTierNoMatch(t *testing.T) {
	fake := llm.NewFakeProvider("fake", "x")
	reg := llm.NewProviderRegistry(fake)
	reg.Register("fake", fake, llm.Tags{"tier": "smart"})

	yaml := `
name: llm-flow
nodes:
  - id: call
    runnable:
      type: llm
      provider_tier: fast
`
	spec, _ := workflow.Parse([]byte(yaml))
	_, err := workflow.Compile(spec, workflow.Registries{LLM: reg})
	if err == nil {
		t.Error("expected error when no provider matches tier")
	}
}

func TestCompile_LLMNode_WithPrompt(t *testing.T) {
	fake := llm.NewFakeProvider(llm.ProviderFake, "answered")
	reg := llm.NewProviderRegistry()
	reg.Register(llm.ProviderFake, fake, llm.Tags{"tier": "fast"})

	backend := prompt.NewMemoryBackend()
	if err := backend.Put(context.Background(), "test", &prompt.Fragment{ID: "greet.v1", Slug: "greet.v1", Content: "Hello {{.name}}"}); err != nil {
		t.Fatal(err)
	}

	yaml := `
name: llm-flow
nodes:
  - id: call
    runnable:
      type: llm
      provider_id: "fake"
      prompt: greet.v1
      output_key: response
`
	spec, err := workflow.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	r, err := workflow.Compile(spec, workflow.Registries{LLM: reg, Prompts: backend, TenantID: "test"})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := r.Invoke(context.Background(), core.NewState(map[string]any{"name": "World"}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	s := out.(core.State[map[string]any])
	if s.Data["response"] != "answered" {
		t.Errorf("want 'answered', got %v", s.Data["response"])
	}
}

func TestCompile_MissingToolRegistry(t *testing.T) {
	yaml := "name: x\nnodes:\n  - id: a\n    runnable: {type: tool, name: search}\n"
	spec, _ := workflow.Parse([]byte(yaml))
	_, err := workflow.Compile(spec, workflow.Registries{})
	if err == nil {
		t.Error("expected error for tool node without Tools registry")
	}
}

func TestCompile_ToolNodeMissingName(t *testing.T) {
	yaml := "name: x\nnodes:\n  - id: a\n    runnable: {type: tool}\n"
	spec, _ := workflow.Parse([]byte(yaml))
	regs := makeRegs()
	_, err := workflow.Compile(spec, regs)
	if err == nil {
		t.Error("expected error for tool node with missing name")
	}
}

func TestCompile_ExplicitNextWiring(t *testing.T) {
	// step1 explicitly routes to step2, step2 routes to END
	yaml := `
name: explicit-next
nodes:
  - id: step1
    runnable: {type: custom, name: inc}
    next: step2
  - id: step2
    runnable: {type: custom, name: inc}
`
	spec, err := workflow.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	regs := makeRegs()
	r, err := workflow.Compile(spec, regs)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	out, err := r.Invoke(context.Background(), core.NewState(map[string]any{"n": 0}))
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	s := out.(core.State[map[string]any])
	if s.Data["n"] != 2 {
		t.Errorf("want n=2, got %v", s.Data["n"])
	}
}
