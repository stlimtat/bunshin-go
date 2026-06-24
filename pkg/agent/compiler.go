package agent

import (
	"context"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
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

// Compile builds a CompiledAgent from an AgentSpec.
//
// Compilation is eager and topological: all references (tools, agents, skills,
// fragments) are resolved at compile time. Missing refs return compile errors,
// not runtime panics. Agents that reference other agents are subject to
// topological sort + cycle detection.
//
// Steps:
// 1. Load system prompt Fragment by slug
// 2. Resolve all tool names against registries.Tools
// 3. Resolve all agent names — topologically ordered + cycle detection
// 4. Resolve all skill names against registries.Skills
// 5. Resolve model selector against registries.LLM
// 6. Build a Graph[AgentState] with LLM and tool nodes + content-based router
// 7. Wrap in iteration cap middleware
// 8. Return CompiledAgent
func Compile(
	ctx context.Context,
	spec *AgentSpec,
	registries CompileRegistries,
	tenantID string,
) (*CompiledAgent, error) {
	if spec == nil {
		return nil, fmt.Errorf("agent: Compile: spec is nil")
	}

	// Step 1: Load system prompt
	systemPrompt, err := registries.Prompts.Get(ctx, tenantID, spec.SystemPrompt.Slug)
	if err != nil {
		return nil, fmt.Errorf("agent %q: system prompt %q: %w", spec.Name, spec.SystemPrompt.Slug, err)
	}

	// Step 2: Resolve tools
	resolvedTools, err := resolveTools(registries.Tools, spec.Tools)
	if err != nil {
		return nil, fmt.Errorf("agent %q: resolve tools: %w", spec.Name, err)
	}

	// Step 3: Resolve agents (with topological ordering and cycle detection)
	resolvedAgents, agentsCycle, err := resolveAgentsTopological(ctx, registries.Agents, tenantID, spec.Agents, spec.Name)
	if err != nil {
		if agentsCycle != nil {
			return nil, fmt.Errorf("agent %q: cycle in agents: %s", spec.Name, formatCyclePath(agentsCycle))
		}
		return nil, fmt.Errorf("agent %q: resolve agents: %w", spec.Name, err)
	}

	// Step 4: Resolve skills
	resolvedSkills, err := resolveSkills(ctx, registries.Skills, tenantID, spec.Skills)
	if err != nil {
		return nil, fmt.Errorf("agent %q: resolve skills: %w", spec.Name, err)
	}
	_ = resolvedSkills // TODO: skills are a future feature

	// Step 5: Resolve LLM provider
	provider, err := selectProvider(registries.LLM, spec.Model.Tier, spec.Model.Tags)
	if err != nil {
		return nil, fmt.Errorf("agent %q: resolve LLM: %w", spec.Name, err)
	}

	// Step 6: Build the graph
	g := graph.New[AgentState](spec.Name)

	// Create LLM node
	llmNode := graph.Node[AgentState]{
		ID: "llm",
		Runnable: createLLMNode(
			spec.Name,
			systemPrompt,
			provider,
			resolvedTools,
			resolvedAgents,
		),
		Router: createContentBasedRouter(),
	}
	g.AddNode(llmNode)

	// Create tools node
	toolsNode := graph.Node[AgentState]{
		ID:       "tools",
		Runnable: createToolsNode(spec.Name, resolvedTools, resolvedAgents),
		Router:   graph.StaticRouter[AgentState]("llm"),
	}
	g.AddNode(toolsNode)

	g.SetEntry("llm")

	// Step 7: Wrap with iteration cap middleware
	// (This would be applied by the LLM node; for now, integration happens at invoke time)

	return &CompiledAgent{
		name:          spec.Name,
		description:   spec.Description,
		graph:         g,
		inputSchema:   spec.InputSchema,
		outputSchema:  spec.OutputSchema,
		maxIterations: spec.MaxIterations,
	}, nil
}

// resolveTools returns the set of tools allowed by the agent.
func resolveTools(registry *tools.ToolRegistry, toolNames []string) (map[string]tools.Tool, error) {
	resolved := make(map[string]tools.Tool)
	var missing []string

	for _, name := range toolNames {
		t, err := registry.Get(name)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		resolved[name] = t
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("tools not found: %v", missing)
	}

	return resolved, nil
}

// resolveAgentsTopological resolves agent names and returns them in topological order.
// If a cycle is detected, returns the cycle path.
func resolveAgentsTopological(
	ctx context.Context,
	resolver AgentResolver,
	tenantID string,
	agentNames []string,
	parentAgent string,
) (map[string]*CompiledAgent, []string, error) {
	if len(agentNames) == 0 {
		return make(map[string]*CompiledAgent), nil, nil
	}

	// Build dependency graph: agent → agents it references
	deps := make(map[string][]string)
	agents := make(map[string]*CompiledAgent)

	// First pass: resolve all agents in the allowlist
	var missing []string
	for _, name := range agentNames {
		agent, err := resolver.Resolve(ctx, tenantID, name)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		agents[name] = agent
	}

	if len(missing) > 0 {
		return nil, nil, fmt.Errorf("agents not found: %v", missing)
	}

	// Second pass: extract dependencies from each resolved agent
	// (This requires introspection into the spec; for now, we assume acyclic)
	// TODO: if we have access to original specs, build the full dependency DAG
	for _, name := range agentNames {
		deps[name] = []string{} // TODO: extract from the agent's spec
	}

	// Kahn's algorithm: topological sort + cycle detection
	cycle := kahnTopologicalSort(agentNames, deps)
	if cycle != nil {
		return nil, cycle, nil
	}

	return agents, nil, nil
}

// kahnTopologicalSort performs topological sort and detects cycles.
// Returns the cycle path if a cycle is detected, nil if the graph is acyclic.
func kahnTopologicalSort(nodes []string, edges map[string][]string) []string {
	// Calculate in-degrees
	inDegree := make(map[string]int)
	for _, node := range nodes {
		if _, ok := inDegree[node]; !ok {
			inDegree[node] = 0
		}
		for _, neighbor := range edges[node] {
			inDegree[neighbor]++
		}
	}

	// Queue of nodes with in-degree 0
	queue := []string{}
	for _, node := range nodes {
		if inDegree[node] == 0 {
			queue = append(queue, node)
		}
	}

	sorted := []string{}
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		// Process neighbors
		for _, neighbor := range edges[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// If sorted has fewer nodes than input, there's a cycle
	if len(sorted) < len(nodes) {
		// Find a cycle via DFS
		return findCycleDFS(nodes, edges)
	}

	return nil
}

// findCycleDFS performs DFS to find and return a cycle path.
func findCycleDFS(nodes []string, edges map[string][]string) []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var path []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, neighbor := range edges[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				// Found cycle: extract from neighbor to current node
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					return true
				}
			}
		}

		path = path[:len(path)-1]
		recStack[node] = false
		return false
	}

	for _, node := range nodes {
		if !visited[node] {
			path = []string{}
			if dfs(node) {
				return path
			}
		}
	}

	return nil
}

