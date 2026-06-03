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

// AzureOpenAIConfig holds configuration for the Azure OpenAI provider.
type AzureOpenAIConfig struct {
	// APIKey is the Azure OpenAI API key. Required.
	APIKey string
	// Endpoint is the Azure resource endpoint, e.g. "https://myresource.openai.azure.com". Required.
	Endpoint string
	// Deployment is the model deployment name. Required.
	Deployment string
	// APIVersion is the REST API version. Defaults to "2024-02-01".
	APIVersion string
	// MaxTokens caps the completion length. Defaults to 1024.
	MaxTokens int
	// HTTPTimeout overrides the default 30s request timeout.
	HTTPTimeout time.Duration
}

// AzureOpenAIProvider implements LLMProvider and Pinger for the Azure OpenAI API.
type AzureOpenAIProvider struct {
	cfg    AzureOpenAIConfig
	client *http.Client
}

// NewAzureOpenAIProvider constructs an AzureOpenAIProvider.
// Returns an error if APIKey, Endpoint, or Deployment are empty.
func NewAzureOpenAIProvider(cfg AzureOpenAIConfig) (*AzureOpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("azure-openai: APIKey is required")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("azure-openai: Endpoint is required")
	}
	if cfg.Deployment == "" {
		return nil, fmt.Errorf("azure-openai: Deployment is required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = "2024-02-01"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1024
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &AzureOpenAIProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// ID returns the Azure OpenAI provider identifier.
func (p *AzureOpenAIProvider) ID() ProviderID { return ProviderAzureOpenAI }

// CanTransferContext returns true when from is also an Azure OpenAI provider.
func (p *AzureOpenAIProvider) CanTransferContext(from LLMProvider) bool {
	return from.ID() == ProviderAzureOpenAI
}

// NativeMessages converts canonical messages into the OpenAI-compatible wire format.
func (p *AzureOpenAIProvider) NativeMessages(msgs []Message) (any, error) {
	wire := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		wire = append(wire, map[string]any{
			"role":    string(m.Role),
			"content": m.Text(),
		})
	}
	return wire, nil
}

// Complete sends a non-streaming request to Azure OpenAI and returns the full response.
func (p *AzureOpenAIProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	wireReq := p.buildWireRequest(req, false)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("azure-openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.completionsURL(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("azure-openai: build request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azure-openai: http: %w", err)
	}
	defer resp.Body.Close()

	if err := p.checkStatus(resp); err != nil {
		return nil, err
	}

	var wireResp openAIWireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wireResp); err != nil {
		return nil, fmt.Errorf("azure-openai: decode response: %w", err)
	}

	content := ""
	if len(wireResp.Choices) > 0 {
		content = wireResp.Choices[0].Message.Content
	}

	return &Response{
		Content: content,
		Model:   ModelID(p.cfg.Deployment),
		Usage: TokenUsage{
			PromptTokens:     wireResp.Usage.PromptTokens,
			CompletionTokens: wireResp.Usage.CompletionTokens,
			TotalTokens:      wireResp.Usage.TotalTokens,
			CachedTokens:     wireResp.Usage.PromptTokensDetails.CachedTokens,
		},
	}, nil
}

// StreamComplete sends a streaming request to Azure OpenAI and returns a channel of chunks.
func (p *AzureOpenAIProvider) StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error) {
	wireReq := p.buildWireRequest(req, true)
	body, err := json.Marshal(wireReq)
	if err != nil {
		return nil, fmt.Errorf("azure-openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.completionsURL(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("azure-openai: build request: %w", err)
	}
	p.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azure-openai: http: %w", err)
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
func (p *AzureOpenAIProvider) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/openai/models?api-version=%s",
		p.cfg.Endpoint, p.cfg.APIVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("azure-openai: ping build request: %w", err)
	}
	httpReq.Header.Set("api-key", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("azure-openai: ping: %w", err)
	}
	defer resp.Body.Close()
	return p.checkStatus(resp)
}

// completionsURL returns the Azure Chat Completions endpoint URL.
func (p *AzureOpenAIProvider) completionsURL() string {
	return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		p.cfg.Endpoint, p.cfg.Deployment, p.cfg.APIVersion)
}

// buildWireRequest converts a canonical Request to the OpenAI-compatible wire format.
// Note: model is omitted for Azure (deployment is encoded in the URL).
func (p *AzureOpenAIProvider) buildWireRequest(req *Request, stream bool) *openAIWireRequest {
	wireReq := &openAIWireRequest{
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

// setHeaders attaches Azure auth and content-type headers.
func (p *AzureOpenAIProvider) setHeaders(r *http.Request) {
	r.Header.Set("api-key", p.cfg.APIKey)
	r.Header.Set("Content-Type", "application/json")
}

// checkStatus maps HTTP error codes to descriptive errors, including the response body.
func (p *AzureOpenAIProvider) checkStatus(resp *http.Response) error {
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
		return fmt.Errorf("azure-openai: authentication failed: %s", detail)
	case http.StatusTooManyRequests:
		return fmt.Errorf("azure-openai: rate limit exceeded: %s", detail)
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("azure-openai: server error %s: %s", resp.Status, detail)
		}
		return fmt.Errorf("azure-openai: request failed (%s): %s", resp.Status, detail)
	}
}

// readSSEStream parses OpenAI-compatible SSE lines and sends Chunks on ch.
func (p *AzureOpenAIProvider) readSSEStream(r io.Reader, ch chan<- Chunk) {
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
		ch <- Chunk{Done: true, Err: fmt.Errorf("azure-openai: stream read: %w", err)}
		return
	}
	ch <- Chunk{Done: true}
}
