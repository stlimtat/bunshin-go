package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/transport"
)

func echoHandler() *transport.MapHandler {
	h := transport.NewMapHandler()
	h.Register("echo", core.NewRunnableFunc("echo", func(_ context.Context, input any) (any, error) {
		return input, nil
	}))
	return h
}

// ---- MapHandler ----

func TestMapHandler_Handle_Found(t *testing.T) {
	h := echoHandler()
	r, err := h.Handle("echo")
	if err != nil || r == nil {
		t.Fatalf("unexpected: %v %v", r, err)
	}
}

func TestMapHandler_Handle_NotFound(t *testing.T) {
	h := transport.NewMapHandler()
	_, err := h.Handle("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---- HTTPTransport synchronous endpoint ----

func TestHTTPTransport_SyncEndpoint(t *testing.T) {
	h := echoHandler()
	tr := transport.NewHTTPTransport(":0")

	body, _ := json.Marshal(transport.WorkflowRequest{
		WorkflowID: "echo",
		ThreadID:   "t1",
		Input:      map[string]any{"msg": "hello"},
	})

	req := httptest.NewRequest(http.MethodPost, "/workflows/echo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Call the handler directly via the mux (without starting the server).
	mux := buildMux(h, tr)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp transport.WorkflowResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}

func TestHTTPTransport_SyncEndpoint_UnknownWorkflow(t *testing.T) {
	h := transport.NewMapHandler() // empty
	tr := transport.NewHTTPTransport(":0")

	body, _ := json.Marshal(transport.WorkflowRequest{WorkflowID: "missing"})
	req := httptest.NewRequest(http.MethodPost, "/workflows/missing", bytes.NewReader(body))
	w := httptest.NewRecorder()

	mux := buildMux(h, tr)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHTTPTransport_SyncEndpoint_InvalidJSON(t *testing.T) {
	h := echoHandler()
	tr := transport.NewHTTPTransport(":0")

	req := httptest.NewRequest(http.MethodPost, "/workflows/echo", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()

	mux := buildMux(h, tr)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// buildMux replicates the HTTPTransport's internal mux for unit testing
// without starting a real server.
func buildMux(h transport.WorkflowHandler, tr *transport.HTTPTransport) *http.ServeMux {
	mux := http.NewServeMux()
	// Register a test-compatible handler that delegates to the transport.
	mux.HandleFunc("/workflows/", func(w http.ResponseWriter, r *http.Request) {
		tr.HandleRequest(w, r, h)
	})
	return mux
}
