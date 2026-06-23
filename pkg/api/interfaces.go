// Package api provides versioned REST API handlers built on pkg/transport.
//
// All application endpoints carry a /v1 prefix. Probe and metrics endpoints
// live in pkg/probe and carry no version prefix.
//
// This package is the reference implementation of the bunshin HTTP API. Users
// who need custom routing should build on pkg/transport directly and use this
// package as a guide.
//
// Registered routes:
//
//	POST   /v1/workflows                          — create draft (returns version)
//	GET    /v1/workflows                          — list workflow names
//	GET    /v1/workflows/{name}                   — get active spec
//	GET    /v1/workflows/{name}/versions          — list versions (oldest first)
//	GET    /v1/workflows/{name}/versions/{ver}    — get specific version
//	POST   /v1/workflows/{name}/activate          — promote version to active
//	DELETE /v1/workflows/{name}                   — soft delete
//	POST   /v1/workflows/{name}/invoke            — synchronous workflow execution
//	GET    /v1/workflows/{name}/stream            — SSE streaming
//	GET    /v1/threads                            — list conversation threads
//	GET    /v1/threads/{id}/messages              — thread message history
//	PUT    /v1/prompts/{slug}                     — upsert (create or update draft, optional rename)
//	GET    /v1/prompts                            — list active fragments for tenant
//	GET    /v1/prompts/{slug}                     — get active version by slug
//	GET    /v1/prompts/id/{id}                    — get by immutable UUID
//	GET    /v1/prompts/{slug}/versions            — list all versions, metadata only, newest-first
//	GET    /v1/prompts/{slug}/versions/{ver}      — get specific version (any status)
//	POST   /v1/prompts/{slug}/activate            — promote newest draft → active
//	DELETE /v1/prompts/{slug}                     — soft delete + cache refresh
//	POST   /v1/prompts/{slug}/purge               — hard delete (501 stub)
//	POST   /v1/prompts/refresh                    — force in-process cache refresh
package api

import (
	"net/http"

	"github.com/stlimtat/bunshin-go/pkg/prompt"
	"github.com/stlimtat/bunshin-go/pkg/transport"
	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// PromptRefresher triggers an immediate pull from Redis into the in-process snapshot.
type PromptRefresher interface {
	Refresh()
}

// RouterConfig holds optional backend integrations for the Router.
// All fields are optional — unset fields result in 501 Not Implemented responses.
type RouterConfig struct {
	// PromptBackend enables prompt CRUD endpoints.
	// When nil, CRUD endpoints return 501 Not Implemented.
	PromptBackend prompt.PromptBackend
	// PromptActivator promotes prompt drafts to active.
	// When nil, activate endpoint returns 501 Not Implemented.
	PromptActivator prompt.PromptActivator
	// Refresher triggers a prompt cache refresh.
	Refresher PromptRefresher
	// WorkflowStore enables workflow CRUD endpoints.
	// When nil, CRUD endpoints return 501 Not Implemented.
	WorkflowStore workflow.Store
	// WorkflowTenantID is the tenant used for all workflow CRUD requests.
	// Defaults to "default" when empty.
	WorkflowTenantID string
}

// Router mounts all /v1 routes onto mux.
type Router struct {
	handler          transport.WorkflowHandler
	promptBackend    prompt.PromptBackend
	promptActivator  prompt.PromptActivator
	refresher        PromptRefresher
	workflowStore    workflow.Store
	workflowTenantID string
}

// NewRouter returns a Router backed by handler.
func NewRouter(handler transport.WorkflowHandler, cfg ...RouterConfig) *Router {
	ro := &Router{handler: handler, workflowTenantID: "default"}
	for _, c := range cfg {
		ro.promptBackend = c.PromptBackend
		ro.promptActivator = c.PromptActivator
		ro.refresher = c.Refresher
		ro.workflowStore = c.WorkflowStore
		if c.WorkflowTenantID != "" {
			ro.workflowTenantID = c.WorkflowTenantID
		}
	}
	return ro
}

// Mount registers all versioned API routes on mux.
func (ro *Router) Mount(mux *http.ServeMux) {
	// Workflow CRUD.
	mux.HandleFunc("POST /v1/workflows", ro.handleWorkflowCreate)
	mux.HandleFunc("GET /v1/workflows", ro.handleWorkflowList)
	mux.HandleFunc("GET /v1/workflows/{name}/versions", ro.handleWorkflowListVersions)
	mux.HandleFunc("GET /v1/workflows/{name}/versions/{ver}", ro.handleWorkflowGetVersion)
	mux.HandleFunc("POST /v1/workflows/{name}/activate", ro.handleWorkflowActivate)
	mux.HandleFunc("DELETE /v1/workflows/{name}", ro.handleWorkflowDelete)
	mux.HandleFunc("GET /v1/workflows/{name}", ro.handleWorkflowGet)

	// Workflow execution.
	mux.HandleFunc("POST /v1/workflows/{name}/invoke", ro.handleWorkflowInvoke)
	mux.HandleFunc("GET /v1/workflows/{name}/stream", ro.handleWorkflowStream)

	// Threads.
	mux.HandleFunc("GET /v1/threads", ro.handleListThreads)
	mux.HandleFunc("GET /v1/threads/{id}/messages", ro.handleGetThreadMessages)

	// Prompts.
	// Note: GET /v1/prompts/id/{id} and GET /v1/prompts/{slug}/versions both match
	// /v1/prompts/id/versions, which Go 1.22+ mux treats as a conflict. We resolve
	// this by serving the by-ID route via the catch-all /v1/prompts/{path...} and
	// dispatching to the right handler based on path shape.
	mux.HandleFunc("POST /v1/prompts/refresh", ro.handlePromptRefresh)
	mux.HandleFunc("GET /v1/prompts", ro.handlePromptList)
	mux.HandleFunc("PUT /v1/prompts/{slug}", ro.handlePromptUpsert)
	mux.HandleFunc("POST /v1/prompts/{slug}/activate", ro.handlePromptActivate)
	mux.HandleFunc("POST /v1/prompts/{slug}/purge", ro.handlePromptPurge)
	mux.HandleFunc("DELETE /v1/prompts/{slug}", ro.handlePromptDelete)
	// GET /v1/prompts/{path...} handles all GET prompt sub-paths:
	//   /v1/prompts/{slug}                 → get by slug
	//   /v1/prompts/id/{id}               → get by immutable UUID
	//   /v1/prompts/{slug}/versions        → list versions (501)
	//   /v1/prompts/{slug}/versions/{ver}  → get specific version
	mux.HandleFunc("GET /v1/prompts/{path...}", ro.handlePromptGet)
}
