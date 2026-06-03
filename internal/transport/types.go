package transport

// WorkflowRequest is the canonical input to a workflow execution over the wire.
type WorkflowRequest struct {
	WorkflowID string         `json:"workflow_id"`
	ThreadID   string         `json:"thread_id,omitempty"`
	Input      map[string]any `json:"input"`
}

// WorkflowResponse is the synchronous response from a workflow execution.
type WorkflowResponse struct {
	ThreadID string         `json:"thread_id"`
	Output   map[string]any `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// StreamEvent is one event in a workflow execution stream.
type StreamEvent struct {
	Type   string `json:"type"`
	StepID string `json:"step_id,omitempty"`
	Token  string `json:"token,omitempty"`
	Output any    `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
