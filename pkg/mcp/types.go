package mcp

import "github.com/stlimtat/bunshin-go/pkg/llm"

// Resource is an MCP resource — a named data source the LLM can read.
type Resource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

// ResourceContent is the resolved content of an MCP resource.
type ResourceContent struct {
	URI      string
	MimeType string
	Text     string
	Blob     []byte
}

// MCPPrompt is an MCP prompt definition — a reusable prompt template.
type MCPPrompt struct {
	Name        string
	Description string
	Arguments   []PromptArgument
}

// PromptArgument describes one input variable of an MCP prompt.
type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}

// PromptMessage is one rendered message from an MCP prompt.
type PromptMessage struct {
	Role    llm.Role
	Content string
}
