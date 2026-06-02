package transport

import (
	"fmt"
	"sync"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

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
