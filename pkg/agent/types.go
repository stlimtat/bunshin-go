package agent

import (
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// CompileRegistries holds the registries needed to compile an agent.
type CompileRegistries struct {
	// Tools resolves tool names against the registered tool set.
	Tools *tools.ToolRegistry

	// Agents resolves agent names (subagent delegation).
	// Typically implements memoization to avoid recompiling the same agent.
	Agents AgentResolver

	// Skills resolves skill names.
	Skills SkillResolver

	// Prompts loads Fragment specs by slug.
	Prompts prompt.PromptBackend

	// LLM resolves model tier and tags to a provider.
	LLM *llm.ProviderRegistry
}

// AgentState is the isolated state context for a compiled agent.
// Each agent invocation runs on a fresh AgentState with its own message history.
//
// Task is the input string (e.g., "investigate the bug at file.go:42").
// Args holds structured arguments validated against InputSchema.
// Messages is the agent's own conversation history across loop turns.
type AgentState struct {
	// Task is the input instruction to the agent.
	Task string

	// Args holds structured input arguments (validated against InputSchema if present).
	Args map[string]any

	// Messages is the agent's own message history, accumulated across loop iterations.
	// Starts empty; each LLM turn appends to it.
	Messages []llm.Message
}
