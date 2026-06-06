package checkpoint

import (
	"encoding/json"
	"errors"
	"time"
)

// RecoveryStrategy controls what happens when a workflow is resumed after failure.
type RecoveryStrategy string

const (
	// ResumeFromCheckpoint loads the last checkpoint and continues from the next step.
	ResumeFromCheckpoint RecoveryStrategy = "resume"
	// ReplayFromStart ignores checkpoints and re-executes from step 0.
	ReplayFromStart RecoveryStrategy = "replay"
)

// CheckpointFreq controls how often checkpoints are saved.
type CheckpointFreq int

const (
	// FreqAfterEachStep saves a checkpoint after every step.
	FreqAfterEachStep CheckpointFreq = 1
	// FreqOnCompletion saves only when the workflow completes successfully.
	FreqOnCompletion CheckpointFreq = -1
)

// FreqEveryN saves a checkpoint every N steps.
func FreqEveryN(n int) CheckpointFreq { return CheckpointFreq(n) }

// RecoveryConfig configures workflow recovery behaviour.
type RecoveryConfig struct {
	Strategy  RecoveryStrategy
	Frequency CheckpointFreq
	// MaxRetries is the number of times to retry a failing step before aborting.
	MaxRetries int
}

// DefaultRecoveryConfig is a safe default: resume from checkpoint, save after each step.
var DefaultRecoveryConfig = RecoveryConfig{
	Strategy:   ResumeFromCheckpoint,
	Frequency:  FreqAfterEachStep,
	MaxRetries: 3,
}

// Checkpoint is a serialised snapshot of workflow State at a specific step.
type Checkpoint struct {
	WorkflowID string
	// ThreadID is the horizontal-scale coordination key.
	ThreadID  string
	StepIndex int
	StepID    string
	// State is the JSON-encoded workflow state at this checkpoint.
	State json.RawMessage
	// Version is the optimistic-lock counter. Incremented on every Save.
	// Pass the value read from Load back into Save to detect concurrent writes.
	Version   int
	CreatedAt time.Time
}

// JournalEntry records one step's execution for audit and replay.
type JournalEntry struct {
	WorkflowID string
	ThreadID   string
	StepIndex  int
	StepID     string
	Inputs     map[string]any
	Outputs    map[string]any
	Err        string // empty if step succeeded
	StartTime  time.Time
	EndTime    time.Time
}

// ErrNoCheckpoint is returned by Checkpointer.Load when no checkpoint exists.
var ErrNoCheckpoint = errors.New("no checkpoint found")
