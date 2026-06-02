package checkpoint

import (
	"context"
	"fmt"
	"sync"
)

// MemoryCheckpointer stores checkpoints in-process.
type MemoryCheckpointer struct {
	mu   sync.RWMutex
	data map[string][]*Checkpoint // threadID → checkpoints (newest last)
}

// NewMemoryCheckpointer constructs an empty MemoryCheckpointer.
func NewMemoryCheckpointer() *MemoryCheckpointer {
	return &MemoryCheckpointer{data: make(map[string][]*Checkpoint)}
}

func (m *MemoryCheckpointer) Save(_ context.Context, cp *Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[cp.ThreadID] = append(m.data[cp.ThreadID], cp)
	return nil
}

func (m *MemoryCheckpointer) Load(_ context.Context, threadID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cps := m.data[threadID]
	if len(cps) == 0 {
		return nil, fmt.Errorf("%w: thread=%q", ErrNoCheckpoint, threadID)
	}
	return cps[len(cps)-1], nil
}

func (m *MemoryCheckpointer) Delete(_ context.Context, threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, threadID)
	return nil
}

func (m *MemoryCheckpointer) History(_ context.Context, threadID string) ([]*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cps := m.data[threadID]
	out := make([]*Checkpoint, len(cps))
	copy(out, cps)
	return out, nil
}

// MemoryJournal stores journal entries in-process.
type MemoryJournal struct {
	mu   sync.RWMutex
	data map[string][]*JournalEntry
}

// NewMemoryJournal constructs an empty MemoryJournal.
func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{data: make(map[string][]*JournalEntry)}
}

func (j *MemoryJournal) Append(_ context.Context, entry *JournalEntry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.data[entry.ThreadID] = append(j.data[entry.ThreadID], entry)
	return nil
}

func (j *MemoryJournal) Entries(_ context.Context, threadID string) ([]*JournalEntry, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	entries := j.data[threadID]
	out := make([]*JournalEntry, len(entries))
	copy(out, entries)
	return out, nil
}

func (j *MemoryJournal) Clear(_ context.Context, threadID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.data, threadID)
	return nil
}
