package llm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func TestNewAnthropicProvider_MissingAPIKey(t *testing.T) {
	_, err := NewAnthropicProvider(AnthropicConfig{})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestAnthropicProvider_ID(t *testing.T) {
	p, err := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID() != ProviderAnthropic {
		t.Errorf("expected %q, got %q", ProviderAnthropic, p.ID())
	}
}

func TestAnthropicProvider_CanTransferContext(t *testing.T) {
	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})
	same, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "other"})
	fake := NewFakeProvider(ProviderFake, "")

	if !p.CanTransferContext(same) {
		t.Error("expected true for same provider")
	}
	if p.CanTransferContext(fake) {
		t.Error("expected false for different provider")
	}
}

func TestAnthropicProvider_TierModelSelection(t *testing.T) {
	cases := []struct {
		tier  ModelTier
		model ModelID
	}{
		{TierFast, "claude-haiku-4-5-20251001"},
		{TierSmart, "claude-sonnet-4-6"},
		{TierReasoning, "claude-opus-4-7"},
	}
	for _, tc := range cases {
		p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test", Tier: tc.tier})
		if p.model != tc.model {
			t.Errorf("tier %s: expected model %s, got %s", tc.tier, tc.model, p.model)
		}
	}
}

func TestAnthropicProvider_Complete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "testkey" {
			t.Errorf("unexpected api key: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("unexpected version header: %s", r.Header.Get("anthropic-version"))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"content":[{"type":"text","text":"hello from claude"}],"usage":{"input_tokens":10,"output_tokens":8},"model":"claude-haiku-4-5-20251001"}`)
	}))
	defer srv.Close()

	p, err := NewAnthropicProvider(AnthropicConfig{
		APIKey:  "testkey",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider: %v", err)
	}

	resp, err := p.Complete(context.Background(), &Request{
		Messages: []Message{
			NewTextMessage(RoleSystem, "Be helpful"),
			NewTextMessage(RoleUser, "hi"),
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello from claude" {
		t.Errorf("expected 'hello from claude', got %q", resp.Content)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 8 {
		t.Errorf("expected 8 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
}

func TestAnthropicProvider_Complete_AuthError(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "bad", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || err.Error() != "anthropic: authentication failed" {
		t.Errorf("expected auth error, got %v", err)
	}
}

func TestAnthropicProvider_Complete_RateLimit(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || err.Error() != "anthropic: rate limit exceeded" {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

func TestAnthropicProvider_Complete_ServerError(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil {
		t.Fatal("expected server error")
	}
}

func TestAnthropicProvider_StreamComplete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}`)
		fmt.Fprintln(w, `data: {"type":"message_delta","usage":{"input_tokens":5,"output_tokens":3}}`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "key", BaseURL: srv.URL})
	ch, err := p.StreamComplete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	var result string
	var done bool
	var finalUsage *TokenUsage
	for chunk := range ch {
		if chunk.Done {
			done = true
			finalUsage = chunk.Usage
		} else {
			result += chunk.Delta
		}
	}

	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
	if !done {
		t.Error("expected Done chunk")
	}
	if finalUsage == nil {
		t.Error("expected usage in final chunk")
	}
}

func TestAnthropicProvider_Ping(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path for ping: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"content":[{"type":"text","text":"."}],"usage":{"input_tokens":1,"output_tokens":1},"model":"claude-haiku-4-5-20251001"}`)
	}))
	defer srv.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "key", BaseURL: srv.URL})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAnthropicProvider_Ping_Failure(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "bad", BaseURL: srv.URL})
	if err := p.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

func TestAnthropicProvider_NativeMessages(t *testing.T) {
	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "key"})
	msgs := []Message{
		NewTextMessage(RoleSystem, "You are helpful"),
		NewTextMessage(RoleUser, "Hello"),
		NewTextMessage(RoleAssistant, "Hi there"),
	}
	native, err := p.NativeMessages(msgs)
	if err != nil {
		t.Fatalf("NativeMessages: %v", err)
	}
	wire, ok := native.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", native)
	}
	// System message should be excluded
	if len(wire) != 2 {
		t.Errorf("expected 2 non-system messages, got %d", len(wire))
	}
}

// Integration tests — skipped unless env var is set.

func TestAnthropicProvider_Integration(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: key, Tier: TierFast})
	resp, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "Reply with exactly: hello")},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("empty response")
	}
}

func TestAnthropicProvider_Ping_Integration(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	p, _ := NewAnthropicProvider(AnthropicConfig{APIKey: key})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
