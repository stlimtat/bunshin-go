package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// ---- workflow execution ----

func (ro *Router) handleWorkflowInvoke(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing workflow name", http.StatusBadRequest)
		return
	}
	runnable, err := ro.handler.Handle(name)
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
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing workflow name", http.StatusBadRequest)
		return
	}
	runnable, err := ro.handler.Handle(name)
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

// ---- workflow CRUD ----

func (ro *Router) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		Spec string `json:"spec"` // raw YAML text
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.Spec == "" {
		http.Error(w, "spec field is required", http.StatusBadRequest)
		return
	}
	spec, err := workflow.Parse([]byte(body.Spec))
	if err != nil {
		http.Error(w, "invalid spec: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	version, err := ro.workflowStore.Create(r.Context(), ro.workflowTenantID, spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"version": version, "name": spec.Name})
}

func (ro *Router) handleWorkflowList(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	names, err := ro.workflowStore.List(r.Context(), ro.workflowTenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if names == nil {
		names = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"workflows": names})
}

func (ro *Router) handleWorkflowGet(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	spec, err := ro.workflowStore.Get(r.Context(), ro.workflowTenantID, name)
	if err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}

func (ro *Router) handleWorkflowListVersions(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	vers, err := ro.workflowStore.ListVersions(r.Context(), ro.workflowTenantID, name)
	if err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if vers == nil {
		vers = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"versions": vers})
}

func (ro *Router) handleWorkflowGetVersion(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	ver := r.PathValue("ver")
	spec, err := ro.workflowStore.GetVersion(r.Context(), ro.workflowTenantID, name, ver)
	if err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}

func (ro *Router) handleWorkflowActivate(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Version == "" {
		http.Error(w, "version field is required", http.StatusBadRequest)
		return
	}
	if err := ro.workflowStore.Activate(r.Context(), ro.workflowTenantID, name, body.Version); err != nil {
		if errors.Is(err, workflow.ErrVersionConflict) || errors.Is(err, workflow.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (ro *Router) handleWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	if ro.workflowStore == nil {
		http.Error(w, "workflow store not configured", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	if err := ro.workflowStore.Delete(r.Context(), ro.workflowTenantID, name); err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- threads ----

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

// ---- prompts ----

func (ro *Router) handlePromptActivate(w http.ResponseWriter, r *http.Request) {
	if ro.activator == nil {
		http.Error(w, "prompt activator not configured", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing prompt name", http.StatusBadRequest)
		return
	}
	if err := ro.activator.Promote(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (ro *Router) handlePromptRefresh(w http.ResponseWriter, _ *http.Request) {
	if ro.refresher == nil {
		http.Error(w, "prompt refresher not configured", http.StatusNotImplemented)
		return
	}
	ro.refresher.Refresh()
	w.WriteHeader(http.StatusAccepted)
}
