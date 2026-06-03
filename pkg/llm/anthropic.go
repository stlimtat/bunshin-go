package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicConfig holds configuration for the Anthropic Messages API provider.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key. Required.
	APIKey string
	// Model overrides tier-based model selection when non-empty.
	Model ModelID
	// Tier selects a default model when Model is empty.
	Tier ModelTier
	// MaxTokens caps the completion length. Defaults to 1024.
	MaxTokens int
	// BaseURL overrides the default API base for testing or proxies.
	BaseURL string
	// HTTPTimeout overrides the default 30s request timeout.
	HTTPTimeout time.Duration
}

// AnthropicProvider implements LLMProvider and Pinger for the Anthropic API.
type AnthropicProvider struct {
	cfg    AnthropicConfig
	model  ModelID
	client *http.Client
}

// anthropicTierModels maps ModelTier to the default Anthropic model ID.
var anthropicTierModels = map[ModelTier]ModelID{
	TierFast:      "claude-haiku-4-5-20251001",
	TierSmart:     "claude-sonnet-4-6",
	TierReasoning: "claude-opus-4-7",
}

const anthropicVersion = "2023-06-01"

// NewAnthropicProvider constructs an AnthropicProvider. Returns an error if APIKey is empty.
func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: APIKey is required")
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1024
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	model := cfg.Model
	if model == "" {
		if m, ok := anthropicTierModels[cfg.Tier]; ok {
			model = m
		} else {
			model = anthropicTierModels[TierFast]
		}
	}

	return &AnthropicProvider{
		cfg:   cfg,
		model: model,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// ID returns the Anthropic provider identifier.
func (p *AnthropicProvider) ID() ProviderID { return ProviderAnthropic }

// CanTransferContext returns true when from is also an Anthropic provider.
func (p *AnthropicProvider) CanTransferContext(from LLMProvider) bool {
	return from.ID() == ProviderAnthropic
}

// NativeMessages converts canonical messages into the Anthropic wire format.
// System messages are excluded here; they go in the top-level system field.
func (p *AnthropicProvider) NativeMessages(msgs []Message) (any, error) {
	wire := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleSystem {
			continue
		}
		wire = append(wire, map[string]any{
			"role":    string(m.Role),
			"content": m.Text(),
		})
	}
	return wire, nil
}

// Complete sends a non-streaming request to Anthropic and returns the full response.
func (p *AnthropicProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	wireReq, _ := p.buildWireRequest(req, false)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	if err := p.checkStatus(resp); err != nil {
		return nil, err
	}

	var wireResp anthropicWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wireResp); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}

	content := ""
	for _, block := range wireResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &Response{
		Content: content,
		Model:   ModelID(wireResp.Model),
		Usage: TokenUsage{
			PromptTokens:     wireResp.Usage.InputTokens,
			CompletionTokens: wireResp.Usage.OutputTokens,
			TotalTokens:      wireResp.Usage.InputTokens + wireResp.Usage.OutputTokens,
		},
	}, nil
}

// StreamComplete sends a streaming request to Anthropic and returns a channel of chunks.
func (p *AnthropicProvider) StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error) {
	wireReq, _ := p.buildWireRequest(req, true)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	p.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http: %w", err)
	}
	if err := p.checkStatus(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan Chunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.readSSEStream(resp.Body, ch)
	}()
	return ch, nil
}

// Ping verifies connectivity by listing available models (cost-free GET request).
func (p *AnthropicProvider) Ping(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.cfg.BaseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("anthropic: ping build request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("anthropic: ping: %w", err)
	}
	defer resp.Body.Close()
	return p.checkStatus(resp)
}

// anthropicWireMessageItem is a single message in the Anthropic messages array.
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

// buildWireRequest converts a canonical Request to the Anthropic wire format.
// Returns the wire request and extracted system text.
func (p *AnthropicProvider) buildWireRequest(req *Request, stream bool) (*anthropicWireRequest, string) {
	maxTokens := p.cfg.MaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	wireReq := &anthropicWireRequest{
		Model:     string(p.model),
		MaxTokens: maxTokens,
		Stream:    stream,
	}

	var systemText string
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemText += m.Text()
			continue
		}
		wireReq.Messages = append(wireReq.Messages, anthropicWireMessageItem{
			Role:    string(m.Role),
			Content: m.Text(),
		})
	}
	wireReq.System = systemText
	return wireReq, systemText
}

// setHeaders attaches Anthropic-specific auth and versioning headers.
func (p *AnthropicProvider) setHeaders(r *http.Request) {
	r.Header.Set("x-api-key", p.cfg.APIKey)
	r.Header.Set("anthropic-version", anthropicVersion)
	r.Header.Set("Content-Type", "application/json")
}

// checkStatus maps HTTP error codes to descriptive errors.
func (p *AnthropicProvider) checkStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("anthropic: authentication failed")
	case http.StatusTooManyRequests:
		return fmt.Errorf("anthropic: rate limit exceeded")
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("anthropic: server error: %s", resp.Status)
		}
		return fmt.Errorf("anthropic: request failed: %s", string(body))
	}
}

// readSSEStream parses Anthropic SSE lines and sends Chunks on ch.
func (p *AnthropicProvider) readSSEStream(r io.Reader, ch chan<- Chunk) {
	scanner := bufio.NewScanner(r)
	var finalUsage *TokenUsage
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		var event anthropicWireStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Type == "text_delta" {
				ch <- Chunk{Delta: event.Delta.Text}
			}
		case "message_delta":
			if event.Usage != nil {
				finalUsage = &TokenUsage{
					PromptTokens:     event.Usage.InputTokens,
					CompletionTokens: event.Usage.OutputTokens,
					TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
				}
			}
		case "message_stop":
			ch <- Chunk{Done: true, Usage: finalUsage}
			return
		}
	}
	// Stream ended without message_stop.
	ch <- Chunk{Done: true, Usage: finalUsage}
}
