package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestNewGoogleProvider_MissingAPIKey(t *testing.T) {
	_, err := NewGoogleProvider(GoogleConfig{})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestGoogleProvider_ID(t *testing.T) {
	p, err := NewGoogleProvider(GoogleConfig{APIKey: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID() != ProviderGoogle {
		t.Errorf("expected %q, got %q", ProviderGoogle, p.ID())
	}
}

func TestGoogleProvider_CanTransferContext(t *testing.T) {
	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "test"})
	same, _ := NewGoogleProvider(GoogleConfig{APIKey: "other"})
	fake := NewFakeProvider(ProviderFake, "")

	if !p.CanTransferContext(same) {
		t.Error("expected true for same provider")
	}
	if p.CanTransferContext(fake) {
		t.Error("expected false for different provider")
	}
}

func TestGoogleProvider_TierModelSelection(t *testing.T) {
	cases := []struct {
		tier  ModelTier
		model ModelID
	}{
		{TierFast, "gemini-2.0-flash-lite"},
		{TierSmart, "gemini-2.0-flash"},
		{TierReasoning, "gemini-2.5-pro"},
	}
	for _, tc := range cases {
		p, _ := NewGoogleProvider(GoogleConfig{APIKey: "test", Tier: tc.tier})
		if p.model != tc.model {
			t.Errorf("tier %s: expected model %s, got %s", tc.tier, tc.model, p.model)
		}
	}
}

func TestGoogleProvider_Complete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "generateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "testkey" {
			t.Errorf("unexpected api key header: %s", r.Header.Get("x-goog-api-key"))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"candidates":[{"content":{"parts":[{"text":"hello from gemini"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":6,"totalTokenCount":16}}`)
	}))
	defer srv.Close()

	p, err := NewGoogleProvider(GoogleConfig{
		APIKey:  "testkey",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewGoogleProvider: %v", err)
	}

	resp, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello from gemini" {
		t.Errorf("expected 'hello from gemini', got %q", resp.Content)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
}

func TestGoogleProvider_Complete_SystemInstruction(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		// Verify system instruction is present
		if _, ok := body["systemInstruction"]; !ok {
			t.Error("expected systemInstruction in request body")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"}}]}`)
	}))
	defer srv.Close()

	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{
			NewTextMessage(RoleSystem, "Be helpful"),
			NewTextMessage(RoleUser, "hi"),
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestGoogleProvider_Complete_AuthError(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "bad", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || !strings.HasPrefix(err.Error(), "google: authentication failed") {
		t.Errorf("expected auth error, got %v", err)
	}
}

func TestGoogleProvider_Complete_RateLimit(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "key", BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || !strings.HasPrefix(err.Error(), "google: rate limit exceeded") {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

func TestGoogleProvider_StreamComplete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "streamGenerateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		fmt.Fprintln(w, `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}]}`)
		fmt.Fprintln(w, `data: {"candidates":[{"content":{"parts":[{"text":" world"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`)

		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "key", BaseURL: srv.URL})
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

func TestGoogleProvider_Ping(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1beta/models") {
			t.Errorf("unexpected path for ping: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "testkey" {
			t.Errorf("unexpected api key header: %s", r.Header.Get("x-goog-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"models":[]}`)
	}))
	defer srv.Close()

	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "testkey", BaseURL: srv.URL})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestGoogleProvider_Ping_Failure(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "bad", BaseURL: srv.URL})
	if err := p.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

func TestGoogleProvider_NativeMessages(t *testing.T) {
	p, _ := NewGoogleProvider(GoogleConfig{APIKey: "key"})
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
	// Assistant should be mapped to "model"
	if wire[1]["role"] != "model" {
		t.Errorf("expected role 'model' for assistant, got %v", wire[1]["role"])
	}
}

// Integration tests — skipped unless env var is set.

func TestGoogleProvider_Integration(t *testing.T) {
	key := os.Getenv("GOOGLE_AI_API_KEY")
	if key == "" {
		t.Skip("GOOGLE_AI_API_KEY not set")
	}
	p, _ := NewGoogleProvider(GoogleConfig{APIKey: key, Tier: TierFast})
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

func TestGoogleProvider_Ping_Integration(t *testing.T) {
	key := os.Getenv("GOOGLE_AI_API_KEY")
	if key == "" {
		t.Skip("GOOGLE_AI_API_KEY not set")
	}
	p, _ := NewGoogleProvider(GoogleConfig{APIKey: key})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
