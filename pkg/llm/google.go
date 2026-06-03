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

// GoogleConfig holds configuration for the Google Gemini API provider.
type GoogleConfig struct {
	// APIKey is the Google AI API key. Required.
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

// GoogleProvider implements LLMProvider and Pinger for the Google Gemini API.
type GoogleProvider struct {
	cfg    GoogleConfig
	model  ModelID
	client *http.Client
}

// googleTierModels maps ModelTier to the default Gemini model ID.
var googleTierModels = map[ModelTier]ModelID{
	TierFast:      "gemini-2.0-flash-lite",
	TierSmart:     "gemini-2.0-flash",
	TierReasoning: "gemini-2.5-pro",
}

// NewGoogleProvider constructs a GoogleProvider. Returns an error if APIKey is empty.
func NewGoogleProvider(cfg GoogleConfig) (*GoogleProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("google: APIKey is required")
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1024
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com"
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	model := cfg.Model
	if model == "" {
		if m, ok := googleTierModels[cfg.Tier]; ok {
			model = m
		} else {
			model = googleTierModels[TierFast]
		}
	}

	return &GoogleProvider{
		cfg:   cfg,
		model: model,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// ID returns the Google provider identifier.
func (p *GoogleProvider) ID() ProviderID { return ProviderGoogle }

// CanTransferContext returns true when from is also a Google provider.
func (p *GoogleProvider) CanTransferContext(from LLMProvider) bool {
	return from.ID() == ProviderGoogle
}

// NativeMessages converts canonical messages into the Google Gemini wire format.
// System messages are excluded; they go in systemInstruction.
func (p *GoogleProvider) NativeMessages(msgs []Message) (any, error) {
	wire := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleSystem {
			continue
		}
		role := googleRole(m.Role)
		wire = append(wire, map[string]any{
			"role": role,
			"parts": []map[string]any{
				{"text": m.Text()},
			},
		})
	}
	return wire, nil
}

// Complete sends a non-streaming request to Google Gemini and returns the full response.
func (p *GoogleProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	wireReq := p.buildWireRequest(req)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("google: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.cfg.BaseURL, p.model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google: http: %w", err)
	}
	defer resp.Body.Close()

	if err := p.checkStatus(resp); err != nil {
		return nil, err
	}

	var wireResp googleWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wireResp); err != nil {
		return nil, fmt.Errorf("google: decode response: %w", err)
	}

	content := ""
	if len(wireResp.Candidates) > 0 {
		for _, part := range wireResp.Candidates[0].Content.Parts {
			content += part.Text
		}
	}

	return &Response{
		Content: content,
		Model:   p.model,
		Usage: TokenUsage{
			PromptTokens:     wireResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: wireResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      wireResp.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

// StreamComplete sends a streaming request to Google Gemini and returns a channel of chunks.
func (p *GoogleProvider) StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error) {
	wireReq := p.buildWireRequest(req)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("google: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", p.cfg.BaseURL, p.model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-goog-api-key", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google: http: %w", err)
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
func (p *GoogleProvider) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1beta/models", p.cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("google: ping build request: %w", err)
	}
	httpReq.Header.Set("x-goog-api-key", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("google: ping: %w", err)
	}
	defer resp.Body.Close()
	return p.checkStatus(resp)
}

// buildWireRequest converts a canonical Request to the Gemini wire format.
func (p *GoogleProvider) buildWireRequest(req *Request) *googleWireRequest {
	wireReq := &googleWireRequest{
		GenerationConfig: &googleGenConfig{
			MaxOutputTokens: p.cfg.MaxTokens,
		},
	}
	if req.MaxTokens != nil {
		wireReq.GenerationConfig.MaxOutputTokens = *req.MaxTokens
	}
	if req.Temperature != nil {
		wireReq.GenerationConfig.Temperature = req.Temperature
	}

	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			if wireReq.SystemInstruction == nil {
				wireReq.SystemInstruction = &googleWireContent{}
			}
			wireReq.SystemInstruction.Parts = append(wireReq.SystemInstruction.Parts,
				googleWirePart{Text: m.Text()})
			continue
		}
		wireReq.Contents = append(wireReq.Contents, googleWireContent{
			Role:  googleRole(m.Role),
			Parts: []googleWirePart{{Text: m.Text()}},
		})
	}
	return wireReq
}

// googleRole maps canonical Role to the Gemini wire role string.
func googleRole(r Role) string {
	switch r {
	case RoleAssistant:
		return "model"
	default:
		return "user"
	}
}

// checkStatus maps HTTP error codes to descriptive errors.
func (p *GoogleProvider) checkStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("google: authentication failed")
	case http.StatusTooManyRequests:
		return fmt.Errorf("google: rate limit exceeded")
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("google: server error: %s", resp.Status)
		}
		return fmt.Errorf("google: request failed: %s", string(body))
	}
}

// readSSEStream parses Gemini SSE lines and sends Chunks on ch.
func (p *GoogleProvider) readSSEStream(r io.Reader, ch chan<- Chunk) {
	scanner := bufio.NewScanner(r)
	var finalUsage *TokenUsage
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		var wireResp googleWireResponse
		if err := json.Unmarshal([]byte(payload), &wireResp); err != nil {
			continue
		}

		for _, candidate := range wireResp.Candidates {
			var sb strings.Builder
			for _, part := range candidate.Content.Parts {
				sb.WriteString(part.Text)
			}
			if sb.Len() > 0 {
				ch <- Chunk{Delta: sb.String()}
			}
		}

		if wireResp.UsageMetadata.TotalTokenCount > 0 {
			finalUsage = &TokenUsage{
				PromptTokens:     wireResp.UsageMetadata.PromptTokenCount,
				CompletionTokens: wireResp.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      wireResp.UsageMetadata.TotalTokenCount,
			}
		}
	}
	ch <- Chunk{Done: true, Usage: finalUsage}
}
