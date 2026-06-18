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
	"github.com/stlimtat/bunshin-go/pkg/workflow"
	wfmemory "github.com/stlimtat/bunshin-go/pkg/workflow/store/memory"
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
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/missing/invoke", http.NoBody)
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
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows/echo/invoke", body))
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
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows/echo/invoke", strings.NewReader("not-json")))
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

func TestRouter_GetThreadMessages(t *testing.T) {
	router := api.NewRouter(&fakeHandler{runnables: map[string]core.Runnable{}})
	mux := http.NewServeMux()
	router.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/threads/thread-123/messages", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if _, ok := body["messages"]; !ok {
		t.Error("expected 'messages' key in response")
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

// ---- workflow CRUD ----

const testSpecYAML = `name: test-wf
nodes:
  - id: step1
    runnable: {type: custom, name: x}
`

func wfRouter(t *testing.T) (*api.Router, *http.ServeMux) {
	t.Helper()
	store := wfmemory.New()
	router := api.NewRouter(
		&fakeHandler{runnables: map[string]core.Runnable{}},
		api.RouterConfig{WorkflowStore: store, WorkflowTenantID: "test"},
	)
	mux := http.NewServeMux()
	router.Mount(mux)
	return router, mux
}

func TestRouter_WorkflowCreate_OK(t *testing.T) {
	_, mux := wfRouter(t)
	body := bytes.NewBufferString(`{"spec":"` + jsonEscape(testSpecYAML) + `"}`)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows", body))
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["version"] == "" {
		t.Error("expected version in response")
	}
}

func TestRouter_WorkflowCreate_BadJSON(t *testing.T) {
	_, mux := wfRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows", strings.NewReader("not-json")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRouter_WorkflowCreate_NoStore(t *testing.T) {
	router := api.NewRouter(&fakeHandler{runnables: map[string]core.Runnable{}})
	mux := http.NewServeMux()
	router.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows", bytes.NewBufferString(`{"spec":"x"}`)))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 without store, got %d", rec.Code)
	}
}

func TestRouter_WorkflowList_Empty(t *testing.T) {
	_, mux := wfRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if _, ok := resp["workflows"]; !ok {
		t.Error("expected workflows key")
	}
}

func TestRouter_WorkflowGet_NotFound(t *testing.T) {
	_, mux := wfRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRouter_WorkflowCRUD_FullLifecycle(t *testing.T) {
	store := wfmemory.New()
	router := api.NewRouter(
		&fakeHandler{runnables: map[string]core.Runnable{}},
		api.RouterConfig{WorkflowStore: store, WorkflowTenantID: "test"},
	)
	mux := http.NewServeMux()
	router.Mount(mux)

	// Create.
	body := bytes.NewBufferString(`{"spec":"` + jsonEscape(testSpecYAML) + `"}`)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&createResp)
	ver := createResp["version"]

	// List.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List: want 200, got %d", rec.Code)
	}

	// Get active — no active yet.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/test-wf", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("Get before activate: want 404, got %d", rec.Code)
	}

	// Activate.
	activateBody := bytes.NewBufferString(`{"version":"` + ver + `"}`)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows/test-wf/activate", activateBody))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("Activate: want 202, got %d: %s", rec.Code, rec.Body.String())
	}

	// Get active — should now return spec.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/test-wf", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("Get after activate: want 200, got %d", rec.Code)
	}

	// ListVersions.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/test-wf/versions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ListVersions: want 200, got %d", rec.Code)
	}

	// GetVersion.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/test-wf/versions/"+ver, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GetVersion: want 200, got %d", rec.Code)
	}

	// Delete.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/workflows/test-wf", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("Delete: want 204, got %d", rec.Code)
	}

	// Get after delete — not found.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/workflows/test-wf", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("Get after delete: want 404, got %d", rec.Code)
	}
}

func TestRouter_WorkflowActivate_MissingVersion(t *testing.T) {
	_, mux := wfRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/workflows/test-wf/activate",
		bytes.NewBufferString(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing version, got %d", rec.Code)
	}
}

func TestRouter_WorkflowDelete_NotFound(t *testing.T) {
	_, mux := wfRouter(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/workflows/nonexistent", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// jsonEscape escapes a YAML string for embedding in a JSON string literal.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// ensure workflow/store/memory import is used
var _ workflow.Store = (*wfmemory.Store)(nil)
