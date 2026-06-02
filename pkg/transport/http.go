package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPTransport serves workflows over HTTP/2.
//
// Endpoints:
//
//	POST /workflows/{id}         — synchronous execution
//	GET  /workflows/{id}/stream  — SSE streaming execution
//
// SSE event format:
//
//	data: {"type":"step_start","step_id":"A"}
//	data: {"type":"llm_token","token":"Hello"}
//	data: {"type":"done","output":{...}}
type HTTPTransport struct {
	addr   string
	server *http.Server
}

// NewHTTPTransport constructs an HTTPTransport listening on addr (e.g. ":8080").
func NewHTTPTransport(addr string) *HTTPTransport {
	return &HTTPTransport{addr: addr}
}

func (t *HTTPTransport) Serve(ctx context.Context, handler WorkflowHandler) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/workflows/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && !isStreamPath(r.URL.Path):
			t.handleSync(w, r, handler)
		case r.Method == http.MethodGet && isStreamPath(r.URL.Path):
			t.handleSSE(w, r, handler)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	t.server = &http.Server{Addr: t.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = t.server.Shutdown(context.Background())
	}()
	return t.server.ListenAndServe()
}

func (t *HTTPTransport) Shutdown(ctx context.Context) error {
	if t.server != nil {
		return t.server.Shutdown(ctx)
	}
	return nil
}

// HandleRequest dispatches a request to the sync or SSE handler.
// Exported for testing without starting a real HTTP server.
func (t *HTTPTransport) HandleRequest(w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	switch {
	case r.Method == http.MethodPost && !isStreamPath(r.URL.Path):
		t.handleSync(w, r, h)
	case r.Method == http.MethodGet && isStreamPath(r.URL.Path):
		t.handleSSE(w, r, h)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (t *HTTPTransport) handleSync(w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	wfID := workflowIDFromPath(r.URL.Path)
	runnable, err := h.Handle(wfID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req WorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	out, err := runnable.Invoke(r.Context(), req.Input)
	resp := WorkflowResponse{ThreadID: req.ThreadID}
	if err != nil {
		resp.Error = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		if m, ok := out.(map[string]any); ok {
			resp.Output = m
		} else {
			resp.Output = map[string]any{"result": out}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (t *HTTPTransport) handleSSE(w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	wfID := workflowIDFromPath(r.URL.Path)
	if len(wfID) > 0 && wfID == "stream" {
		parts := splitPath(r.URL.Path)
		if len(parts) >= 2 {
			wfID = parts[len(parts)-2]
		}
	}

	runnable, err := h.Handle(wfID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var input map[string]any
	if r.URL.Query().Get("input") != "" {
		_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &input)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ch, err := runnable.Stream(r.Context(), input)
	if err != nil {
		writeSSE(w, flusher, StreamEvent{Type: "error", Error: err.Error()})
		return
	}

	for chunk := range ch {
		if chunk.Err != nil {
			writeSSE(w, flusher, StreamEvent{Type: "error", Error: chunk.Err.Error()})
			return
		}
		writeSSE(w, flusher, StreamEvent{Type: "llm_token", Token: fmt.Sprintf("%v", chunk.Value)})
	}
	writeSSE(w, flusher, StreamEvent{Type: "done"})
}

func writeSSE(w http.ResponseWriter, f http.Flusher, event StreamEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

func isStreamPath(path string) bool {
	return len(path) > 7 && path[len(path)-7:] == "/stream"
}

func workflowIDFromPath(path string) string {
	parts := splitPath(path)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func splitPath(path string) []string {
	var parts []string
	start := 0
	for i, c := range path {
		if c == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}
