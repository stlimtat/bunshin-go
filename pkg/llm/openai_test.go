package llm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func TestNewOpenAIProvider_MissingAPIKey(t *testing.T) {
	_, err := NewOpenAIProvider(OpenAIConfig{})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestOpenAIProvider_ID(t *testing.T) {
	p, err := NewOpenAIProvider(OpenAIConfig{APIKey: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID() != ProviderOpenAI {
		t.Errorf("expected %q, got %q", ProviderOpenAI, p.ID())
	}
}

func TestOpenAIProvider_CanTransferContext(t *testing.T) {
	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "test"})
	same, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "other"})
	fake := NewFakeProvider(ProviderFake, "")

	if !p.CanTransferContext(same) {
		t.Error("expected true for same provider")
	}
	if p.CanTransferContext(fake) {
		t.Error("expected false for different provider")
	}
}

func TestOpenAIProvider_TierModelSelection(t *testing.T) {
	cases := []struct {
		tier  ModelTier
		model ModelID
	}{
		{TierFast, "gpt-4o-mini"},
		{TierSmart, "gpt-4o"},
		{TierReasoning, "o3-mini"},
	}
	for _, tc := range cases {
		p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "test", Tier: tc.tier})
		if p.model != tc.model {
			t.Errorf("tier %s: expected model %s, got %s", tc.tier, tc.model, p.model)
		}
	}
}

func TestOpenAIProvider_Complete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices":[{"message":{"content":"hello world"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15},"model":"gpt-4o-mini"}`)
	}))
	defer srv.Close()

	p, err := NewOpenAIProvider(OpenAIConfig{
		APIKey:  "testkey",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	resp, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello world" {
		t.Errorf("expected 'hello world', got %q", resp.Content)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
}

func TestOpenAIProvider_Complete_AuthError(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "bad", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if err.Error() != "openai: authentication failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenAIProvider_Complete_RateLimit(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || err.Error() != "openai: rate limit exceeded" {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

func TestOpenAIProvider_Complete_ServerError(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil {
		t.Fatal("expected server error")
	}
}

func TestOpenAIProvider_StreamComplete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hello"}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":" world"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: srv.URL})
	ch, err := p.StreamComplete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	var result string
	var done bool
	for chunk := range ch {
		if chunk.Done {
			done = true
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
}

func TestOpenAIProvider_Ping(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path for ping: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":[]}`)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "key", BaseURL: srv.URL})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenAIProvider_Ping_Failure(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "bad", BaseURL: srv.URL})
	if err := p.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

func TestOpenAIProvider_NativeMessages(t *testing.T) {
	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: "key"})
	msgs := []Message{
		NewTextMessage(RoleSystem, "You are helpful"),
		NewTextMessage(RoleUser, "Hello"),
	}
	native, err := p.NativeMessages(msgs)
	if err != nil {
		t.Fatalf("NativeMessages: %v", err)
	}
	wire, ok := native.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", native)
	}
	if len(wire) != 2 {
		t.Errorf("expected 2 messages, got %d", len(wire))
	}
}

// Integration tests — skipped unless env var is set.

func TestOpenAIProvider_Integration(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: key, Tier: TierFast})
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

func TestOpenAIProvider_Ping_Integration(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	p, _ := NewOpenAIProvider(OpenAIConfig{APIKey: key})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
