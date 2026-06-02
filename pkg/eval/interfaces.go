// Package eval provides the bunshin-go evaluation harness.
//
// The eval system operates in two modes:
//
//  1. Standalone: run EvalRunner.Run against a local Dataset; results written
//     to disk as JSON. No external services required.
//
//  2. LangSmith-synced: push Dataset to LangSmith before a run, push EvalReport
//     afterwards. Results appear as an Experiment in the LangSmith UI.
//
// Built-in Evaluators: ExactMatch, ContainsAll, Latency.
package eval

import "context"

// Evaluator assesses one EvalRun and returns a Score.
type Evaluator interface {
	Name() string
	Evaluate(ctx context.Context, run *EvalRun) (*Score, error)
}

// DatasetBackend stores and retrieves Datasets and EvalReports.
type DatasetBackend interface {
	Push(ctx context.Context, dataset *Dataset) error
	Pull(ctx context.Context, name string) (*Dataset, error)
	PushResults(ctx context.Context, report *EvalReport) error
	ListDatasets(ctx context.Context) ([]*Dataset, error)
}
