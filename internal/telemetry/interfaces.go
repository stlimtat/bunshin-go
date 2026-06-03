// Package telemetry provides LLM-specific observability via LangSmith
// and infrastructure metrics via OpenTelemetry.
//
// LangSmith (primary) captures: run trees, chains, LLM calls, tool calls,
// prompt renders, token usage, feedback, and cost. Every Runnable invocation
// is a "Run" with a parent-child relationship, forming a trace tree.
//
// OpenTelemetry (secondary) captures: request latency, error rates, and system
// metrics for infrastructure alerting (Datadog, Honeycomb, Grafana Cloud).
//
// RunIDs are propagated through context.Context so child Runnables automatically
// parent their runs correctly.
package telemetry

import (
	"context"

	"github.com/google/uuid"
)

// TelemetryBackend receives tracing events.
// Multiple backends can be composed with MultiBackend.
type TelemetryBackend interface {
	// StartRun records the start of a run. Returns a context with the run ID injected.
	StartRun(ctx context.Context, run *Run) (context.Context, error)
	// EndRun records the end of a run with its outputs and optional error.
	EndRun(ctx context.Context, runID uuid.UUID, outputs map[string]any, err error) error
	// AddFeedback attaches feedback to a completed run.
	AddFeedback(ctx context.Context, runID uuid.UUID, feedback Feedback) error
	// Flush ensures all buffered events are sent. Call before process exit.
	Flush(ctx context.Context) error
}
