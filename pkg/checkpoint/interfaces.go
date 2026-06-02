// Package checkpoint implements workflow recovery via checkpointing and journaling.
//
// Two recovery strategies are supported, selected via RecoveryConfig.Strategy:
//
//	ResumeFromCheckpoint: Load the last saved Checkpoint for a ThreadID and
//	continue execution from the step after the checkpoint. Minimises re-work
//	after a node failure.
//
//	ReplayFromStart: Discard any checkpoint, re-execute all steps from the
//	beginning. Useful for debugging (deterministic replay) and for workflows
//	where partial state is unsafe to resume.
//
// ThreadID is the horizontal-scale coordination key. Any node that loads
// the same ThreadID from a shared Checkpointer (Redis, S3, DB) picks up
// exactly where another node left off.
package checkpoint

import "context"

// Checkpointer persists and retrieves workflow checkpoints.
type Checkpointer interface {
	// Save persists a checkpoint.
	Save(ctx context.Context, cp *Checkpoint) error
	// Load retrieves the most recent checkpoint for threadID.
	// Returns ErrNoCheckpoint if none exists.
	Load(ctx context.Context, threadID string) (*Checkpoint, error)
	// Delete removes all checkpoints for threadID.
	Delete(ctx context.Context, threadID string) error
	// History returns all saved checkpoints for threadID, oldest first.
	History(ctx context.Context, threadID string) ([]*Checkpoint, error)
}

// Journal persists step execution records for audit and replay.
type Journal interface {
	// Append adds a journal entry.
	Append(ctx context.Context, entry *JournalEntry) error
	// Entries returns all entries for threadID, in execution order.
	Entries(ctx context.Context, threadID string) ([]*JournalEntry, error)
	// Clear removes all entries for threadID.
	Clear(ctx context.Context, threadID string) error
}
