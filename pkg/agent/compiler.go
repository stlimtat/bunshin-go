package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// maxAgentDepth is the runtime cap on nested agent invocation depth.
// Exceeded depth returns an error to the calling LLM, not a panic.
const maxAgentDepth = 8

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
	if agentsCycle != nil {
		return nil, fmt.Errorf("agent %q: cycle in agents: %s", spec.Name, formatCyclePath(agentsCycle))
	}
	if err != nil {
		return nil, fmt.Errorf("agent %q: resolve agents: %w", spec.Name, err)
	}

	// Step 4: Resolve skills
	resolvedSkills, err := resolveSkills(ctx, registries.Skills, tenantID, spec.Skills)
	if err != nil {
		return nil, fmt.Errorf("agent %q: resolve skills: %w", spec.Name, err)
	}

	// Step 5: Resolve LLM provider
	provider, err := selectProvider(registries.LLM, spec.Model.Tier, spec.Model.Tags)
	if err != nil {
		return nil, fmt.Errorf("agent %q: resolve LLM: %w", spec.Name, err)
	}

	maxIter := spec.MaxIterations
	if maxIter == 0 {
		maxIter = 8
	}

	// Step 6: Build the graph
	g := graph.New[AgentState](spec.Name)

	llmNode := graph.Node[AgentState]{
		ID: "llm",
		Runnable: createLLMNode(
			spec.Name,
			systemPrompt,
			provider,
			resolvedTools,
			resolvedAgents,
			resolvedSkills,
		),
		Router: createContentBasedRouter(maxIter),
	}
	g.AddNode(llmNode)

	toolsNode := graph.Node[AgentState]{
		ID:       "tools",
		Runnable: createToolsNode(spec.Name, resolvedTools, resolvedAgents, resolvedSkills),
		Router:   graph.StaticRouter[AgentState]("llm"),
	}
	g.AddNode(toolsNode)

	g.SetEntry("llm")

	return &CompiledAgent{
		name:          spec.Name,
		description:   spec.Description,
		graph:         g,
		inputSchema:   spec.InputSchema,
		outputSchema:  spec.OutputSchema,
		maxIterations: maxIter,
		agentNames:    spec.Agents,
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

// resolveAgentsTopological resolves agent names and detects cycles via BFS + Kahn's algorithm.
// Builds the full transitive dep graph so cycles like A→B→A are caught even when B
// was resolved independently. Returns the cycle path when a cycle is detected.
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

	// BFS: resolve all reachable agents transitively.
	agents := make(map[string]*CompiledAgent)
	visited := map[string]bool{parentAgent: true}
	queue := make([]string, len(agentNames))
	copy(queue, agentNames)

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if _, ok := agents[name]; ok {
			continue
		}

		a, err := resolver.Resolve(ctx, tenantID, name)
		if err != nil {
			return nil, nil, fmt.Errorf("agents not found: [%s]: %w", name, err)
		}
		agents[name] = a

		// Enqueue the sub-agent's declared deps (from its spec, even if not compiled).
		for _, sub := range a.AgentNames() {
			if !visited[sub] {
				visited[sub] = true
				queue = append(queue, sub)
			}
		}
	}

	// Build full dependency map including the parent.
	deps := make(map[string][]string, len(agents)+1)
	deps[parentAgent] = agentNames
	for name, a := range agents {
		deps[name] = a.AgentNames()
	}

	allNodes := make([]string, 0, len(agents)+1)
	allNodes = append(allNodes, parentAgent)
	for name := range agents {
		allNodes = append(allNodes, name)
	}

	cycle := kahnTopologicalSort(allNodes, deps)
	if cycle != nil {
		return nil, cycle, nil
	}

	return agents, nil, nil
}

