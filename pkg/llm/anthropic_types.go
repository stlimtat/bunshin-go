package llm

// anthropicWireMessageItem is a single message in the Anthropic messages array.
// Content is a plain string (text only). Multi-modal messages require a
// []ContentBlock type that is not yet implemented.
type anthropicWireMessageItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicWireRequest is the JSON body sent to POST /v1/messages.
type anthropicWireRequest struct {
	Model     string                     `json:"model"`
	MaxTokens int                        `json:"max_tokens"`
	System    string                     `json:"system,omitempty"`
	Messages  []anthropicWireMessageItem `json:"messages"`
	Stream    bool                       `json:"stream,omitempty"`
}

// anthropicWireResponse is the JSON body returned by POST /v1/messages.
type anthropicWireResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

// anthropicWireStreamEvent is one SSE event payload during a streaming response.
type anthropicWireStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
