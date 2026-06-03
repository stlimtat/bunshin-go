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

// OpenAIConfig holds configuration for the OpenAI Chat Completions provider.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key. Required.
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

// OpenAIProvider implements LLMProvider and Pinger for the OpenAI API.
type OpenAIProvider struct {
	cfg    OpenAIConfig
	model  ModelID
	client *http.Client
}

// openAITierModels maps ModelTier to the default OpenAI model ID.
var openAITierModels = map[ModelTier]ModelID{
	TierFast:      "gpt-4o-mini",
	TierSmart:     "gpt-4o",
	TierReasoning: "o3-mini",
}

// NewOpenAIProvider constructs an OpenAIProvider. Returns an error if APIKey is empty.
func NewOpenAIProvider(cfg OpenAIConfig) (*OpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: APIKey is required")
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1024
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	model := cfg.Model
	if model == "" {
		if m, ok := openAITierModels[cfg.Tier]; ok {
			model = m
		} else {
			model = openAITierModels[TierFast]
		}
	}

	return &OpenAIProvider{
		cfg:   cfg,
		model: model,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// ID returns the OpenAI provider identifier.
func (p *OpenAIProvider) ID() ProviderID { return ProviderOpenAI }

// CanTransferContext returns true when from is also an OpenAI provider.
func (p *OpenAIProvider) CanTransferContext(from LLMProvider) bool {
	return from.ID() == ProviderOpenAI
}

// NativeMessages converts canonical messages into the OpenAI wire format.
func (p *OpenAIProvider) NativeMessages(msgs []Message) (any, error) {
	wire := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		wire = append(wire, map[string]any{
			"role":    string(m.Role),
			"content": m.Text(),
		})
	}
	return wire, nil
}

// Complete sends a non-streaming request to OpenAI and returns the full response.
func (p *OpenAIProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	wireReq := p.buildWireRequest(req, false)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: http: %w", err)
	}
	defer resp.Body.Close()

	if err := p.checkStatus(resp); err != nil {
		return nil, err
	}

	var wireResp openAIWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wireResp); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}

	content := ""
	if len(wireResp.Choices) > 0 {
		content = wireResp.Choices[0].Message.Content
	}

	return &Response{
		Content: content,
		Model:   ModelID(wireResp.Model),
		Usage: TokenUsage{
			PromptTokens:     wireResp.Usage.PromptTokens,
			CompletionTokens: wireResp.Usage.CompletionTokens,
			TotalTokens:      wireResp.Usage.TotalTokens,
			CachedTokens:     wireResp.Usage.PromptTokensDetails.CachedTokens,
		},
	}, nil
}

// StreamComplete sends a streaming request to OpenAI and returns a channel of chunks.
func (p *OpenAIProvider) StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error) {
	wireReq := p.buildWireRequest(req, true)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	p.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: http: %w", err)
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

// Ping verifies connectivity by listing models.
func (p *OpenAIProvider) Ping(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("openai: ping build request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("openai: ping: %w", err)
	}
	defer resp.Body.Close()
	return p.checkStatus(resp)
}

// buildWireRequest converts a canonical Request to the OpenAI wire format.
func (p *OpenAIProvider) buildWireRequest(req *Request, stream bool) *openAIWireRequest {
	wireReq := &openAIWireRequest{
		Model:     string(p.model),
		MaxTokens: p.cfg.MaxTokens,
		Stream:    stream,
	}
	if req.MaxTokens != nil {
		wireReq.MaxTokens = *req.MaxTokens
	}
	if req.Temperature != nil {
		wireReq.Temperature = req.Temperature
	}

	for _, m := range req.Messages {
		wireReq.Messages = append(wireReq.Messages, openAIWireMessage{
			Role:    string(m.Role),
			Content: m.Text(),
		})
	}
	return wireReq
}

// setHeaders attaches auth and content-type headers.
func (p *OpenAIProvider) setHeaders(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	r.Header.Set("Content-Type", "application/json")
}

// checkStatus maps HTTP error codes to descriptive errors, including the response body.
func (p *OpenAIProvider) checkStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, readErr := io.ReadAll(resp.Body)
	detail := string(body)
	if readErr != nil {
		detail = fmt.Sprintf("<failed to read error body: %v>", readErr)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("openai: authentication failed: %s", detail)
	case http.StatusTooManyRequests:
		return fmt.Errorf("openai: rate limit exceeded: %s", detail)
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("openai: server error %s: %s", resp.Status, detail)
		}
		return fmt.Errorf("openai: request failed (%s): %s", resp.Status, detail)
	}
}

// readSSEStream parses OpenAI SSE lines and sends Chunks on ch.
func (p *OpenAIProvider) readSSEStream(r io.Reader, ch chan<- Chunk) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			ch <- Chunk{Done: true}
			return
		}

		var chunk openAIWireStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		c := Chunk{}
		if len(chunk.Choices) > 0 {
			c.Delta = chunk.Choices[0].Delta.Content
		}
		if chunk.Usage != nil {
			c.Usage = &TokenUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
				CachedTokens:     chunk.Usage.PromptTokensDetails.CachedTokens,
			}
		}
		ch <- c
	}
	if err := scanner.Err(); err != nil {
		ch <- Chunk{Done: true, Err: fmt.Errorf("openai: stream read: %w", err)}
		return
	}
	// Stream ended without [DONE] — send terminal chunk.
	ch <- Chunk{Done: true}
}
