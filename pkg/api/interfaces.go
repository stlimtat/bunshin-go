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
//	POST   /v1/prompts/{name}/activate            — activate a prompt version
//	POST   /v1/prompts/refresh                    — force prompt cache refresh
package api

import (
	"context"
	"net/http"

	"github.com/stlimtat/bunshin-go/pkg/transport"
	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// PromptActivator promotes the newest draft of a named fragment to active.
type PromptActivator interface {
	Promote(ctx context.Context, name string) error
}

// PromptRefresher triggers an immediate pull from Redis into the in-process snapshot.
type PromptRefresher interface {
	Refresh()
}

// RouterConfig holds optional backend integrations for the Router.
// All fields are optional — unset fields result in 501 Not Implemented responses.
type RouterConfig struct {
	// Activator promotes prompt drafts to active.
	Activator PromptActivator
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
	activator        PromptActivator
	refresher        PromptRefresher
	workflowStore    workflow.Store
	workflowTenantID string
}

// NewRouter returns a Router backed by handler.
func NewRouter(handler transport.WorkflowHandler, cfg ...RouterConfig) *Router {
	ro := &Router{handler: handler, workflowTenantID: "default"}
	for _, c := range cfg {
		ro.activator = c.Activator
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

	// Workflow execution (renamed from /{id} to /{name}/invoke and /{name}/stream).
	mux.HandleFunc("POST /v1/workflows/{name}/invoke", ro.handleWorkflowInvoke)
	mux.HandleFunc("GET /v1/workflows/{name}/stream", ro.handleWorkflowStream)

	// Threads.
	mux.HandleFunc("GET /v1/threads", ro.handleListThreads)
	mux.HandleFunc("GET /v1/threads/{id}/messages", ro.handleGetThreadMessages)

	// Prompts.
	mux.HandleFunc("POST /v1/prompts/{name}/activate", ro.handlePromptActivate)
	mux.HandleFunc("POST /v1/prompts/refresh", ro.handlePromptRefresh)
}
