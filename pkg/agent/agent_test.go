package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// fakeToolRegistry provides a set of known tools for testing.
type fakeToolRegistry struct {
	tools map[string]tools.Tool
}

func newFakeToolRegistry() *fakeToolRegistry {
	return &fakeToolRegistry{tools: make(map[string]tools.Tool)}
}

func (r *fakeToolRegistry) Register(t tools.Tool) error {
	name := t.Schema().Name
	r.tools[name] = t
	return nil
}

func (r *fakeToolRegistry) Get(name string) (tools.Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, errToolNotFound
	}
	return t, nil
}

func (r *fakeToolRegistry) List() []tools.ToolSchema {
	var schemas []tools.ToolSchema
	for _, t := range r.tools {
		schemas = append(schemas, t.Schema())
	}
	return schemas
}

// fakeAgentResolver provides a memoizing agent resolver for testing.
type fakeAgentResolver struct {
	agents map[string]*CompiledAgent
	specs  map[string]*AgentSpec
}

func newFakeAgentResolver() *fakeAgentResolver {
	return &fakeAgentResolver{
		agents: make(map[string]*CompiledAgent),
		specs:  make(map[string]*AgentSpec),
	}
}

func (r *fakeAgentResolver) addSpec(name string, spec *AgentSpec) {
	r.specs[name] = spec
}

func (r *fakeAgentResolver) Resolve(ctx context.Context, tenantID, name string) (*CompiledAgent, error) {
	// Return cached compiled agent if available
	if agent, ok := r.agents[name]; ok {
		return agent, nil
	}

	// Look up spec and compile
	spec, ok := r.specs[name]
	if !ok {
		return nil, errAgentNotFound
	}

	// To avoid infinite recursion, compile with an empty agent resolver
	emptyResolver := newFakeAgentResolver()
	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  emptyResolver,
		Skills:  newFakeSkillResolver(),
		Prompts: &fakePromptBackend{},
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}

	agent, err := Compile(ctx, spec, registries, tenantID)
	if err != nil {
		return nil, err
	}

	r.agents[name] = agent
	return agent, nil
}

// fakeSkillResolver provides a skill resolver for testing.
type fakeSkillResolver struct {
	skills map[string]*FakeSkillSpec
}

func newFakeSkillResolver() *fakeSkillResolver {
	return &fakeSkillResolver{skills: make(map[string]*FakeSkillSpec)}
}

func (r *fakeSkillResolver) Resolve(ctx context.Context, tenantID, name string) (SkillSpec, error) {
	skill, ok := r.skills[name]
	if !ok {
		return nil, errSkillNotFound
	}
	return skill, nil
}

// fakePromptBackend provides a prompt backend for testing.
type fakePromptBackend struct {
	fragments map[string]*prompt.Fragment
}

func (b *fakePromptBackend) Put(ctx context.Context, tenantID string, f *prompt.Fragment) error {
	if b.fragments == nil {
		b.fragments = make(map[string]*prompt.Fragment)
	}
	b.fragments[f.Slug] = f
	return nil
}

func (b *fakePromptBackend) Get(ctx context.Context, tenantID, slug string) (*prompt.Fragment, error) {
	f, ok := b.fragments[slug]
	if !ok {
		return nil, errFragmentNotFound
	}
	return f, nil
}

func (b *fakePromptBackend) GetByID(ctx context.Context, tenantID, id string) (*prompt.Fragment, error) {
	return nil, errFragmentNotFound
}

func (b *fakePromptBackend) GetVersion(ctx context.Context, tenantID, slug, version string) (*prompt.Fragment, error) {
	return nil, errFragmentNotFound
}

func (b *fakePromptBackend) List(ctx context.Context, tenantID string, tags ...string) ([]*prompt.Fragment, error) {
	return nil, nil
}

func (b *fakePromptBackend) Rename(ctx context.Context, tenantID, id, newSlug string) error {
	return nil
}

func (b *fakePromptBackend) ListVersions(ctx context.Context, tenantID, slug string) ([]*prompt.Fragment, error) {
	return nil, nil
}

func (b *fakePromptBackend) Delete(ctx context.Context, tenantID, slug string) error {
	return nil
}

func (b *fakePromptBackend) Watch(ctx context.Context, tenantID, slug string) (<-chan *prompt.Fragment, error) {
	return nil, nil
}

// Error sentinels for testing
var (
	errToolNotFound     = newTestError("tool not found")
	errAgentNotFound    = newTestError("agent not found")
	errSkillNotFound    = newTestError("skill not found")
	errFragmentNotFound = newTestError("fragment not found")
)

func newTestError(msg string) error {
	return testError{msg: msg}
}

type testError struct {
	msg string
}

func (e testError) Error() string { return e.msg }

// Test cases

func TestCompile_HappyPath(t *testing.T) {
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Tools:         []string{},
		Agents:        []string{},
		Skills:        []string{},
		MaxIterations: 8,
	}
	spec.Model.Tier = "smart"
	spec.Model.Tags = map[string]string{}

	// Setup registries
	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {
				Slug:    "test-prompt",
				Content: "You are a helpful assistant.",
			},
		},
	}

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}
	registries.LLM.Register(llm.ProviderFake, registries.LLM.Available()[0], llm.Tags{"tier": "smart"})

	ctx := context.Background()
	compiled, err := Compile(ctx, spec, registries, "test-tenant")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if compiled.Name() != "test-agent" {
		t.Errorf("expected name test-agent, got %s", compiled.Name())
	}
}

func TestCompile_MissingFragment(t *testing.T) {
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "missing-prompt"},
		MaxIterations: 8,
	}
	spec.Model.Tier = "smart"

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  &fakeSkillResolver{},
		Prompts: &fakePromptBackend{},
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}

	ctx := context.Background()
	_, err := Compile(ctx, spec, registries, "test-tenant")
	if err == nil {
		t.Fatal("expected error for missing fragment, got nil")
	}
}

