// Package transport provides the core interfaces and types shared between
// the public pkg/transport package and internal handler implementations.
package transport

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// WorkflowHandler maps workflow IDs to Runnables.
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

// MessageBroker is the pub/sub primitive backing StreamTransport.
type MessageBroker interface {
	Publish(ctx context.Context, topic string, msg []byte) error
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
	Close() error
}
