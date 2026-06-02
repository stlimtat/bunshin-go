package memory_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/memory"
)

func msg(role llm.Role, text string) llm.Message {
	return llm.NewTextMessage(role, text)
}

func TestMemoryStore_AppendAndLen(t *testing.T) {
	s := memory.NewMemoryStore()
	if s.Len() != 0 {
		t.Fatalf("want Len=0, got %d", s.Len())
	}
	_ = s.Append(context.Background(), msg(llm.RoleUser, "hello"))
	_ = s.Append(context.Background(), msg(llm.RoleAssistant, "hi"))
	if s.Len() != 2 {
		t.Fatalf("want Len=2, got %d", s.Len())
	}
}

func TestMemoryStore_Window_NoLimit(t *testing.T) {
	s := memory.NewMemoryStore()
	_ = s.Append(context.Background(), msg(llm.RoleUser, "a"))
	_ = s.Append(context.Background(), msg(llm.RoleUser, "b"))
	_ = s.Append(context.Background(), msg(llm.RoleUser, "c"))

	msgs, err := s.Window(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
}

func TestMemoryStore_Window_TokenBudget(t *testing.T) {
	// Each message "xxxx" = 4 chars = ~1 token.
	s := memory.NewMemoryStore(memory.WithTokenCounter(func(m llm.Message) int {
		return len(m.Text()) // 1 token per char for easy arithmetic
	}))
	_ = s.Append(context.Background(), msg(llm.RoleUser, "aaa"))  // 3 tokens
	_ = s.Append(context.Background(), msg(llm.RoleUser, "bb"))   // 2 tokens
	_ = s.Append(context.Background(), msg(llm.RoleUser, "cccc")) // 4 tokens

	// Budget=6: should fit "bb"(2) + "cccc"(4) but not "aaa" as well.
	msgs, err := s.Window(context.Background(), 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Text() != "bb" || msgs[1].Text() != "cccc" {
		t.Fatalf("unexpected messages: %v", msgs)
	}
}

func TestMemoryStore_Window_BudgetExceedsAll(t *testing.T) {
	s := memory.NewMemoryStore()
	_ = s.Append(context.Background(), msg(llm.RoleUser, "hi"))
	msgs, _ := s.Window(context.Background(), 999999)
	if len(msgs) != 1 {
		t.Fatalf("want 1, got %d", len(msgs))
	}
}

func TestMemoryStore_Window_Empty(t *testing.T) {
	s := memory.NewMemoryStore()
	msgs, err := s.Window(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("want 0, got %d", len(msgs))
	}
}

func TestMemoryStore_WindowFor_CachesNative(t *testing.T) {
	s := memory.NewMemoryStore()
	_ = s.Append(context.Background(), msg(llm.RoleUser, "test"))

	p := llm.NewFakeProvider(llm.ProviderOpenAI, "")
	req, err := s.WindowFor(context.Background(), p, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(req.Messages))
	}
	// Native cache should be populated.
	_, ok := req.Messages[0].Native(llm.ProviderOpenAI)
	if !ok {
		t.Fatal("expected native cache to be populated for OpenAI")
	}
}

func TestMemoryStore_ConcurrentAppend(t *testing.T) {
	s := memory.NewMemoryStore()
	var wg sync.WaitGroup
	const goroutines = 50
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = s.Append(context.Background(), msg(llm.RoleUser, "x"))
		}()
	}
	wg.Wait()
	if s.Len() != goroutines {
		t.Fatalf("want Len=%d after concurrent appends, got %d", goroutines, s.Len())
	}
}

func TestMemoryStore_SnapshotRestoreNoOp(t *testing.T) {
	s := memory.NewMemoryStore()
	_ = s.Append(context.Background(), msg(llm.RoleUser, "hi"))
	if err := s.Snapshot(context.Background()); err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if err := s.Restore(context.Background()); err != nil {
		t.Fatalf("Restore error: %v", err)
	}
	// Messages survive snapshot/restore on in-memory backend.
	if s.Len() != 1 {
		t.Fatalf("want Len=1 after restore, got %d", s.Len())
	}
}
