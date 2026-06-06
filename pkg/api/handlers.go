package api

import (
	"encoding/json"
	"net/http"
)

func (ro *Router) handleWorkflowInvoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing workflow id", http.StatusBadRequest)
		return
	}
	runnable, err := ro.handler.Handle(id)
	if err != nil {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}

	var input any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	output, err := runnable.Invoke(r.Context(), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(output)
}

func (ro *Router) handleWorkflowStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing workflow id", http.StatusBadRequest)
		return
	}
	runnable, err := ro.handler.Handle(id)
	if err != nil {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}

	var input any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		input = nil
	}

	ch, err := runnable.Stream(r.Context(), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, canFlush := w.(http.Flusher)

	enc := json.NewEncoder(w)
	for chunk := range ch {
		_, _ = w.Write([]byte("data: "))
		_ = enc.Encode(chunk)
		if canFlush {
			flusher.Flush()
		}
	}
	_, _ = w.Write([]byte("data: {\"type\":\"done\"}\n\n"))
	if canFlush {
		flusher.Flush()
	}
}

// handleListThreads is a stub — requires MessageStore integration.
func (ro *Router) handleListThreads(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
}

// handleGetThreadMessages is a stub — requires MessageStore integration.
func (ro *Router) handleGetThreadMessages(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"messages": []any{}})
}

// handlePromptActivate is a stub — requires PromptCache integration.
func (ro *Router) handlePromptActivate(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
}

// handlePromptRefresh is a stub — requires PromptCache integration.
func (ro *Router) handlePromptRefresh(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
}