// formatCyclePath formats a cycle path for error messages.
func formatCyclePath(cycle []string) string {
	if len(cycle) == 0 {
		return "unknown cycle"
	}
	s := ""
	for i, node := range cycle {
		if i > 0 {
			s += " → "
		}
		s += node
	}
	// Close the cycle
	if len(cycle) > 0 {
		s += " → " + cycle[0]
	}
	return s
}

// resolveSkills resolves skill names.
func resolveSkills(
	ctx context.Context,
	resolver SkillResolver,
	tenantID string,
	skillNames []string,
) (map[string]SkillSpec, error) {
	resolved := make(map[string]SkillSpec)
	var missing []string

	for _, name := range skillNames {
		skill, err := resolver.Resolve(ctx, tenantID, name)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		resolved[name] = skill
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("skills not found: %v", missing)
	}

	return resolved, nil
}

// selectProvider selects an LLM provider matching the tier and tags.
func selectProvider(
	registry *llm.ProviderRegistry,
	tier string,
	tags map[string]string,
) (llm.LLMProvider, error) {
	// Build tag filters
	filters := []llm.Tags{llm.Tag("tier", tier)}
	for k, v := range tags {
		filters = append(filters, llm.Tag(k, v))
	}

	providers := registry.Select(filters...)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no LLM provider found for tier=%q, tags=%v", tier, tags)
	}

	// Return the first (most available) provider
	return providers[0], nil
}

