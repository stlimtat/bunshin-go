package transport

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
)

// HandleSync processes a synchronous workflow execution request.
func HandleSync(logger zerolog.Logger, w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	wfID := WorkflowIDFromPath(r.URL.Path)
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
		logger.Error().Err(err).Msg("response encode failed")
	}
}

// HandleSSE processes a streaming workflow execution request via Server-Sent Events.
func HandleSSE(logger zerolog.Logger, w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	wfID := WorkflowIDFromPath(r.URL.Path)
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
			if writeErr := writeSSE(w, flusher, StreamEvent{Type: "error", Error: "invalid input JSON: " + err.Error()}); writeErr != nil {
				logger.Error().Err(writeErr).Msg("sse write failed")
			}
			return
		}
	}

	ch, err := runnable.Stream(r.Context(), input)
	if err != nil {
		if writeErr := writeSSE(w, flusher, StreamEvent{Type: "error", Error: err.Error()}); writeErr != nil {
			logger.Error().Err(writeErr).Msg("sse write failed")
		}
		return
	}

	ctx := r.Context()
	drain := func() {
		go func() {
			// Drain remaining chunks so the producer goroutine is not blocked.
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	for chunk := range ch {
		if chunk.Err != nil {
			if writeErr := writeSSE(w, flusher, StreamEvent{Type: "error", Error: chunk.Err.Error()}); writeErr != nil {
				logger.Error().Err(writeErr).Msg("sse write failed")
			}
			drain()
			return
		}
		token, err := json.Marshal(chunk.Value)
		if err != nil {
			logger.Error().Err(err).Msg("failed to marshal stream chunk")
			if writeErr := writeSSE(w, flusher, StreamEvent{Type: "error", Error: "internal: failed to serialize chunk"}); writeErr != nil {
				logger.Error().Err(writeErr).Msg("sse error write failed")
			}
			drain()
			return
		}
		if err := writeSSE(w, flusher, StreamEvent{Type: "llm_token", Token: string(token)}); err != nil {
			drain()
			return
		}
	}
	if err := writeSSE(w, flusher, StreamEvent{Type: "done"}); err != nil {
		logger.Error().Err(err).Msg("sse done write failed")
	}
}
