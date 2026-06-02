package tools

// ToolSchema describes a tool's identity and input contract.
// Parameters is a JSON Schema object — same format used by OpenAI and Anthropic
// function-calling APIs, so no translation is needed when building LLM requests.
type ToolSchema struct {
	Name        string
	Description string
	// Parameters is a JSON Schema describing the tool's input object.
	Parameters map[string]any
}
