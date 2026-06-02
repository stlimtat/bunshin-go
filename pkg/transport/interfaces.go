// Package transport provides server interfaces for exposing bunshin-go workflows
// over the network.
//
// Three transport modes:
//
//	GRPCTransport  — gRPC/HTTP2. Bidirectional streaming via ExecuteStream RPC.
//	               Best for service-to-service calls, microservice architectures.
//
//	HTTPTransport  — HTTP/2 with Server-Sent Events (SSE) for streaming LLM token
//	               output to browser clients. Also exposes a synchronous POST endpoint.
//
//	StreamTransport — Abstract pub/sub interface. Backed by Kafka, NATS, or WebSocket.
//	                  Useful for event-driven architectures and async workflows.
//
// All transports share the WorkflowHandler interface — implement once, expose anywhere.
package transport

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// WorkflowHandler maps workflow IDs to Runnables.
// The transport calls Invoke or Stream on the matching Runnable.
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
