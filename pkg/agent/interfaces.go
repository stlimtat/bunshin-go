// Package agent provides declarative agents with systematic subagent and skill composition.
//
// An Agent is a Tool-like Runnable with its own system prompt, tool allowlist,
// nested agent allowlist, and isolated message loop. Agents are:
//   - Compiled from AgentSpec at load time (topological compile + cycle detection)
//   - Invoked via three surfaces: as a Tool, as a WorkflowNode, or as a top-level Runnable
//   - Isolated on fresh AgentState (own message history, separate from parent)
//   - Subject to iteration caps (max_iterations, agent depth guards)
//
// Agents reference other agents (subagent delegation), introducing potential cycles.
// The compiler uses topological sort (Kahn's algorithm) to detect cycles at
// compile time and order agents for compilation.
//
// Skills (lightweight, loop-free capabilities) are defined in pkg/skill.
package agent

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// CompiledAgent is the result of Compile(spec, registries).
// It wraps a Graph[AgentState] and implements three interfaces:
//   - core.Runnable (top-level invocation)
//   - tools.Tool (agent-as-tool delegation)
//   - core.TypedRunnable[State[AgentState], State[AgentState]] (composition inside graphs)
type CompiledAgent struct {
	// Name and Description from the spec.
	name        string
	description string

	// The compiled execution graph: llm node → router → tools node → llm (loop until END).
	graph *graph.Graph[AgentState]

	// Input/output schemas for validation.
	inputSchema  map[string]any
	outputSchema map[string]any

	// Max iterations: enforced in the content-based router closure.
	maxIterations int

	// agentNames is the raw agent allowlist from the spec.
	// Used by the compiler's cycle detector to traverse the transitive dependency graph.
	agentNames []string
}

// Name returns the agent identifier.
func (ca *CompiledAgent) Name() string {
	return ca.name
}

// Description returns the agent's human-readable description.
func (ca *CompiledAgent) Description() string {
	return ca.description
}

// AgentNames returns the agent names declared in the spec's agents allowlist.
// Used by the compiler's cycle detector to traverse the transitive dependency graph.
func (ca *CompiledAgent) AgentNames() []string {
	return ca.agentNames
}

// Invoke executes the agent synchronously on the given task.
// Input is expected to be core.State[AgentState] with Data.Task and Data.Args populated.
// Returns the final agent state (with Messages accumulated across turns).
//
// Implements core.Runnable.
func (ca *CompiledAgent) Invoke(ctx context.Context, input any) (any, error) {
	state, ok := input.(core.State[AgentState])
	if !ok {
		return nil, &core.TypeMismatchError{Runnable: ca.name, Got: input}
	}

	// Delegate to the graph.
	result, err := ca.graph.Invoke(ctx, state)
	return result, core.WrapError(ca.name, err)
}

// Stream is not implemented for agents yet (returns an error).
func (ca *CompiledAgent) Stream(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := ca.Invoke(ctx, input)
		ch <- core.StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}

// Schema returns the agent's tool schema (name, description, input contract).
// This allows agents to be listed in tool allowlists and called via LLM function-calling.
//
// Implements tools.Tool.
func (ca *CompiledAgent) Schema() tools.ToolSchema {
	return tools.ToolSchema{
		Name:        ca.name,
		Description: ca.description,
		Parameters:  ca.inputSchema,
	}
}

// AgentResolver resolves agent names to compiled agents.
// Used during compilation to resolve agent-as-tool references.
type AgentResolver interface {
	// Resolve returns the CompiledAgent for the given name, or an error if not found.
	// The resolver may use memoization to avoid recompiling the same agent.
	Resolve(ctx context.Context, tenantID, name string) (*CompiledAgent, error)
}

// SkillResolver resolves skill names to skill specs.
// Used during compilation to resolve skill references.
type SkillResolver interface {
	// Resolve returns the SkillSpec for the given name, or an error if not found.
	Resolve(ctx context.Context, tenantID, name string) (SkillSpec, error)
}

// SkillSpec is a placeholder for the actual skill definition (defined in pkg/skill).
// It represents a lightweight, loop-free capability that can be injected.
type SkillSpec interface {
	// SkillName returns the skill identifier.
	SkillName() string
}
