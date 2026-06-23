package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/auth"
	"github.com/stlimtat/bunshin-go/pkg/prompt"
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

func (ro *Router) tenantID(r *http.Request) string {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		return "default"
	}
	return p.TenantID
}

func (ro *Router) handlePromptUpsert(w http.ResponseWriter, r *http.Request) {
	if ro.promptBackend == nil {
		http.Error(w, "prompt backend not configured", http.StatusNotImplemented)
		return
	}
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "missing slug", http.StatusBadRequest)
		return
	}
	var body struct {
		Content      string   `json:"content"`
		Tags         []string `json:"tags"`
		VersionLabel string   `json:"version_label"`
		Slug         string   `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	tenantID := ro.tenantID(r)

	// Rename if body slug differs from URL slug.
	targetSlug := slug
	if body.Slug != "" && body.Slug != slug {
		existing, err := ro.promptBackend.Get(r.Context(), tenantID, slug)
		if err == nil {
			if err := ro.promptBackend.Rename(r.Context(), tenantID, existing.ID, body.Slug); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		targetSlug = body.Slug
	}

	f := &prompt.Fragment{
		Slug:    targetSlug,
		Content: body.Content,
		Tags:    body.Tags,
		Version: body.VersionLabel,
	}
	if err := ro.promptBackend.Put(r.Context(), tenantID, f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(f)
}

func (ro *Router) handlePromptList(w http.ResponseWriter, r *http.Request) {
	if ro.promptBackend == nil {
		http.Error(w, "prompt backend not configured", http.StatusNotImplemented)
		return
	}
	frags, err := ro.promptBackend.List(r.Context(), ro.tenantID(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if frags == nil {
		frags = []*prompt.Fragment{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"fragments": frags})
}

// handlePromptGet is a catch-all GET dispatcher for /v1/prompts/{path...}.
// It routes to the correct sub-handler based on the path shape:
//
//	/v1/prompts/{slug}                → get by slug
//	/v1/prompts/id/{id}              → get by immutable UUID
//	/v1/prompts/{slug}/versions       → list versions (501)
//	/v1/prompts/{slug}/versions/{ver} → get specific version
//
// This catch-all is needed because Go 1.22+ ServeMux panics when registering
// GET /v1/prompts/id/{id} and GET /v1/prompts/{slug}/versions in the same mux
// (both match /v1/prompts/id/versions).
func (ro *Router) handlePromptGet(w http.ResponseWriter, r *http.Request) {
	if ro.promptBackend == nil {
		http.Error(w, "prompt backend not configured", http.StatusNotImplemented)
		return
	}
	path := r.PathValue("path")
	parts := strings.SplitN(path, "/", 3)
	tenantID := ro.tenantID(r)

	switch {
	case len(parts) == 1:
		// GET /v1/prompts/{slug}
		ro.getBySlug(w, r, tenantID, parts[0])
	case len(parts) == 2 && parts[0] == "id":
		// GET /v1/prompts/id/{id}
		ro.getByID(w, r, tenantID, parts[1])
	case len(parts) == 2 && parts[1] == "versions":
		// GET /v1/prompts/{slug}/versions
		http.Error(w, "list versions not yet implemented", http.StatusNotImplemented)
	case len(parts) == 3 && parts[1] == "versions":
		// GET /v1/prompts/{slug}/versions/{ver}
		ro.getVersion(w, r, tenantID, parts[0], parts[2])
	default:
		http.NotFound(w, r)
	}
}

func (ro *Router) getBySlug(w http.ResponseWriter, r *http.Request, tenantID, slug string) {
	f, err := ro.promptBackend.Get(r.Context(), tenantID, slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

func (ro *Router) getByID(w http.ResponseWriter, r *http.Request, tenantID, id string) {
	f, err := ro.promptBackend.GetByID(r.Context(), tenantID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

func (ro *Router) getVersion(w http.ResponseWriter, r *http.Request, tenantID, slug, ver string) {
	f, err := ro.promptBackend.GetVersion(r.Context(), tenantID, slug, ver)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

func (ro *Router) handlePromptGetBySlug(w http.ResponseWriter, r *http.Request) {
	if ro.promptBackend == nil {
		http.Error(w, "prompt backend not configured", http.StatusNotImplemented)
		return
	}
	slug := r.PathValue("slug")
	ro.getBySlug(w, r, ro.tenantID(r), slug)
}

func (ro *Router) handlePromptActivate(w http.ResponseWriter, r *http.Request) {
	if ro.promptActivator == nil {
		http.Error(w, "prompt activator not configured", http.StatusNotImplemented)
		return
	}
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "missing prompt slug", http.StatusBadRequest)
		return
	}
	tenantID := ro.tenantID(r)
	if err := ro.promptActivator.Promote(r.Context(), tenantID, slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (ro *Router) handlePromptDelete(w http.ResponseWriter, r *http.Request) {
	if ro.promptBackend == nil {
		http.Error(w, "prompt backend not configured", http.StatusNotImplemented)
		return
	}
	slug := r.PathValue("slug")
	if err := ro.promptBackend.Delete(r.Context(), ro.tenantID(r), slug); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if ro.refresher != nil {
		ro.refresher.Refresh()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handlePromptPurge(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "purge not yet implemented", http.StatusNotImplemented)
}

func (ro *Router) handlePromptRefresh(w http.ResponseWriter, _ *http.Request) {
	if ro.refresher == nil {
		http.Error(w, "prompt refresher not configured", http.StatusNotImplemented)
		return
	}
	ro.refresher.Refresh()
	w.WriteHeader(http.StatusAccepted)
}
