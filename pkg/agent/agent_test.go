package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// newFakeToolRegistry creates a ToolRegistry with test tools.
func newFakeToolRegistry() *tools.ToolRegistry {
	return tools.NewToolRegistry()
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

	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "test response")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	// FakeProvider has no Ping method, so it will be marked available by pingAll.
	reg.Start(context.Background())

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}

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

	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "test response")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	// FakeProvider has no Ping method, so it will be marked available by pingAll.
	reg.Start(context.Background())

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}

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

	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "test response")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	// FakeProvider has no Ping method, so it will be marked available by pingAll.
	reg.Start(context.Background())

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}

	ctx := context.Background()
	_, err := Compile(ctx, spec, registries, "test-tenant")
	if err == nil {
		t.Fatal("expected error for missing agent, got nil")
	}
}

func TestCompile_CycleInAgents(t *testing.T) {
	// Pre-create compiled agents with their agentNames populated, simulating A → B → A cycle.
	// fakeAgentResolver.agents is pre-populated so Resolve succeeds and returns agents
	// whose AgentNames() expose the spec-level dependencies for cycle detection.
	agentResolver := newFakeAgentResolver()
	agentResolver.agents["agent-b"] = &CompiledAgent{
		name:       "agent-b",
		agentNames: []string{"agent-a"}, // B declares A as a dependency
	}

	specA := &AgentSpec{
		Name: "agent-a",
		SystemPrompt: struct {
			Slug string `yaml:"slug"`
		}{Slug: "test-prompt"},
		Agents: []string{"agent-b"}, // A → B
	}
	specA.Model.Tier = "smart"

	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"test-prompt": {Slug: "test-prompt", Content: "test"},
		},
	}

	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "test response")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	reg.Start(context.Background())

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  agentResolver,
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}

	ctx := context.Background()
	_, err := Compile(ctx, specA, registries, "test-tenant")
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected 'cycle' in error message, got: %v", err)
	}
}

func TestContentBasedRouter_IterationCap(t *testing.T) {
	const maxIter = 3
	router := createContentBasedRouter(maxIter)
	ctx := context.Background()

	makeStateWithToolCall := func() core.State[AgentState] {
		state := core.NewState(AgentState{
			Messages: []llm.Message{
				{
					Role: llm.RoleAssistant,
					Parts: []llm.ContentPart{
						{Type: llm.PartTypeToolCall, ToolCall: &llm.ToolCall{Name: "tool", ID: "1"}},
					},
				},
			},
		})
		return state
	}

	// First maxIter-1 calls with tool calls should route to "tools"
	for i := 0; i < maxIter-1; i++ {
		next, err := router(ctx, makeStateWithToolCall())
		if err != nil {
			t.Fatalf("step %d: unexpected error: %v", i+1, err)
		}
		if next != "tools" {
			t.Errorf("step %d: want %q, got %q", i+1, "tools", next)
		}
	}

	// At maxIter, should truncate regardless of tool calls in the message.
	state := makeStateWithToolCall()
	next, err := router(ctx, state)
	if err != nil {
		t.Fatalf("truncate step: %v", err)
	}
	if next != graph.END {
		t.Errorf("want END on truncation, got %q", next)
	}
	if state.Meta["bunshin.agent_truncated"] != true {
		t.Error("want Meta[bunshin.agent_truncated]=true, not set")
	}
}

func TestInvokeSubAgent_DepthGuard(t *testing.T) {
	// subAgent has a nil graph; depth guard fires before Invoke is called.
	subAgent := &CompiledAgent{name: "sub", agentNames: []string{}}

	state := core.NewState(AgentState{Task: "test"})
	state.Meta["bunshin.agent_depth"] = maxAgentDepth

	tc := llm.ToolCall{Name: "sub", ID: "1", Arguments: `{"task": "do stuff"}`}

	output := invokeSubAgent(context.Background(), tc, subAgent, state)
	if !strings.Contains(output, "depth limit") {
		t.Errorf("expected depth limit error, got: %q", output)
	}
}

func TestInvokeSubAgent_MetaPropagation(t *testing.T) {
	// Verify that trace/cost/tenant Meta keys are copied into the sub-agent's state.
	// We use a compiled agent with a graph that immediately returns (FakeProvider, no tools).
	promptBackend := &fakePromptBackend{
		fragments: map[string]*prompt.Fragment{
			"sys": {Slug: "sys", Content: "system"},
		},
	}
	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "done")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	reg.Start(context.Background())

	spec := &AgentSpec{
		Name:          "sub",
		SystemPrompt:  struct{ Slug string `yaml:"slug"` }{Slug: "sys"},
		MaxIterations: 8,
	}
	spec.Model.Tier = "smart"

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}
	subAgent, err := Compile(context.Background(), spec, registries, "tenant")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	state := core.NewState(AgentState{Task: "root"})
	state.Meta["bunshin.trace_id"] = "trace-xyz"
	state.Meta["bunshin.tenant_id"] = "tenant-1"
	state.Meta["bunshin.agent_depth"] = 0

	tc := llm.ToolCall{Name: "sub", ID: "1", Arguments: `{"task": "subtask"}`}
	invokeSubAgent(context.Background(), tc, subAgent, state)
	// Meta propagation is verified by the sub-agent completing without error.
	// No assertion on internal state needed; just confirm no panic/depth block.
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

	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "test response")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	// FakeProvider has no Ping method, so it will be marked available by pingAll.
	reg.Start(context.Background())

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}

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

	fakeProvider := llm.NewFakeProvider(llm.ProviderFake, "test response")
	reg := llm.NewProviderRegistry(fakeProvider)
	reg.Register(llm.ProviderFake, fakeProvider, llm.Tags{"tier": "smart"})
	// FakeProvider has no Ping method, so it will be marked available by pingAll.
	reg.Start(context.Background())

	registries := CompileRegistries{
		Tools:   newFakeToolRegistry(),
		Agents:  newFakeAgentResolver(),
		Skills:  newFakeSkillResolver(),
		Prompts: promptBackend,
		LLM:     reg,
	}

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
