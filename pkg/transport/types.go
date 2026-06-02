package transport

// WorkflowRequest is the canonical input to a workflow execution over the wire.
type WorkflowRequest struct {
	// WorkflowID identifies which workflow to run.
	WorkflowID string `json:"workflow_id"`
	// ThreadID is the horizontal-scale coordination key for checkpoint/resume.
	ThreadID string `json:"thread_id,omitempty"`
	// Input is the workflow-specific input payload.
	Input map[string]any `json:"input"`
}

// WorkflowResponse is the synchronous response from a workflow execution.
type WorkflowResponse struct {
	ThreadID string         `json:"thread_id"`
	Output   map[string]any `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// StreamEvent is one event in a workflow execution stream.
type StreamEvent struct {
	// Type classifies the event: "step_start", "llm_token", "step_end", "error", "done".
	Type   string `json:"type"`
	StepID string `json:"step_id,omitempty"`
	Token  string `json:"token,omitempty"`
	Output any    `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
