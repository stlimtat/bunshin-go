// Package transport provides server interfaces for exposing bunshin-go workflows
// over the network.
//
// Three transport modes:
//
//   GRPCTransport  — gRPC/HTTP2. Bidirectional streaming via ExecuteStream RPC.
//                    Best for service-to-service calls, microservice architectures.
//
//   HTTPTransport  — HTTP/2 with Server-Sent Events (SSE) for streaming LLM token
//                    output to browser clients. Also exposes a synchronous POST endpoint.
//
//   StreamTransport — Abstract pub/sub interface. Backed by Kafka, NATS, or WebSocket.
//                     Useful for event-driven architectures and async workflows.
//
// All transports share the WorkflowHandler interface — implement once, expose anywhere.
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// WorkflowRequest is the canonical input to a workflow execution over the wire.
type WorkflowRequest struct {
	// WorkflowID identifies which workflow to run.
	WorkflowID string `json:"workflow_id"`
	// ThreadID is the horizontal-scale coordination key for checkpoint/resume.
	ThreadID string `json:"thread_id,omitempty"`
	// Input is the workflow-specific input payload.
	Input map[string]any `json:"input"`
}

// WorkflowResponse is the synchronous response from a workflow execution.
type WorkflowResponse struct {
	ThreadID string         `json:"thread_id"`
	Output   map[string]any `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// StreamEvent is one event in a workflow execution stream.
type StreamEvent struct {
	// Type classifies the event: "step_start", "llm_token", "step_end", "error", "done".
	Type    string `json:"type"`
	StepID  string `json:"step_id,omitempty"`
	Token   string `json:"token,omitempty"`
	Output  any    `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WorkflowHandler maps workflow IDs to Runnables.
// The transport calls Invoke or Stream on the matching Runnable.
type WorkflowHandler interface {
	// Handle returns the Runnable for workflowID, or an error if not found.
	Handle(workflowID string) (core.Runnable, error)
}

// Transport is the interface all server backends implement.
type Transport interface {
	// Serve starts the server and blocks until ctx is cancelled.
	Serve(ctx context.Context, handler WorkflowHandler) error
	// Shutdown gracefully stops the server.
	Shutdown(ctx context.Context) error
}

// MapHandler is a simple WorkflowHandler backed by a map.
type MapHandler struct {
	mu        sync.RWMutex
	workflows map[string]core.Runnable
}

// NewMapHandler constructs an empty MapHandler.
func NewMapHandler() *MapHandler {
	return &MapHandler{workflows: make(map[string]core.Runnable)}
}

// Register adds a workflow.
func (h *MapHandler) Register(id string, r core.Runnable) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workflows[id] = r
}

// Handle returns the Runnable for workflowID.
func (h *MapHandler) Handle(workflowID string) (core.Runnable, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.workflows[workflowID]
	if !ok {
		return nil, fmt.Errorf("workflow %q not found", workflowID)
	}
	return r, nil
}

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

	// Synchronous endpoint.
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

func isStreamPath(path string) bool {
	return len(path) > 7 && path[len(path)-7:] == "/stream"
}

func workflowIDFromPath(path string) string {
	// /workflows/{id} or /workflows/{id}/stream
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
	// Remove "/stream" suffix to get workflow ID.
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

// StreamTransport is an abstract pub/sub transport.
// Implement MessageBroker to back it with Kafka, NATS, or WebSocket.
type StreamTransport struct {
	broker MessageBroker
}

// MessageBroker is the pub/sub primitive backing StreamTransport.
type MessageBroker interface {
	Publish(ctx context.Context, topic string, msg []byte) error
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
	Close() error
}

// NewStreamTransport constructs a StreamTransport backed by broker.
func NewStreamTransport(broker MessageBroker) *StreamTransport {
	return &StreamTransport{broker: broker}
}

func (t *StreamTransport) Serve(ctx context.Context, handler WorkflowHandler) error {
	// Subscribe to workflow execution requests.
	msgs, err := t.broker.Subscribe(ctx, "bunshin.workflow.requests")
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data, ok := <-msgs:
			if !ok {
				return nil
			}
			var req WorkflowRequest
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}
			go t.dispatch(ctx, req, handler)
		}
	}
}

func (t *StreamTransport) dispatch(ctx context.Context, req WorkflowRequest, h WorkflowHandler) {
	runnable, err := h.Handle(req.WorkflowID)
	resp := WorkflowResponse{ThreadID: req.ThreadID}
	if err != nil {
		resp.Error = err.Error()
	} else {
		out, err := runnable.Invoke(ctx, req.Input)
		if err != nil {
			resp.Error = err.Error()
		} else if m, ok := out.(map[string]any); ok {
			resp.Output = m
		} else {
			resp.Output = map[string]any{"result": out}
		}
	}
	data, _ := json.Marshal(resp)
	_ = t.broker.Publish(ctx, "bunshin.workflow.responses."+req.ThreadID, data)
}

func (t *StreamTransport) Shutdown(_ context.Context) error {
	return t.broker.Close()
}