// kahnTopologicalSort performs topological sort and detects cycles.
// Returns the cycle path if a cycle is detected, nil if the graph is acyclic.
func kahnTopologicalSort(nodes []string, edges map[string][]string) []string {
	inDegree := make(map[string]int)
	for _, node := range nodes {
		if _, ok := inDegree[node]; !ok {
			inDegree[node] = 0
		}
		for _, neighbor := range edges[node] {
			inDegree[neighbor]++
		}
	}

	queue := []string{}
	for _, node := range nodes {
		if inDegree[node] == 0 {
			queue = append(queue, node)
		}
	}

	sorted := []string{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		for _, neighbor := range edges[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) < len(nodes) {
		return findCycleDFS(nodes, edges)
	}

	return nil
}

// findCycleDFS performs DFS to find and return a cycle path.
// The returned slice starts and ends at the same node (closed cycle).
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
				// Found back-edge: extract the cycle from neighbor onward and close it.
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					path = append(path[cycleStart:], neighbor)
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
	return strings.Join(cycle, " → ")
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
	filters := []llm.Tags{llm.Tag("tier", tier)}
	for k, v := range tags {
		filters = append(filters, llm.Tag(k, v))
	}

	providers := registry.Select(filters...)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no LLM provider found for tier=%q, tags=%v", tier, tags)
	}

	return providers[0], nil
}

