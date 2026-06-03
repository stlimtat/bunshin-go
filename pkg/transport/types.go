package transport

import itransport "github.com/stlimtat/bunshin-go/internal/transport"

// WorkflowRequest is the canonical input to a workflow execution over the wire.
type WorkflowRequest = itransport.WorkflowRequest

// WorkflowResponse is the synchronous response from a workflow execution.
type WorkflowResponse = itransport.WorkflowResponse

// StreamEvent is one event in a workflow execution stream.
type StreamEvent = itransport.StreamEvent
