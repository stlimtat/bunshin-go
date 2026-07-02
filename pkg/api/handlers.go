package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/auth"
	"github.com/stlimtat/bunshin-go/pkg/llm"
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

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	var threadID string
	if tid, ok := raw["thread_id"]; ok {
		_ = json.Unmarshal(tid, &threadID)
	}

	// Use "input" field if present; otherwise treat the whole body as input.
	var input any
	if inputRaw, ok := raw["input"]; ok {
		_ = json.Unmarshal(inputRaw, &input)
	} else {
		converted := make(map[string]any, len(raw))
		for k, v := range raw {
			var val any
			_ = json.Unmarshal(v, &val)
			converted[k] = val
		}
		input = converted
	}

	output, err := runnable.Invoke(r.Context(), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if ro.threads != nil && threadID != "" {
		store, serr := ro.threads.GetOrCreate(r.Context(), threadID)
		if serr == nil {
			inputText, _ := json.Marshal(input)
			outputText, _ := json.Marshal(output)
			_ = store.Append(r.Context(), llm.NewTextMessage(llm.RoleUser, string(inputText)))
			_ = store.Append(r.Context(), llm.NewTextMessage(llm.RoleAssistant, string(outputText)))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"thread_id": threadID, "output": output})
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
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
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

func (ro *Router) handleListThreads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if ro.threads == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		return
	}
	ids, err := ro.threads.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	threads := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		threads = append(threads, map[string]string{"id": id})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"threads": threads})
}

func (ro *Router) handleGetThreadMessages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	threadID := r.PathValue("id")
	if ro.threads == nil || threadID == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []any{}})
		return
	}
	store, err := ro.threads.GetOrCreate(r.Context(), threadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	msgs, err := store.Window(r.Context(), 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type apiMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	out := make([]apiMsg, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, apiMsg{Role: string(m.Role), Content: messageText(m)})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"messages": out})
}

// messageText extracts plain text from the first text part of a message.
func messageText(m llm.Message) string {
	for _, p := range m.Parts {
		if p.Text != "" {
			return p.Text
		}
	}
	return fmt.Sprintf("%v", m.Parts)
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
		ro.listVersions(w, r, tenantID, parts[0])
	case len(parts) == 3 && parts[1] == "versions":
		// GET /v1/prompts/{slug}/versions/{ver}
		ro.getVersion(w, r, tenantID, parts[0], parts[2])
	default:
		http.NotFound(w, r)
	}
}

func (ro *Router) listVersions(w http.ResponseWriter, r *http.Request, tenantID, slug string) {
	frags, err := ro.promptBackend.ListVersions(r.Context(), tenantID, slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	type versionMeta struct {
		Version string `json:"version"`
		Status  string `json:"status"`
	}
	meta := make([]versionMeta, len(frags))
	for i, f := range frags {
		meta[i] = versionMeta{Version: f.Version, Status: f.Status}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"versions": meta})
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
