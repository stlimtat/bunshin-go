package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func TestNewFallbackProvider_NoProviders(t *testing.T) {
	_, err := llm.NewFallbackProvider("fallback")
	if err == nil {
		t.Fatal("expected error when no providers given, got nil")
	}
}

func TestFallbackProvider_ID(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "ok")
	fp, err := llm.NewFallbackProvider("fallback", p1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp.ID() != "fallback" {
		t.Errorf("expected ID 'fallback', got %q", fp.ID())
	}
}

func TestFallbackProvider_Complete_FirstSucceeds(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "hello")
	p2 := llm.NewFakeProvider("p2", "world")
	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)

	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}}
	resp, err := fp.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got %q", resp.Content)
	}
	if p1.CallCount != 1 {
		t.Errorf("expected p1 called once, got %d", p1.CallCount)
	}
	if p2.CallCount != 0 {
		t.Errorf("expected p2 not called, got %d", p2.CallCount)
	}
}

func TestFallbackProvider_Complete_SkipsFailedFirst(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "hello")
	p1.Err = errors.New("p1 unavailable")
	p2 := llm.NewFakeProvider("p2", "world")
	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)

	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}}
	resp, err := fp.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "world" {
		t.Errorf("expected 'world', got %q", resp.Content)
	}
	if p2.CallCount != 1 {
		t.Errorf("expected p2 called once, got %d", p2.CallCount)
	}
}

func TestFallbackProvider_Complete_AllFail(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "")
	p1.Err = errors.New("p1 down")
	p2 := llm.NewFakeProvider("p2", "")
	p2.Err = errors.New("p2 down")
	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)

	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}}
	_, err := fp.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when all providers fail, got nil")
	}
	// The combined error should mention both providers.
	errStr := err.Error()
	if !containsStr(errStr, "p1") || !containsStr(errStr, "p2") {
		t.Errorf("expected combined error to mention both providers, got: %s", errStr)
	}
}

func TestFallbackProvider_StreamComplete_FallsBack(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "")
	p1.Err = errors.New("stream open failed")
	p2 := llm.NewFakeProvider("p2", "streamed")
	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)

	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}}
	ch, err := fp.StreamComplete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var delta string
	for chunk := range ch {
		if !chunk.Done {
			delta += chunk.Delta
		}
	}
	if delta != "streamed" {
		t.Errorf("expected 'streamed', got %q", delta)
	}
}

func TestFallbackProvider_WithRegistry_FiltersAvailable(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "from-p1")
	p2 := llm.NewFakeProvider("p2", "from-p2")

	// Registry with only p2 available.
	reg := llm.NewProviderRegistry(p1, p2)
	// Manually start with a cancelled context so background loop doesn't interfere,
	// but we rely on the registry's pingAll having run. We use a pre-cancelled ctx
	// so Start returns quickly.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	// p1 and p2 are non-Pinger FakeProviders → assumed available after Start.
	reg.Start(cancelledCtx)

	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)
	fp.WithRegistry(reg)

	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}}
	resp, err := fp.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both are available, so p1 should be tried first.
	if resp.Content != "from-p1" {
		t.Errorf("expected 'from-p1', got %q", resp.Content)
	}
}

func TestFallbackProvider_WithRegistry_FallsBackWhenNoneAvailable(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "from-p1")
	p2 := llm.NewFakeProvider("p2", "from-p2")

	// Registry with no providers started — available map is empty.
	reg := llm.NewProviderRegistry() // no providers registered

	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)
	fp.WithRegistry(reg)

	// Registry marks neither p1 nor p2 available (they aren't registered in it).
	// candidates() should fall back to all providers.
	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}}
	resp, err := fp.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "from-p1" {
		t.Errorf("expected 'from-p1' as last-resort fallback, got %q", resp.Content)
	}
}

func TestFallbackProvider_CanTransferContext(t *testing.T) {
	p1 := llm.NewFakeProvider("same", "")
	p2 := llm.NewFakeProvider("same", "")
	fp, _ := llm.NewFallbackProvider("fallback", p1, p2)

	from := llm.NewFakeProvider("same", "")
	if !fp.CanTransferContext(from) {
		t.Error("expected CanTransferContext true when all providers match")
	}

	other := llm.NewFakeProvider("other", "")
	if fp.CanTransferContext(other) {
		t.Error("expected CanTransferContext false when provider IDs differ")
	}
}

func TestFallbackProvider_NativeMessages(t *testing.T) {
	p1 := llm.NewFakeProvider("p1", "")
	fp, _ := llm.NewFallbackProvider("fallback", p1)

	msgs := []llm.Message{llm.NewTextMessage(llm.RoleUser, "hello")}
	got, err := fp.NativeMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil native messages")
	}
}

// containsStr is a helper to check substring presence.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
