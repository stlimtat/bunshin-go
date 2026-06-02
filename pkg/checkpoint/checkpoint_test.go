package checkpoint_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/checkpoint"
)

func makeCheckpoint(threadID string, stepIndex int) *checkpoint.Checkpoint {
	return &checkpoint.Checkpoint{
		WorkflowID: "wf-1",
		ThreadID:   threadID,
		StepIndex:  stepIndex,
		StepID:     "step-" + string(rune('A'+stepIndex)),
		State:      json.RawMessage(`{"count":` + string(rune('0'+stepIndex)) + `}`),
		CreatedAt:  time.Now(),
	}
}

// ---- MemoryCheckpointer ----

func TestMemoryCheckpointer_SaveAndLoad(t *testing.T) {
	cp := checkpoint.NewMemoryCheckpointer()
	_ = cp.Save(context.Background(), makeCheckpoint("t1", 2))

	got, err := cp.Load(context.Background(), "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StepIndex != 2 {
		t.Fatalf("want StepIndex=2, got %d", got.StepIndex)
	}
}

func TestMemoryCheckpointer_Load_NoCheckpoint(t *testing.T) {
	cp := checkpoint.NewMemoryCheckpointer()
	_, err := cp.Load(context.Background(), "nonexistent")
	if !errors.Is(err, checkpoint.ErrNoCheckpoint) {
		t.Fatalf("want ErrNoCheckpoint, got %v", err)
	}
}

func TestMemoryCheckpointer_Load_ReturnsLatest(t *testing.T) {
	cp := checkpoint.NewMemoryCheckpointer()
	_ = cp.Save(context.Background(), makeCheckpoint("t1", 0))
	_ = cp.Save(context.Background(), makeCheckpoint("t1", 3))
	_ = cp.Save(context.Background(), makeCheckpoint("t1", 7))

	got, _ := cp.Load(context.Background(), "t1")
	if got.StepIndex != 7 {
		t.Fatalf("want latest StepIndex=7, got %d", got.StepIndex)
	}
}

func TestMemoryCheckpointer_Delete(t *testing.T) {
	cp := checkpoint.NewMemoryCheckpointer()
	_ = cp.Save(context.Background(), makeCheckpoint("t1", 0))
	_ = cp.Delete(context.Background(), "t1")
	_, err := cp.Load(context.Background(), "t1")
	if !errors.Is(err, checkpoint.ErrNoCheckpoint) {
		t.Fatalf("want ErrNoCheckpoint after delete, got %v", err)
	}
}

func TestMemoryCheckpointer_History(t *testing.T) {
	cp := checkpoint.NewMemoryCheckpointer()
	for i := 0; i < 5; i++ {
		_ = cp.Save(context.Background(), makeCheckpoint("t1", i))
	}
	history, _ := cp.History(context.Background(), "t1")
	if len(history) != 5 {
		t.Fatalf("want 5 history entries, got %d", len(history))
	}
	// Oldest first.
	if history[0].StepIndex != 0 || history[4].StepIndex != 4 {
		t.Fatalf("unexpected ordering: %v", history)
	}
}

func TestMemoryCheckpointer_MultipleThreads(t *testing.T) {
	cp := checkpoint.NewMemoryCheckpointer()
	_ = cp.Save(context.Background(), makeCheckpoint("tA", 1))
	_ = cp.Save(context.Background(), makeCheckpoint("tB", 2))

	a, _ := cp.Load(context.Background(), "tA")
	b, _ := cp.Load(context.Background(), "tB")
	if a.StepIndex != 1 || b.StepIndex != 2 {
		t.Fatalf("threads interfere: a=%d b=%d", a.StepIndex, b.StepIndex)
	}
}

// ---- MemoryJournal ----

func TestMemoryJournal_AppendAndEntries(t *testing.T) {
	j := checkpoint.NewMemoryJournal()
	_ = j.Append(context.Background(), &checkpoint.JournalEntry{
		ThreadID: "t1", StepIndex: 0, StepID: "step-A",
	})
	_ = j.Append(context.Background(), &checkpoint.JournalEntry{
		ThreadID: "t1", StepIndex: 1, StepID: "step-B",
	})

	entries, _ := j.Entries(context.Background(), "t1")
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
}

func TestMemoryJournal_Clear(t *testing.T) {
	j := checkpoint.NewMemoryJournal()
	_ = j.Append(context.Background(), &checkpoint.JournalEntry{ThreadID: "t1"})
	_ = j.Clear(context.Background(), "t1")

	entries, _ := j.Entries(context.Background(), "t1")
	if len(entries) != 0 {
		t.Fatalf("want 0 entries after clear, got %d", len(entries))
	}
}

func TestMemoryJournal_EmptyEntries(t *testing.T) {
	j := checkpoint.NewMemoryJournal()
	entries, err := j.Entries(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("want 0, got %d", len(entries))
	}
}
