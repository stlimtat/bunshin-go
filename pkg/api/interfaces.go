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
//	POST   /v1/workflows/{id}         — synchronous workflow execution
//	GET    /v1/workflows/{id}/stream  — SSE streaming (llm_token + step events)
//	GET    /v1/threads                — list conversation threads
//	GET    /v1/threads/{id}/messages  — thread message history
//	POST   /v1/prompts/{name}/activate — activate a prompt version
//	POST   /v1/prompts/refresh        — force prompt cache refresh on this node
package api

import (
	"context"
	"net/http"

	"github.com/stlimtat/bunshin-go/pkg/transport"
)

// PromptActivator promotes the newest draft of a named fragment to active.
// Implement this with prompt.PostgresStore.Promote for a full implementation.
type PromptActivator interface {
	Promote(ctx context.Context, name string) error
}

// PromptRefresher triggers an immediate pull from Redis into the in-process snapshot.
// Implement this with prompt.PromptCache.Refresh.
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
}

// Router mounts all /v1 routes onto mux.
type Router struct {
	handler   transport.WorkflowHandler
	activator PromptActivator
	refresher PromptRefresher
}

// NewRouter returns a Router backed by handler.
func NewRouter(handler transport.WorkflowHandler, cfg ...RouterConfig) *Router {
	ro := &Router{handler: handler}
	for _, c := range cfg {
		ro.activator = c.Activator
		ro.refresher = c.Refresher
	}
	return ro
}

// Mount registers all versioned API routes on mux.
func (ro *Router) Mount(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workflows/{id}", ro.handleWorkflowInvoke)
	mux.HandleFunc("GET /v1/workflows/{id}/stream", ro.handleWorkflowStream)
	mux.HandleFunc("GET /v1/threads", ro.handleListThreads)
	mux.HandleFunc("GET /v1/threads/{id}/messages", ro.handleGetThreadMessages)
	mux.HandleFunc("POST /v1/prompts/{name}/activate", ro.handlePromptActivate)
	mux.HandleFunc("POST /v1/prompts/refresh", ro.handlePromptRefresh)
}