// createLLMNode creates the LLM invocation node.
func createLLMNode(
	agentName string,
	systemPrompt *prompt.Fragment,
	provider llm.LLMProvider,
	tools map[string]tools.Tool,
	agents map[string]*CompiledAgent,
) core.TypedRunnable[core.State[AgentState], core.State[AgentState]] {
	return core.TypedFunc(func(ctx context.Context, state core.State[AgentState]) (core.State[AgentState], error) {
		// Build LLM request
		// System prompt + task
		messages := []llm.Message{
			llm.NewTextMessage(llm.RoleSystem, systemPrompt.Content),
			llm.NewTextMessage(llm.RoleUser, state.Data.Task),
		}

		// Append agent's own history
		messages = append(messages, state.Data.Messages...)

		// Build tool definitions from tools + agents
		var toolDefs []llm.ToolDefinition
		for _, t := range tools {
			schema := t.Schema()
			toolDefs = append(toolDefs, llm.ToolDefinition{
				Name:        schema.Name,
				Description: schema.Description,
				Parameters:  schema.Parameters,
			})
		}
		for _, a := range agents {
			schema := a.Schema()
			toolDefs = append(toolDefs, llm.ToolDefinition{
				Name:        schema.Name,
				Description: schema.Description,
				Parameters:  schema.Parameters,
			})
		}

		// Call LLM
		resp, err := provider.Complete(ctx, &llm.Request{
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return state, fmt.Errorf("agent %q LLM call: %w", agentName, err)
		}

		// Append LLM response to messages
		state.Data.Messages = append(state.Data.Messages, llm.NewTextMessage(llm.RoleAssistant, resp.Content))
		for _, tc := range resp.ToolCalls {
			state.Data.Messages = append(state.Data.Messages, llm.Message{
				Role: llm.RoleAssistant,
				Parts: []llm.ContentPart{
					{Type: llm.PartTypeToolCall, ToolCall: &tc},
				},
			})
		}

		return state, nil
	})
}

// createToolsNode creates the tools invocation node.
func createToolsNode(
	agentName string,
	tools map[string]tools.Tool,
	agents map[string]*CompiledAgent,
) core.TypedRunnable[core.State[AgentState], core.State[AgentState]] {
	return core.TypedFunc(func(ctx context.Context, state core.State[AgentState]) (core.State[AgentState], error) {
		// Extract tool calls from the last message
		if len(state.Data.Messages) == 0 {
			return state, nil // No messages, no tool calls to process
		}

		lastMsg := state.Data.Messages[len(state.Data.Messages)-1]
		if lastMsg.Role != llm.RoleAssistant {
			return state, nil // Last message is not from assistant, no tool calls
		}

		// Find tool calls in the message parts
		var toolCalls []llm.ToolCall
		for _, part := range lastMsg.Parts {
			if part.Type == llm.PartTypeToolCall && part.ToolCall != nil {
				toolCalls = append(toolCalls, *part.ToolCall)
			}
		}

		if len(toolCalls) == 0 {
			return state, nil // No tool calls, nothing to do
		}

		// Execute tool calls
		results := []llm.ToolResult{}
		for _, tc := range toolCalls {
			// Try to find tool
			tool, ok := tools[tc.Name]
			var output string

			if ok {
				// Tool found, execute it
				// Parse args as JSON input
				result, err := tool.Invoke(ctx, tc.Arguments)
				if err != nil {
					output = fmt.Sprintf("error: %v", err)
				} else {
					// Convert result to string (simplified)
					output = fmt.Sprintf("%v", result)
				}
			} else {
				// Try to find as agent
				agent, ok := agents[tc.Name]
				if ok {
					// Delegate to agent
					subState := core.NewState(AgentState{
						Task:     tc.Arguments,
						Args:     map[string]any{},
						Messages: []llm.Message{},
					})
					result, err := agent.Invoke(ctx, subState)
					if err != nil {
						output = fmt.Sprintf("error: %v", err)
					} else {
						// Extract result from state
						if resultState, ok := result.(core.State[AgentState]); ok {
							if len(resultState.Data.Messages) > 0 {
								output = resultState.Data.Messages[len(resultState.Data.Messages)-1].Text()
							}
						}
					}
				} else {
					output = fmt.Sprintf("tool %q not found", tc.Name)
				}
			}

			results = append(results, llm.ToolResult{
				ToolCallID: tc.ID,
				Content:    output,
			})
		}

		// Append tool results to messages
		for _, result := range results {
			state.Data.Messages = append(state.Data.Messages, llm.Message{
				Role: llm.RoleTool,
				Parts: []llm.ContentPart{
					{Type: llm.PartTypeToolResult, ToolResult: &result},
				},
			})
		}

		return state, nil
	})
}

// createContentBasedRouter routes based on presence of tool calls in the last message.
// If the last assistant message contains tool calls, route to "tools".
// Otherwise, route to END (terminate).
func createContentBasedRouter() graph.Router[AgentState] {
	return func(ctx context.Context, state core.State[AgentState]) (string, error) {
		if len(state.Data.Messages) == 0 {
			return graph.END, nil
		}

		lastMsg := state.Data.Messages[len(state.Data.Messages)-1]
		if lastMsg.Role != llm.RoleAssistant {
			return graph.END, nil
		}

		// Check for tool calls
		for _, part := range lastMsg.Parts {
			if part.Type == llm.PartTypeToolCall && part.ToolCall != nil {
				return "tools", nil
			}
		}

		// No tool calls, terminate
		return graph.END, nil
	}
}
