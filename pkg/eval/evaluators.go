package eval

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExactMatch scores 1.0 if actual[key] == reference[key], 0.0 otherwise.
type ExactMatch struct {
	Key string
}

func (e *ExactMatch) Name() string { return "exact_match." + e.Key }

func (e *ExactMatch) Evaluate(_ context.Context, run *EvalRun) (*Score, error) {
	if run.Err != nil {
		return &Score{Key: e.Name(), Value: 0, Comment: "run error"}, nil
	}
	got, ref := fmt.Sprintf("%v", run.Actual[e.Key]), fmt.Sprintf("%v", run.Reference[e.Key])
	if got == ref {
		return &Score{Key: e.Name(), Value: 1}, nil
	}
	return &Score{Key: e.Name(), Value: 0, Comment: fmt.Sprintf("got %q, want %q", got, ref)}, nil
}

// ContainsAll scores 1.0 if every required phrase appears in actual[key].
type ContainsAll struct {
	Key      string
	Required []string
}

func (c *ContainsAll) Name() string { return "contains_all." + c.Key }

func (c *ContainsAll) Evaluate(_ context.Context, run *EvalRun) (*Score, error) {
	if run.Err != nil {
		return &Score{Key: c.Name(), Value: 0, Comment: "run error"}, nil
	}
	text := fmt.Sprintf("%v", run.Actual[c.Key])
	for _, phrase := range c.Required {
		if !strings.Contains(text, phrase) {
			return &Score{Key: c.Name(), Value: 0, Comment: fmt.Sprintf("missing phrase: %q", phrase)}, nil
		}
	}
	return &Score{Key: c.Name(), Value: 1}, nil
}

// Latency scores 1.0 if the run completed within MaxDuration, 0.0 otherwise.
type Latency struct {
	MaxDuration time.Duration
}

func (l *Latency) Name() string { return "latency" }

func (l *Latency) Evaluate(_ context.Context, run *EvalRun) (*Score, error) {
	if run.Latency <= l.MaxDuration {
		return &Score{Key: l.Name(), Value: 1}, nil
	}
	return &Score{
		Key:     l.Name(),
		Value:   0,
		Comment: fmt.Sprintf("latency %v exceeded max %v", run.Latency, l.MaxDuration),
	}, nil
}