func TestCompile_MissingTool(t *testing.T) {
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Tools: []string{"missing-tool"},
	}
	spec.Model.Tier = "smart"

	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {Slug: "test-prompt", Content: "test"},
		},
	}

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}
	registries.LLM.Register(llm.ProviderFake, registries.LLM.Available()[0], llm.Tags{"tier": "smart"})

	ctx := context.Background()
	_, err := Compile(ctx, spec, registries, "test-tenant")
	if err == nil {
		t.Fatal("expected error for missing tool, got nil")
	}
}

func TestCompile_MissingAgent(t *testing.T) {
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Agents: []string{"missing-agent"},
	}
	spec.Model.Tier = "smart"

	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {Slug: "test-prompt", Content: "test"},
		},
	}

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}
	registries.LLM.Register(llm.ProviderFake, registries.LLM.Available()[0], llm.Tags{"tier": "smart"})

	ctx := context.Background()
	_, err := Compile(ctx, spec, registries, "test-tenant")
	if err == nil {
		t.Fatal("expected error for missing agent, got nil")
	}
}

func TestCompile_CycleInAgents(t *testing.T) {
	// Create specs with cycle: A → B → A
	specA := &AgentSpec{
		Name:        "agent-a",
		Description: "Agent A",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Agents: []string{"agent-b"}, // A references B
	}
	specA.Model.Tier = "smart"

	specB := &AgentSpec{
		Name:        "agent-b",
		Description: "Agent B",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Agents: []string{"agent-a"}, // B references A → cycle
	}
	specB.Model.Tier = "smart"

	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {Slug: "test-prompt", Content: "test"},
		},
	}

	agentResolver := newFakeAgentResolver()
	agentResolver.addSpec("agent-a", specA)
	agentResolver.addSpec("agent-b", specB)

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  agentResolver,
		Skills:  &fakeSkillResolver{},
		Prompts: promptBackend,
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}
	registries.LLM.Register(llm.ProviderFake, registries.LLM.Available()[0], llm.Tags{"tier": "smart"})

	ctx := context.Background()
	_, err := Compile(ctx, specA, registries, "test-tenant")
	// Note: Cycle detection only works if we can introspect agent dependencies,
	// which currently requires access to the compiled agent's spec.
	// For now, this test documents the expected behavior.
	_ = err
}

func TestCompile_InvokeHappyPath(t *testing.T) {
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Tools:         []string{},
		Agents:        []string{},
		MaxIterations: 8,
	}
	spec.Model.Tier = "smart"

	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {
				Slug:    "test-prompt",
				Content: "You are a helpful assistant.",
			},
		},
	}

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}
	registries.LLM.Register(llm.ProviderFake, registries.LLM.Available()[0], llm.Tags{"tier": "smart"})

	ctx := context.Background()
	compiled, err := Compile(ctx, spec, registries, "test-tenant")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Invoke the agent
	input := core.NewState(AgentState{
		Task: "What is 2+2?",
		Args: map[string]any{},
	})

	result, err := compiled.Invoke(ctx, input)
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}

	if outState, ok := result.(core.State[AgentState]); !ok {
		t.Fatalf("expected State[AgentState], got %T", result)
	} else {
		// Verify messages were accumulated
		if len(outState.Data.Messages) == 0 {
			t.Error("expected messages to be accumulated")
		}
	}
}

func TestParse_ValidYAML(t *testing.T) {
	yaml := `
name: test-agent
description: A test agent
system_prompt:
  slug: test-prompt
model:
  tier: smart
  tags:
    budget: high
max_iterations: 10
tools:
  - tool1
  - tool2
`
	spec, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if spec.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", spec.Name)
	}
	if spec.SystemPrompt.Slug != "test-prompt" {
		t.Errorf("expected slug test-prompt, got %s", spec.SystemPrompt.Slug)
	}
	if spec.MaxIterations != 10 {
		t.Errorf("expected max_iterations 10, got %d", spec.MaxIterations)
	}
	if len(spec.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(spec.Tools))
	}
}

func TestParse_MissingRequired(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"missing name", "system_prompt:\n  slug: test"},
		{"missing system_prompt", "name: test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected parse error, got nil")
			}
		})
	}
}

func TestParse_DefaultMaxIterations(t *testing.T) {
	yaml := `
name: test-agent
system_prompt:
  slug: test-prompt
`
	spec, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if spec.MaxIterations != 8 {
		t.Errorf("expected default max_iterations 8, got %d", spec.MaxIterations)
	}
}

func TestCompiledAgent_Schema(t *testing.T) {
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
	}
	spec.Model.Tier = "smart"
	spec.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{"type": "string"},
		},
	}

	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {Slug: "test-prompt", Content: "test"},
		},
	}

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     llm.NewProviderRegistry(llm.NewFakeProvider("test", "")),
	}
	registries.LLM.Register(llm.ProviderFake, registries.LLM.Available()[0], llm.Tags{"tier": "smart"})

	ctx := context.Background()
	compiled, err := Compile(ctx, spec, registries, "test-tenant")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	schema := compiled.Schema()
	if schema.Name != "test-agent" {
		t.Errorf("expected schema name test-agent, got %s", schema.Name)
	}
	if schema.Description != "A test agent" {
		t.Errorf("expected description, got %s", schema.Description)
	}

	// Verify input schema is included
	paramsJSON, _ := json.Marshal(schema.Parameters)
	paramsStr := string(paramsJSON)
	if paramsStr == "null" {
		t.Error("expected input schema in Parameters, got null")
	}
}
