// Package transport provides server interfaces for exposing bunshin-go workflows
// over the network.
//
// Three transport modes:
//
//	HTTPTransport   — HTTP with Server-Sent Events (SSE) for streaming LLM token
//	                  output to browser clients. Also exposes a synchronous POST endpoint.
//
//	StreamTransport — Abstract pub/sub interface. Backed by Kafka, NATS, or WebSocket.
//	                  Useful for event-driven architectures and async workflows.
//
// All transports share the WorkflowHandler interface — implement once, expose anywhere.
package transport

import itransport "github.com/stlimtat/bunshin-go/internal/transport"

// WorkflowHandler maps workflow IDs to Runnables.
type WorkflowHandler = itransport.WorkflowHandler

// Transport is the interface all server backends implement.
type Transport = itransport.Transport

// MessageBroker is the pub/sub primitive backing StreamTransport.
type MessageBroker = itransport.MessageBroker
