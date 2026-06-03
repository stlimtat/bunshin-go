package llm

// openAIWireMessage is the JSON message structure used by the OpenAI and
// Azure OpenAI Chat Completions API.
// Content is a plain string (text only). Vision/multi-modal messages require a
// []ContentPart array type that is not yet implemented.
type openAIWireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIWireRequest is the JSON body sent to POST /v1/chat/completions (OpenAI)
// or the Azure OpenAI Chat Completions endpoint.
type openAIWireRequest struct {
	Model     string              `json:"model,omitempty"`
	Messages  []openAIWireMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
	Stream    bool                `json:"stream,omitempty"`
	// Temperature is omitted when nil to let the API use its default.
	Temperature *float64 `json:"temperature,omitempty"`
}

// openAIWireResponse is the JSON body returned by POST /v1/chat/completions
// (non-streaming).
type openAIWireResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Model string `json:"model"`
}

// openAIWireStreamChunk is one SSE payload during a streaming response.
type openAIWireStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}
