// Package telemetry provides LangSmith run tracing and OTEL propagation.
//
// Each workflow invocation carries a RunContext in its context.Context.
// Middleware (WithLangSmith) creates the root RunContext; Nodes read it to
// post child run spans back to LangSmith.
//
// The RunContext is intentionally thin — it carries IDs only. Callers post
// run data via a LangSmithClient (injected separately), keeping this package
// free of HTTP dependencies.
package telemetry

import "github.com/google/uuid"

// RunContext holds the LangSmith trace identifiers for a single workflow run.
// It flows through context.Context so every node in the graph can attach
// child spans without explicit parameter threading.
type RunContext struct {
	// RunID is the unique identifier for this execution node's span.
	RunID string
	// ParentRunID is the RunID of the calling span, empty for the root.
	ParentRunID string
	// ProjectName is the LangSmith project to send spans to.
	ProjectName string
	// TraceID is the root run ID — shared by all spans in one workflow invocation.
	TraceID string
}

// NewRunContext creates a root RunContext for a new workflow invocation.
func NewRunContext(project string) RunContext {
	id := uuid.New().String()
	return RunContext{
		RunID:       id,
		ParentRunID: "",
		ProjectName: project,
		TraceID:     id,
	}
}

// Child creates a child RunContext for a nested span (e.g. a node or tool call).
func (rc RunContext) Child() RunContext {
	return RunContext{
		RunID:       uuid.New().String(),
		ParentRunID: rc.RunID,
		ProjectName: rc.ProjectName,
		TraceID:     rc.TraceID,
	}
}
