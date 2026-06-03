package llm

// Role identifies the author of a message in a conversation.
type Role string

const (
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
	RoleUser      Role = "user"
)

// ContentPartType identifies the kind of content in a ContentPart.
type ContentPartType string

const (
	PartTypeImageURL   ContentPartType = "image_url"
	PartTypeText       ContentPartType = "text"
	PartTypeToolCall   ContentPartType = "tool_call"
	PartTypeToolResult ContentPartType = "tool_result"
)

// ContentPart is one element of a multi-modal message.
type ContentPart struct {
	Type ContentPartType

	// Text is set when Type == PartTypeText.
	Text string

	// ImageURL is set when Type == PartTypeImageURL.
	ImageURL string

	// ToolCall is set when Type == PartTypeToolCall.
	ToolCall *ToolCall

	// ToolResult is set when Type == PartTypeToolResult.
	ToolResult *ToolResult
}

// ToolCall describes a function call the LLM wants to make.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON-encoded arguments
}

// ToolResult carries the output of a tool call back to the LLM.
type ToolResult struct {
	ToolCallID string
	Content    string
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string
	Description string
	// Parameters is a JSON Schema object describing the tool's input.
	Parameters map[string]any
}

// Request is the canonical input to an LLM call.
type Request struct {
	Messages    []Message
	Temperature *float64
	MaxTokens   *int
	// Tools lists the tools available to the model for this call.
	Tools []ToolDefinition
	// Stream indicates the caller wants a streaming response.
	Stream bool
}

// Response is the canonical output from an LLM call.
type Response struct {
	// Content holds the model's text reply (empty if the response is a tool call).
	Content string
	// ToolCalls holds any function calls the model wants to make.
	ToolCalls []ToolCall
	// Usage reports token consumption for cost tracking.
	Usage TokenUsage
	// Model is the model ID that produced this response.
	Model ModelID
}

// TokenUsage tracks token consumption for a single LLM call.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// CachedTokens counts prompt tokens served from the provider's prompt cache.
	CachedTokens int
}

// Chunk is one piece of a streaming response.
type Chunk struct {
	// Delta is the new text fragment in this chunk.
	Delta string
	// ToolCallDelta is a partial tool call update.
	ToolCallDelta *ToolCall
	// Done is true on the final chunk.
	Done bool
	// Usage is populated on the final chunk.
	Usage *TokenUsage
}
