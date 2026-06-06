package credentials_test

import (
	"context"
	"os"
	"testing"

	"github.com/stlimtat/bunshin-go/internal/credentials"
)

func TestWithCredential_RoundTrip(t *testing.T) {
	ctx := credentials.WithCredential(context.Background(), "openai", credentials.APIKeyCredential("sk-test"))
	got, ok := credentials.FromContext(ctx, "openai")
	if !ok {
		t.Fatal("expected credential in context")
	}
	if got.APIKey != "sk-test" {
		t.Errorf("expected sk-test, got %q", got.APIKey)
	}
}

func TestFromContext_Missing(t *testing.T) {
	_, ok := credentials.FromContext(context.Background(), "missing")
	if ok {
		t.Error("expected false for missing service")
	}
}

func TestFromContext_ServiceIsolation(t *testing.T) {
	ctx := credentials.WithCredential(context.Background(), "openai", credentials.APIKeyCredential("key-a"))
	ctx = credentials.WithCredential(ctx, "anthropic", credentials.APIKeyCredential("key-b"))

	a, _ := credentials.FromContext(ctx, "openai")
	b, _ := credentials.FromContext(ctx, "anthropic")
	if a.APIKey != "key-a" || b.APIKey != "key-b" {
		t.Errorf("service isolation broken: openai=%q anthropic=%q", a.APIKey, b.APIKey)
	}
}

func TestEnvProvider_Get(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "env-sk-test")
	p := &credentials.EnvProvider{Map: map[string]string{"openai": "TEST_OPENAI_KEY"}}
	cred, err := p.Get(context.Background(), "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.APIKey != "env-sk-test" {
		t.Errorf("expected env-sk-test, got %q", cred.APIKey)
	}
}

func TestEnvProvider_Missing(t *testing.T) {
	os.Unsetenv("UNSET_KEY_12345")
	p := &credentials.EnvProvider{Map: map[string]string{"svc": "UNSET_KEY_12345"}}
	cred, err := p.Get(context.Background(), "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.APIKey != "" {
		t.Errorf("expected empty APIKey, got %q", cred.APIKey)
	}
}

func TestStaticProvider_Get(t *testing.T) {
	p := &credentials.StaticProvider{Cred: credentials.APIKeyCredential("static-key")}
	cred, err := p.Get(context.Background(), "any-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.APIKey != "static-key" {
		t.Errorf("expected static-key, got %q", cred.APIKey)
	}
}
