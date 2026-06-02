package eval

import (
	"time"

	"github.com/google/uuid"
)

// Example is one test case: an input and the expected output.
type Example struct {
	ID        uuid.UUID
	Input     map[string]any
	Reference map[string]any
	Tags      []string
	Metadata  map[string]any
}

// Dataset is a named collection of Examples.
type Dataset struct {
	ID       uuid.UUID
	Name     string
	Examples []*Example
}

// EvalRun is the result of running one Example through a Runnable.
type EvalRun struct {
	ExampleID uuid.UUID
	Input     map[string]any
	Actual    map[string]any
	Reference map[string]any
	// RunID links this eval run back to a LangSmith trace.
	RunID   uuid.UUID
	Latency time.Duration
	Err     error
}

// Score is the result of one Evaluator assessing one EvalRun.
type Score struct {
	Key     string  // e.g. "correctness", "faithfulness"
	Value   float64 // 0.0–1.0
	Comment string
}

// EvalReport aggregates results from a full dataset run.
type EvalReport struct {
	ID        uuid.UUID
	DatasetID uuid.UUID
	Scores    map[string]float64       // evaluator name → mean score
	Runs      []*EvalRun
	ScoreMap  map[uuid.UUID][]*Score   // example ID → scores
	StartTime time.Time
	EndTime   time.Time
}