// createLLMNode creates the LLM invocation node.
// Skills with trigger=model are advertised as synthetic load_skill_<name> tools.
func createLLMNode(
	agentName string,
	systemPrompt *prompt.Fragment,
	provider llm.LLMProvider,
	resolvedTools map[string]tools.Tool,
	agents map[string]*CompiledAgent,
	skills map[string]SkillSpec,
) core.TypedRunnable[core.State[AgentState], core.State[AgentState]] {
	return core.TypedFunc(func(ctx context.Context, state core.State[AgentState]) (core.State[AgentState], error) {
		messages := []llm.Message{
			llm.NewTextMessage(llm.RoleSystem, systemPrompt.Content),
			llm.NewTextMessage(llm.RoleUser, state.Data.Task),
		}
		messages = append(messages, state.Data.Messages...)

		var toolDefs []llm.ToolDefinition
		for _, t := range resolvedTools {
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
		// Advertise model-triggered skills as synthetic load tools.
		// Full body is injected in createToolsNode when the LLM calls the tool.
		for _, s := range skills {
			toolDefs = append(toolDefs, llm.ToolDefinition{
				Name:        "load_skill_" + s.SkillName(),
				Description: "Load the " + s.SkillName() + " skill into context.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			})
		}

		resp, err := provider.Complete(ctx, &llm.Request{
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return state, fmt.Errorf("agent %q LLM call: %w", agentName, err)
		}

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
// Handles tool calls, agent-as-tool delegation, and load_skill_<name> calls.
// Enforces the runtime agent-depth guard and propagates Meta to sub-agents.
func createToolsNode(
	agentName string,
	resolvedTools map[string]tools.Tool,
	agents map[string]*CompiledAgent,
	skills map[string]SkillSpec,
) core.TypedRunnable[core.State[AgentState], core.State[AgentState]] {
	return core.TypedFunc(func(ctx context.Context, state core.State[AgentState]) (core.State[AgentState], error) {
		if len(state.Data.Messages) == 0 {
			return state, nil
		}

		lastMsg := state.Data.Messages[len(state.Data.Messages)-1]
		if lastMsg.Role != llm.RoleAssistant {
			return state, nil
		}

		var toolCalls []llm.ToolCall
		for _, part := range lastMsg.Parts {
			if part.Type == llm.PartTypeToolCall && part.ToolCall != nil {
				toolCalls = append(toolCalls, *part.ToolCall)
			}
		}

		if len(toolCalls) == 0 {
			return state, nil
		}

		var results []llm.ToolResult
		for _, tc := range toolCalls {
			output := executeToolCall(ctx, tc, resolvedTools, agents, skills, state, agentName)
			results = append(results, llm.ToolResult{
				ToolCallID: tc.ID,
				Content:    output,
			})
		}

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

// executeToolCall dispatches a single tool call and returns its string output.
func executeToolCall(
	ctx context.Context,
	tc llm.ToolCall,
	resolvedTools map[string]tools.Tool,
	agents map[string]*CompiledAgent,
	skills map[string]SkillSpec,
	state core.State[AgentState],
	agentName string,
) string {
	// Regular tool
	if tool, ok := resolvedTools[tc.Name]; ok {
		result, err := tool.Invoke(ctx, tc.Arguments)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		out, err := json.Marshal(result)
		if err != nil {
			return fmt.Sprintf("%v", result)
		}
		return string(out)
	}

	// Agent-as-tool delegation
	if agent, ok := agents[tc.Name]; ok {
		return invokeSubAgent(ctx, tc, agent, state)
	}

	// Skill load tool
	if strings.HasPrefix(tc.Name, "load_skill_") {
		skillName := strings.TrimPrefix(tc.Name, "load_skill_")
		if s, ok := skills[skillName]; ok {
			// Body injection requires PromptBackend access wired through registries.
			// For now return the skill name; full injection lands with PromptComposer integration.
			return fmt.Sprintf("Skill %q loaded. Name: %s", skillName, s.SkillName())
		}
		return fmt.Sprintf("skill %q not found", skillName)
	}

	return fmt.Sprintf("tool %q not found", tc.Name)
}

// invokeSubAgent delegates a tool call to a nested agent, enforcing the depth guard
// and propagating Meta keys (trace, cost, tenant, depth) into the sub-agent's state.
func invokeSubAgent(
	ctx context.Context,
	tc llm.ToolCall,
	agent *CompiledAgent,
	state core.State[AgentState],
) string {
	// Depth guard
	parentDepth := 0
	if d, ok := state.Meta["bunshin.agent_depth"].(int); ok {
		parentDepth = d
	}
	if parentDepth >= maxAgentDepth {
		return fmt.Sprintf("error: agent depth limit (%d) exceeded", maxAgentDepth)
	}

	// Parse args to extract task string and structured args
	taskStr := tc.Arguments
	args := map[string]any{}
	var argMap map[string]any
	if err := json.Unmarshal([]byte(tc.Arguments), &argMap); err == nil {
		if task, ok := argMap["task"].(string); ok {
			taskStr = task
			delete(argMap, "task")
			args = argMap
		} else {
			args = argMap
		}
	}

	subState := core.NewState(AgentState{
		Task:     taskStr,
		Args:     args,
		Messages: []llm.Message{},
	})

	// Propagate Meta: depth, trace, cost, tenant
	subState.Meta["bunshin.agent_depth"] = parentDepth + 1
	for _, k := range []string{
		"bunshin.tenant_id",
		"bunshin.trace_id",
		"bunshin.cost_budget",
	} {
		if v, ok := state.Meta[k]; ok {
			subState.Meta[k] = v
		}
	}

	result, err := agent.Invoke(ctx, subState)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	if resultState, ok := result.(core.State[AgentState]); ok {
		if len(resultState.Data.Messages) > 0 {
			return resultState.Data.Messages[len(resultState.Data.Messages)-1].Text()
		}
	}
	return ""
}

// createContentBasedRouter routes based on tool calls in the last message.
// Closes over maxIterations: after that many LLM turns, routes to END and sets
// Meta["bunshin.agent_truncated"] = true instead of erroring.
func createContentBasedRouter(maxIterations int) graph.Router[AgentState] {
	step := 0
	return func(ctx context.Context, state core.State[AgentState]) (string, error) {
		step++

		if len(state.Data.Messages) == 0 {
			return graph.END, nil
		}

		lastMsg := state.Data.Messages[len(state.Data.Messages)-1]
		if lastMsg.Role != llm.RoleAssistant {
			return graph.END, nil
		}

		// Truncate on cap: return last content rather than erroring.
		if step >= maxIterations {
			if state.Meta != nil {
				state.Meta["bunshin.agent_truncated"] = true
			}
			return graph.END, nil
		}

		for _, part := range lastMsg.Parts {
			if part.Type == llm.PartTypeToolCall && part.ToolCall != nil {
				return "tools", nil
			}
		}

		return graph.END, nil
	}
}
