package transport

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- health handlers ----

func TestHandleHealth_GET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	HandleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("expected 'ok' in body, got %q", w.Body.String())
	}
}

func TestHandleHealth_NonGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	w := httptest.NewRecorder()
	HandleHealth(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

func TestHandleLive_GET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	HandleLive(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "live") {
		t.Errorf("expected 'live' in body, got %q", w.Body.String())
	}
}

func TestHandleLive_NonGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/livez", nil)
	w := httptest.NewRecorder()
	HandleLive(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

func TestHandleReady_GET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	HandleReady(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ready") {
		t.Errorf("expected 'ready' in body, got %q", w.Body.String())
	}
}

func TestHandleReady_NonGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/readyz", nil)
	w := httptest.NewRecorder()
	HandleReady(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

// ---- path helpers ----

func TestIsStreamPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/workflows/foo/stream", true},
		{"/workflows/foo", false},
		{"/stream", true},
		{"", false},
		{"/workflows/foo/stream/extra", false},
	}
	for _, tc := range cases {
		if got := IsStreamPath(tc.path); got != tc.want {
			t.Errorf("IsStreamPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestWorkflowIDFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/workflows/my-flow", "my-flow"},
		{"/workflows/my-flow/stream", "my-flow"},
		{"/workflows/", ""},
		{"/workflows", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := WorkflowIDFromPath(tc.path); got != tc.want {
			t.Errorf("WorkflowIDFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// ---- writeSSE ----

func TestWriteSSE(t *testing.T) {
	w := httptest.NewRecorder()
	flusher := w // httptest.ResponseRecorder does not implement http.Flusher directly
	_ = flusher

	// Use a buffered flusher shim.
	bf := &bufferingFlusher{ResponseRecorder: w}
	event := StreamEvent{Type: "llm_token", Token: "hello"}
	if err := writeSSE(bf, bf, event); err != nil {
		t.Fatalf("writeSSE error: %v", err)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("expected SSE data prefix, got %q", body)
	}
	if !strings.Contains(body, "llm_token") {
		t.Errorf("expected type in body, got %q", body)
	}
}

// bufferingFlusher wraps httptest.ResponseRecorder to satisfy http.Flusher.
type bufferingFlusher struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (b *bufferingFlusher) Flush() {
	b.flushed = true
}

// ---- writeSSE flushed ----

func TestWriteSSE_Flushed(t *testing.T) {
	w := httptest.NewRecorder()
	bf := &bufferingFlusher{ResponseRecorder: w}
	_ = writeSSE(bf, bf, StreamEvent{Type: "done"})
	if !bf.flushed {
		t.Error("expected Flush() to be called")
	}
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	found := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "done") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'done' in SSE output, got %q", w.Body.String())
	}
}
