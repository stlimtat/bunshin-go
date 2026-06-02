package core

// State is the envelope passed between Runnables at composition seams.
//
// Data holds the typed workflow payload for a specific workflow.
// Meta carries cross-cutting concerns: trace IDs, session tokens, routing hints,
// cost budgets — anything that should flow transparently through the pipeline
// without polluting the domain struct.
//
// Errors are NOT carried in State. Runnable.Invoke returns (any, error) directly;
// the executor decides whether to halt, retry, or route to a recovery node.
type State[S any] struct {
	// Data is the typed payload. Define one struct per workflow.
	Data S

	// Meta carries telemetry, session, and routing metadata.
	// Keys are namespaced by convention: "bunshin.trace_id", "bunshin.session_id".
	Meta map[string]any
}

// NewState constructs a State with an initialised Meta map.
func NewState[S any](data S) State[S] {
	return State[S]{Data: data, Meta: make(map[string]any)}
}

// WithMeta returns a shallow copy of State with the given key set in Meta.
// Use for immutable state updates in reducer-style workflows.
func (s State[S]) WithMeta(key string, value any) State[S] {
	next := State[S]{Data: s.Data, Meta: make(map[string]any, len(s.Meta)+1)}
	for k, v := range s.Meta {
		next.Meta[k] = v
	}
	next.Meta[key] = value
	return next
}

// GetMeta returns the Meta value for key, and whether it was present.
func (s State[S]) GetMeta(key string) (any, bool) {
	v, ok := s.Meta[key]
	return v, ok
}

// Well-known Meta keys used by bunshin-go internals.
const (
	MetaTraceID    = "bunshin.trace_id"
	MetaSessionID  = "bunshin.session_id"
	MetaRunID      = "bunshin.run_id"
	MetaThreadID   = "bunshin.thread_id"   // horizontal-scale coordination key
	MetaCostBudget = "bunshin.cost_budget" // remaining token/dollar budget
)
