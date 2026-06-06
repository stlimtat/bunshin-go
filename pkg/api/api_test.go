package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRouter_WorkflowInvoke_OK(t *testing.T) {
	handler := &fakeHandler{runnables: map[string]core.Runnable{"echo": echoRunnable()}}
	router := api.NewRouter(handler)
	mux := http.NewServeMux()
	router.Mount(mux)

	body := bytes.NewBufferString(`{"msg":"hi"}`)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows/echo", body))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRouter_WorkflowInvoke_BadJSON(t *testing.T) {
	handler := &fakeHandler{runnables: map[string]core.Runnable{"echo": echoRunnable()}}
	router := api.NewRouter(handler)
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows/echo", strings.NewReader("not-json")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", rec.Code)
	}
}

func TestRouter_WorkflowStream_SSE(t *testing.T) {
	streamRunnable := core.NewRunnableFuncWithStream(
		"stream-echo",
		func(_ context.Context, input any) (any, error) { return input, nil },
		func(_ context.Context, input any) (<-chan core.StreamChunk, error) {
			ch := make(chan core.StreamChunk, 2)
			ch <- core.StreamChunk{Value: map[string]any{"type": "llm_token", "token": "hello"}}
			close(ch)
			return ch, nil
		},
	)
	handler := &fakeHandler{runnables: map[string]core.Runnable{"s": streamRunnable}}
	router := api.NewRouter(handler)
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/s/stream", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", ct)
	}
	found := false
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "llm_token") {
			found = true
		}
	}
	if !found {
		t.Error("expected llm_token event in SSE stream")
	}
}

func TestRouter_PromptActivate_NotConfigured(t *testing.T) {
	router := api.NewRouter(&fakeHandler{runnables: map[string]core.Runnable{}})
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/prompts/my-prompt/activate", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 when no activator configured, got %d", rec.Code)
	}
}

type fakeActivator struct{ promoted string }

func (f *fakeActivator) Promote(_ context.Context, name string) error {
	f.promoted = name
	return nil
}

func TestRouter_PromptActivate_WithActivator(t *testing.T) {
	activator := &fakeActivator{}
	router := api.NewRouter(
		&fakeHandler{runnables: map[string]core.Runnable{}},
		api.RouterConfig{Activator: activator},
	)
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/prompts/my-prompt/activate", nil))
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
	if activator.promoted != "my-prompt" {
		t.Errorf("expected Promote called with 'my-prompt', got %q", activator.promoted)
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
