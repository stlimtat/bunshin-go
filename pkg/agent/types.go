package agent

import (
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

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
