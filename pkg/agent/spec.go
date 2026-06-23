package agent

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// AgentSpec is the declarative definition of an Agent.
// It mirrors WorkflowSpec: an agent references a Fragment for its system prompt,
// specifies allowed tools/agents/skills, model tier, and iteration constraints.
//
// Agents are compiled at load time to CompiledAgent; all references are resolved
// against the provided Registries, surfacing missing refs as compile errors.
type AgentSpec struct {
	// Name is the agent identifier, tenant-unique.
	Name string `yaml:"name"`

	// Description is a human-readable explanation of the agent's purpose.
	Description string `yaml:"description"`

	// SystemPrompt references the Fragment slug containing the agent's system instructions.
	SystemPrompt struct {
		Slug string `yaml:"slug"`
	} `yaml:"system_prompt"`

	// Tools is an allowlist of tool names resolved at compile time against ToolRegistry.
	Tools []string `yaml:"tools"`

	// Agents is an allowlist of agent names (subagent delegation).
	// Resolved at compile time against AgentResolver; subject to topological
	// compile and cycle detection.
	Agents []string `yaml:"agents"`

	// Skills is an allowlist of skill names (injected capabilities).
	// Resolved at compile time against SkillResolver.
	Skills []string `yaml:"skills"`

	// Model specifies the LLM provider and tier for this agent.
	Model struct {
		// Tier selects a model tier: fast, smart, reasoning.
		Tier string `yaml:"tier"`
		// Tags are additional selection filters (budget, region, etc.).
		Tags map[string]string `yaml:"tags"`
	} `yaml:"model"`

	// MaxIterations is the cap on agent loop iterations before truncating and returning.
	// Defaults to 8. Exceeded iterations set Meta["bunshin.agent_truncated"] = true.
	MaxIterations int `yaml:"max_iterations"`

	// InputSchema is an optional JSON Schema validating the input task + args.
	// Enforced at Compile time and at Invoke time.
	InputSchema map[string]any `yaml:"input_schema"`

	// OutputSchema is an optional JSON Schema forcing structured final output.
	// Enforced at Compile time; triggers a final structured turn if provided.
	OutputSchema map[string]any `yaml:"output_schema"`
}

// Parse unmarshals an AgentSpec from YAML bytes.
func Parse(data []byte) (*AgentSpec, error) {
	var spec AgentSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("agent: parse YAML: %w", err)
	}

	// Validate required fields
	if spec.Name == "" {
		return nil, fmt.Errorf("agent: spec.name is required")
	}
	if spec.SystemPrompt.Slug == "" {
		return nil, fmt.Errorf("agent: spec.system_prompt.slug is required")
	}

	// Apply defaults
	if spec.MaxIterations == 0 {
		spec.MaxIterations = 8
	}

	return &spec, nil
}
