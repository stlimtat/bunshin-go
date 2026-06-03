package llm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestNewAzureOpenAIProvider_MissingAPIKey(t *testing.T) {
	_, err := NewAzureOpenAIProvider(AzureOpenAIConfig{
		Endpoint:   "https://example.openai.azure.com",
		Deployment: "gpt-4o",
	})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestNewAzureOpenAIProvider_MissingEndpoint(t *testing.T) {
	_, err := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "key",
		Deployment: "gpt-4o",
	})
	if err == nil {
		t.Fatal("expected error for missing Endpoint")
	}
}

func TestNewAzureOpenAIProvider_MissingDeployment(t *testing.T) {
	_, err := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:   "key",
		Endpoint: "https://example.openai.azure.com",
	})
	if err == nil {
		t.Fatal("expected error for missing Deployment")
	}
}

func TestAzureOpenAIProvider_ID(t *testing.T) {
	p, err := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "test",
		Endpoint:   "https://example.openai.azure.com",
		Deployment: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID() != ProviderAzureOpenAI {
		t.Errorf("expected %q, got %q", ProviderAzureOpenAI, p.ID())
	}
}

func TestAzureOpenAIProvider_CanTransferContext(t *testing.T) {
	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "test",
		Endpoint:   "https://example.openai.azure.com",
		Deployment: "gpt-4o",
	})
	same, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "other",
		Endpoint:   "https://other.openai.azure.com",
		Deployment: "gpt-4o-mini",
	})
	fake := NewFakeProvider(ProviderFake, "")

	if !p.CanTransferContext(same) {
		t.Error("expected true for same provider")
	}
	if p.CanTransferContext(fake) {
		t.Error("expected false for different provider")
	}
}

func TestAzureOpenAIProvider_DefaultAPIVersion(t *testing.T) {
	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "key",
		Endpoint:   "https://example.openai.azure.com",
		Deployment: "gpt-4o",
	})
	if p.cfg.APIVersion != "2024-02-01" {
		t.Errorf("expected default API version '2024-02-01', got %s", p.cfg.APIVersion)
	}
}

func TestAzureOpenAIProvider_Complete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/openai/deployments/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.Path, "chat/completions") {
			t.Errorf("expected chat/completions in path: %s", r.URL.Path)
		}
		if r.Header.Get("api-key") != "testkey" {
			t.Errorf("unexpected api-key: %s", r.Header.Get("api-key"))
		}
		if r.URL.Query().Get("api-version") == "" {
			t.Error("expected api-version query param")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices":[{"message":{"content":"hello from azure"}}],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}`)
	}))
	defer srv.Close()

	p, err := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "testkey",
		Endpoint:   srv.URL,
		Deployment: "my-deployment",
	})
	if err != nil {
		t.Fatalf("NewAzureOpenAIProvider: %v", err)
	}

	resp, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello from azure" {
		t.Errorf("expected 'hello from azure', got %q", resp.Content)
	}
	if resp.Usage.PromptTokens != 8 {
		t.Errorf("expected 8 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
}

func TestAzureOpenAIProvider_Complete_AuthError(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "bad",
		Endpoint:   srv.URL,
		Deployment: "deploy",
	})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || !strings.HasPrefix(err.Error(), "azure-openai: authentication failed") {
		t.Errorf("expected auth error, got %v", err)
	}
}

func TestAzureOpenAIProvider_Complete_RateLimit(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "key",
		Endpoint:   srv.URL,
		Deployment: "deploy",
	})
	_, err := p.Complete(context.Background(), &Request{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
	})
	if err == nil || !strings.HasPrefix(err.Error(), "azure-openai: rate limit exceeded") {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

func TestAzureOpenAIProvider_StreamComplete(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hello"}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":" azure"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "key",
		Endpoint:   srv.URL,
		Deployment: "deploy",
	})
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

	if result != "hello azure" {
		t.Errorf("expected 'hello azure', got %q", result)
	}
	if !done {
		t.Error("expected Done chunk")
	}
}

func TestAzureOpenAIProvider_Ping(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/models" {
			t.Errorf("unexpected path for ping: %s", r.URL.Path)
		}
		if r.Header.Get("api-key") != "testkey" {
			t.Errorf("unexpected api-key header: %s", r.Header.Get("api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":[]}`)
	}))
	defer srv.Close()

	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "testkey",
		Endpoint:   srv.URL,
		Deployment: "deploy",
	})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAzureOpenAIProvider_Ping_Failure(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "bad",
		Endpoint:   srv.URL,
		Deployment: "deploy",
	})
	if err := p.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

func TestAzureOpenAIProvider_NativeMessages(t *testing.T) {
	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     "key",
		Endpoint:   "https://example.openai.azure.com",
		Deployment: "gpt-4o",
	})
	msgs := []Message{
		NewTextMessage(RoleSystem, "Be concise"),
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

// Integration tests — skipped unless env vars are set.

func TestAzureOpenAIProvider_Integration(t *testing.T) {
	key := os.Getenv("AZURE_OPENAI_API_KEY")
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")
	if key == "" || endpoint == "" || deployment == "" {
		t.Skip("AZURE_OPENAI_API_KEY, AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_DEPLOYMENT not set")
	}
	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     key,
		Endpoint:   endpoint,
		Deployment: deployment,
	})
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

func TestAzureOpenAIProvider_Ping_Integration(t *testing.T) {
	key := os.Getenv("AZURE_OPENAI_API_KEY")
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")
	if key == "" || endpoint == "" || deployment == "" {
		t.Skip("AZURE_OPENAI_API_KEY, AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_DEPLOYMENT not set")
	}
	p, _ := NewAzureOpenAIProvider(AzureOpenAIConfig{
		APIKey:     key,
		Endpoint:   endpoint,
		Deployment: deployment,
	})
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
