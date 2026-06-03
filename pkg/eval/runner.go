package eval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// EvalRunner runs a dataset through a function and scores the results.
type EvalRunner struct {
	fn          func(ctx context.Context, input map[string]any) (map[string]any, error)
	evaluators  []Evaluator
	concurrency int
	logger      zerolog.Logger
}

// NewEvalRunner constructs an EvalRunner.
// fn is the function under evaluation (e.g. a Chain or Agent invocation).
// concurrency controls how many examples run in parallel (default: 5).
func NewEvalRunner(
	fn func(ctx context.Context, input map[string]any) (map[string]any, error),
	evaluators []Evaluator,
	concurrency int,
) *EvalRunner {
	if concurrency <= 0 {
		concurrency = 5
	}
	return &EvalRunner{fn: fn, evaluators: evaluators, concurrency: concurrency, logger: zerolog.Nop()}
}

// WithLogger sets the logger for the EvalRunner.
func (r *EvalRunner) WithLogger(logger zerolog.Logger) *EvalRunner {
	r.logger = logger
	return r
}

// Run evaluates all examples in dataset and returns a report.
func (r *EvalRunner) Run(ctx context.Context, dataset *Dataset) (*EvalReport, error) {
	report := &EvalReport{
		ID:        uuid.New(),
		DatasetID: dataset.ID,
		ScoreMap:  make(map[uuid.UUID][]*Score),
		StartTime: time.Now(),
	}

	sem := make(chan struct{}, r.concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ex := range dataset.Examples {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(ex *Example) {
			defer wg.Done()
			defer func() { <-sem }()

			run := r.runExample(ctx, ex)

			var scores []*Score
			for _, ev := range r.evaluators {
				s, err := ev.Evaluate(ctx, run)
				if err != nil {
					r.logger.Error().
						Str("evaluator", ev.Name()).
						Str("example_id", ex.ID.String()).
						Err(err).
						Msg("evaluator failed")
					s = &Score{Key: ev.Name(), Value: 0, Comment: fmt.Sprintf("evaluator error: %v", err)}
				}
				if s != nil {
					scores = append(scores, s)
				}
			}

			mu.Lock()
			report.Runs = append(report.Runs, run)
			report.ScoreMap[ex.ID] = scores
			mu.Unlock()
		}(ex)
	}
	wg.Wait()
	report.EndTime = time.Now()
	report.Scores = aggregateScores(report.ScoreMap, r.evaluators)
	return report, nil
}

func (r *EvalRunner) runExample(ctx context.Context, ex *Example) *EvalRun {
	start := time.Now()
	actual, err := r.fn(ctx, ex.Input)
	return &EvalRun{
		ExampleID: ex.ID,
		Input:     ex.Input,
		Actual:    actual,
		Reference: ex.Reference,
		RunID:     uuid.New(),
		Latency:   time.Since(start),
		Err:       err,
	}
}

// aggregateScores computes mean score per evaluator across all examples.
func aggregateScores(scoreMap map[uuid.UUID][]*Score, evaluators []Evaluator) map[string]float64 {
	sums := make(map[string]float64)
	counts := make(map[string]int)
	for _, scores := range scoreMap {
		for _, s := range scores {
			sums[s.Key] += s.Value
			counts[s.Key]++
		}
	}
	means := make(map[string]float64, len(evaluators))
	for _, ev := range evaluators {
		if n := counts[ev.Name()]; n > 0 {
			means[ev.Name()] = sums[ev.Name()] / float64(n)
		}
	}
	return means
}
