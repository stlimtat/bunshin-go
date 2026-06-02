package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// feedbackRecord pairs feedback with the run it belongs to.
type feedbackRecord struct {
	RunID    uuid.UUID
	Feedback Feedback
}

// MemoryBackend is an in-process TelemetryBackend for tests.
// It records all runs and supports inspection.
type MemoryBackend struct {
	mu       sync.RWMutex
	runs     map[uuid.UUID]*Run
	feedback []feedbackRecord
	order    []uuid.UUID
}

// NewMemoryBackend constructs an empty MemoryBackend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{runs: make(map[uuid.UUID]*Run)}
}

func (b *MemoryBackend) StartRun(ctx context.Context, run *Run) (context.Context, error) {
	run.StartTime = time.Now()
	if parentID := RunIDFromContext(ctx); parentID != uuid.Nil && run.ParentID == nil {
		id := parentID
		run.ParentID = &id
	}
	b.mu.Lock()
	b.runs[run.ID] = run
	b.order = append(b.order, run.ID)
	b.mu.Unlock()
	return WithRunID(ctx, run.ID), nil
}

func (b *MemoryBackend) EndRun(_ context.Context, runID uuid.UUID, outputs map[string]any, err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	run, ok := b.runs[runID]
	if !ok {
		return nil
	}
	now := time.Now()
	run.EndTime = &now
	run.Outputs = outputs
	if err != nil {
		s := err.Error()
		run.Error = &s
	}
	return nil
}

func (b *MemoryBackend) AddFeedback(_ context.Context, runID uuid.UUID, f Feedback) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.feedback = append(b.feedback, feedbackRecord{RunID: runID, Feedback: f})
	return nil
}

func (b *MemoryBackend) Flush(_ context.Context) error { return nil }

// GetRun returns the run with the given ID.
func (b *MemoryBackend) GetRun(id uuid.UUID) (*Run, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	r, ok := b.runs[id]
	return r, ok
}

// Runs returns all runs in insertion order.
func (b *MemoryBackend) Runs() []*Run {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]*Run, 0, len(b.order))
	for _, id := range b.order {
		out = append(out, b.runs[id])
	}
	return out
}

// MultiBackend fans out events to multiple backends.
type MultiBackend struct {
	backends []TelemetryBackend
}

// NewMultiBackend constructs a MultiBackend.
func NewMultiBackend(backends ...TelemetryBackend) *MultiBackend {
	return &MultiBackend{backends: backends}
}

func (m *MultiBackend) StartRun(ctx context.Context, run *Run) (context.Context, error) {
	var lastErr error
	for _, b := range m.backends {
		c, err := b.StartRun(ctx, run)
		if err != nil {
			lastErr = err
		} else {
			ctx = c
		}
	}
	return ctx, lastErr
}

func (m *MultiBackend) EndRun(ctx context.Context, runID uuid.UUID, outputs map[string]any, err error) error {
	var lastErr error
	for _, b := range m.backends {
		if e := b.EndRun(ctx, runID, outputs, err); e != nil {
			lastErr = e
		}
	}
	return lastErr
}

func (m *MultiBackend) AddFeedback(ctx context.Context, runID uuid.UUID, f Feedback) error {
	var lastErr error
	for _, b := range m.backends {
		if e := b.AddFeedback(ctx, runID, f); e != nil {
			lastErr = e
		}
	}
	return lastErr
}

func (m *MultiBackend) Flush(ctx context.Context) error {
	var lastErr error
	for _, b := range m.backends {
		if e := b.Flush(ctx); e != nil {
			lastErr = e
		}
	}
	return lastErr
}
