package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on DefaultServeMux
	"time"

	"github.com/rs/zerolog"
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
	logger zerolog.Logger
}

// NewHTTPTransport constructs an HTTPTransport listening on addr (e.g. ":8080").
func NewHTTPTransport(addr string) *HTTPTransport {
	return &HTTPTransport{addr: addr, logger: zerolog.Nop()}
}

// WithLogger sets the logger for the transport.
func (t *HTTPTransport) WithLogger(logger zerolog.Logger) *HTTPTransport {
	t.logger = logger
	return t
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

	// Health endpoints
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/live", handleLive)
	mux.HandleFunc("/ready", handleReady)

	// Profiling — exposes /debug/pprof/* via DefaultServeMux handlers.
	mux.Handle("/debug/pprof/", http.DefaultServeMux)

	t.server = &http.Server{Addr: t.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := t.server.Shutdown(shutCtx); err != nil {
			t.logger.Error().Err(err).Msg("graceful shutdown incomplete")
		}
	}()
	if err := t.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
	w.Header().Set("Content-Type", "application/json")
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
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.logger.Error().Err(err).Msg("response encode failed")
	}
}

func (t *HTTPTransport) handleSSE(w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	// Extract workflow ID from /workflows/{id}/stream — parts[1] is always the ID.
	wfID := workflowIDFromPath(r.URL.Path)

	runnable, err := h.Handle(wfID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var input map[string]any
	if raw := r.URL.Query().Get("input"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			_ = writeSSE(w, flusher, StreamEvent{Type: "error", Error: "invalid input JSON: " + err.Error()})
			return
		}
	}

	ch, err := runnable.Stream(r.Context(), input)
	if err != nil {
		_ = writeSSE(w, flusher, StreamEvent{Type: "error", Error: err.Error()})
		return
	}

	for chunk := range ch {
		if chunk.Err != nil {
			_ = writeSSE(w, flusher, StreamEvent{Type: "error", Error: chunk.Err.Error()})
			return
		}
		if err := writeSSE(w, flusher, StreamEvent{Type: "llm_token", Token: fmt.Sprintf("%v", chunk.Value)}); err != nil {
			return
		}
	}
	_ = writeSSE(w, flusher, StreamEvent{Type: "done"})
}

func writeSSE(w http.ResponseWriter, f http.Flusher, event StreamEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	f.Flush()
	return nil
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

// handleHealth returns 200 with a JSON body. Used by Docker healthchecks and load balancers.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handleLive returns 200 as long as the process is running (Kubernetes liveness probe).
func handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"live"}`))
}

// handleReady returns 200 when the server is ready to accept traffic (Kubernetes readiness probe).
// Extend this to check downstream dependencies (DB, Redis) when those are wired in.
func handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ready"}`))
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
