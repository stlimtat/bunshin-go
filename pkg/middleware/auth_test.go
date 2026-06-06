package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/auth"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

var echoRunnable = core.AsRunnable("echo", core.TypedFunc(func(_ context.Context, input any) (any, error) {
	return input, nil
}))

func TestWithRBAC_AllowsMatchingRole(t *testing.T) {
	mw := middleware.WithRBAC(func(p auth.Principal) bool {
		return p.HasRole("admin")
	})
	guarded := middleware.Chain(echoRunnable, mw)

	ctx := auth.WithContext(context.Background(), auth.Principal{
		Subject: "u1", TenantID: "t1", Roles: []string{"admin"},
	})
	_, err := guarded.Invoke(ctx, "ping")
	if err != nil {
		t.Errorf("expected no error for admin role, got %v", err)
	}
}

func TestWithRBAC_RejectsMissingRole(t *testing.T) {
	mw := middleware.WithRBAC(func(p auth.Principal) bool {
		return p.HasRole("admin")
	})
	guarded := middleware.Chain(echoRunnable, mw)

	ctx := auth.WithContext(context.Background(), auth.Principal{
		Subject: "u1", TenantID: "t1", Roles: []string{"viewer"},
	})
	_, err := guarded.Invoke(ctx, "ping")
	if !errors.Is(err, middleware.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestWithRBAC_RejectsNoPrincipal(t *testing.T) {
	mw := middleware.WithRBAC(func(p auth.Principal) bool { return true })
	guarded := middleware.Chain(echoRunnable, mw)

	_, err := guarded.Invoke(context.Background(), "ping")
	if !errors.Is(err, middleware.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized when no principal, got %v", err)
	}
}

func TestWithAPIKeyHTTP_Valid(t *testing.T) {
	var capturedPrincipal auth.Principal
	handler := middleware.WithAPIKeyHTTP("secret-key", "tenant-1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal, _ = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "secret-key")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedPrincipal.TenantID != "tenant-1" {
		t.Errorf("expected TenantID=tenant-1, got %q", capturedPrincipal.TenantID)
	}
}

func TestWithAPIKeyHTTP_Invalid(t *testing.T) {
	handler := middleware.WithAPIKeyHTTP("secret-key", "tenant-1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestWithBearerJWTHTTP_Valid(t *testing.T) {
	validate := func(token string) (auth.Principal, error) {
		if token == "valid-token" {
			return auth.Principal{Subject: "user-1", TenantID: "t1", Roles: []string{"editor"}}, nil
		}
		return auth.Principal{}, errors.New("invalid")
	}
	var capturedPrincipal auth.Principal
	handler := middleware.WithBearerJWTHTTP(validate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal, _ = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedPrincipal.Subject != "user-1" {
		t.Errorf("expected Subject=user-1, got %q", capturedPrincipal.Subject)
	}
}

func TestWithBearerJWTHTTP_Missing(t *testing.T) {
	validate := func(token string) (auth.Principal, error) { return auth.Principal{}, nil }
	handler := middleware.WithBearerJWTHTTP(validate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestWithAPIKey_RejectsNoPrincipal(t *testing.T) {
	guarded := middleware.Chain(echoRunnable, middleware.WithAPIKey("sk-test"))
	_, err := guarded.Invoke(context.Background(), "input")
	if !errors.Is(err, middleware.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized when no principal, got %v", err)
	}
}

func TestWithAPIKey_AllowsWithPrincipal(t *testing.T) {
	guarded := middleware.Chain(echoRunnable, middleware.WithAPIKey("sk-test"))
	ctx := auth.WithContext(context.Background(), auth.Principal{Subject: "user", TenantID: "t1"})
	out, err := guarded.Invoke(ctx, "ping")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "ping" {
		t.Errorf("expected echo, got %v", out)
	}
}

func TestWithBearerJWT_RejectsNoToken(t *testing.T) {
	validate := func(token string) (auth.Principal, error) {
		return auth.Principal{Subject: "u"}, nil
	}
	guarded := middleware.Chain(echoRunnable, middleware.WithBearerJWT(validate))
	_, err := guarded.Invoke(context.Background(), "input")
	if !errors.Is(err, middleware.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestWithBearerJWT_AllowsValidToken(t *testing.T) {
	validate := func(token string) (auth.Principal, error) {
		if token == "tok" {
			return auth.Principal{Subject: "u", TenantID: "t1"}, nil
		}
		return auth.Principal{}, errors.New("invalid")
	}
	guarded := middleware.Chain(echoRunnable, middleware.WithBearerJWT(validate))

	// Inject token via WithBearerJWTHTTP into context, then call Runnable
	var capturedCtx context.Context
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.WithBearerJWTHTTP(validate, innerHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	out, err := guarded.Invoke(capturedCtx, "hello")
	if err != nil {
		t.Fatalf("expected no error with valid token, got %v", err)
	}
	if out != "hello" {
		t.Errorf("expected echo, got %v", out)
	}
}

func TestWithIPAllowlistHTTP_Allowed(t *testing.T) {
	handler := middleware.WithIPAllowlistHTTP([]string{"127.0.0"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for allowed IP, got %d", rec.Code)
	}
}

func TestWithIPAllowlistHTTP_Denied(t *testing.T) {
	handler := middleware.WithIPAllowlistHTTP([]string{"10.0.0"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.5:9999"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied IP, got %d", rec.Code)
	}
}
