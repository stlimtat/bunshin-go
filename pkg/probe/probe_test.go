package probe_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/probe"
)

func TestHandler_Healthz_NoCheckers(t *testing.T) {
	h := probe.NewHandler()
	mux := http.NewServeMux()
	h.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with no checkers, got %d", rec.Code)
	}
}

func TestHandler_Readyz_AllPass(t *testing.T) {
	h := probe.NewHandler()
	h.RegisterReadiness("db", probe.CheckerFunc(func(_ context.Context) error { return nil }))
	h.RegisterReadiness("cache", probe.CheckerFunc(func(_ context.Context) error { return nil }))

	mux := http.NewServeMux()
	h.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_Readyz_OneFails(t *testing.T) {
	h := probe.NewHandler()
	h.RegisterReadiness("db", probe.CheckerFunc(func(_ context.Context) error { return nil }))
	h.RegisterReadiness("cache", probe.CheckerFunc(func(_ context.Context) error {
		return errors.New("connection refused")
	}))

	mux := http.NewServeMux()
	h.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when checker fails, got %d", rec.Code)
	}
}

func TestHandler_Healthz_LivenessFails(t *testing.T) {
	h := probe.NewHandler()
	h.RegisterLiveness("goroutine-leak", probe.CheckerFunc(func(_ context.Context) error {
		return errors.New("goroutine count exceeded threshold")
	}))

	mux := http.NewServeMux()
	h.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}
