package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/api"
	"github.com/stlimtat/bunshin-go/pkg/core"
)

// fakeHandler implements transport.WorkflowHandler for tests.
type fakeHandler struct {
	runnables map[string]core.Runnable
}

func (f *fakeHandler) Handle(id string) (core.Runnable, error) {
	r, ok := f.runnables[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return r, nil
}

func echoRunnable() core.Runnable {
	return core.AsRunnable("echo", core.TypedFunc(func(_ context.Context, input any) (any, error) {
		return input, nil
	}))
}

func TestRouter_WorkflowInvoke_NotFound(t *testing.T) {
	router := api.NewRouter(&fakeHandler{runnables: map[string]core.Runnable{}})
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/missing", http.NoBody)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown workflow, got %d", rec.Code)
	}
}

func TestRouter_ListThreads(t *testing.T) {
	router := api.NewRouter(&fakeHandler{runnables: map[string]core.Runnable{}})
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/threads", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if _, ok := body["threads"]; !ok {
		t.Error("expected 'threads' key in response")
	}
}

func TestRouter_PromptRefresh_NotConfigured(t *testing.T) {
	router := api.NewRouter(&fakeHandler{runnables: map[string]core.Runnable{}})
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/prompts/refresh", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 when no refresher configured, got %d", rec.Code)
	}
}

type fakeRefresher struct{ called bool }

func (f *fakeRefresher) Refresh() { f.called = true }

func TestRouter_PromptRefresh_WithRefresher(t *testing.T) {
	refresher := &fakeRefresher{}
	router := api.NewRouter(
		&fakeHandler{runnables: map[string]core.Runnable{}},
		api.RouterConfig{Refresher: refresher},
	)
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/prompts/refresh", nil))
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
	if !refresher.called {
		t.Error("Refresh() not called")
	}
}
