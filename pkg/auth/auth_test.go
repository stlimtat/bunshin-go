package auth_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/auth"
)

func TestFromContext_missing(t *testing.T) {
	_, ok := auth.FromContext(context.Background())
	if ok {
		t.Fatal("expected no Principal in empty context")
	}
}

func TestWithContext_roundtrip(t *testing.T) {
	p := auth.Principal{
		Subject:  "user-1",
		TenantID: "tenant-abc",
		Roles:    []string{"admin", "editor"},
		Claims:   map[string]any{"email": "user@example.com"},
	}
	ctx := auth.WithContext(context.Background(), p)
	got, ok := auth.FromContext(ctx)
	if !ok {
		t.Fatal("expected Principal in context")
	}
	if got.Subject != p.Subject || got.TenantID != p.TenantID {
		t.Fatalf("got %+v, want %+v", got, p)
	}
}

func TestMustFromContext_panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	auth.MustFromContext(context.Background())
}

func TestPrincipal_HasRole(t *testing.T) {
	p := auth.Principal{Roles: []string{"editor", "viewer"}}
	if !p.HasRole("editor") {
		t.Error("expected HasRole(editor) true")
	}
	if p.HasRole("admin") {
		t.Error("expected HasRole(admin) false")
	}
}
