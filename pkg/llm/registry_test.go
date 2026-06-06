package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// fakePinger wraps FakeProvider and adds a configurable Ping method.
type fakePinger struct {
	*llm.FakeProvider
	pingErr error
}

func (fp *fakePinger) Ping(_ context.Context) error {
	return fp.pingErr
}

func TestNewProviderRegistry_Basic(t *testing.T) {
	p1 := &fakePinger{FakeProvider: llm.NewFakeProvider("p1", ""), pingErr: nil}
	reg := llm.NewProviderRegistry(p1)
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestProviderRegistry_IsAvailable_BeforeStart(t *testing.T) {
	p1 := &fakePinger{FakeProvider: llm.NewFakeProvider("p1", ""), pingErr: nil}
	reg := llm.NewProviderRegistry(p1)
	// Before Start, no ping has run → should be false.
	if reg.IsAvailable("p1") {
		t.Error("expected p1 unavailable before Start")
	}
}

func TestProviderRegistry_Start_PingsImmediately(t *testing.T) {
	p1 := &fakePinger{FakeProvider: llm.NewFakeProvider("p1", ""), pingErr: nil}
	reg := llm.NewProviderRegistry(p1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	if !reg.IsAvailable("p1") {
		t.Error("expected p1 available after Start with nil ping error")
	}
}

func TestProviderRegistry_PingFailed_MarksUnavailable(t *testing.T) {
	p1 := &fakePinger{
		FakeProvider: llm.NewFakeProvider("p1", ""),
		pingErr:      errors.New("connection refused"),
	}
	reg := llm.NewProviderRegistry(p1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	if reg.IsAvailable("p1") {
		t.Error("expected p1 unavailable when Ping returns error")
	}
}

func TestProviderRegistry_Available_ReturnsAllOnNoAvailable(t *testing.T) {
	p1 := &fakePinger{
		FakeProvider: llm.NewFakeProvider("p1", ""),
		pingErr:      errors.New("down"),
	}
	p2 := &fakePinger{
		FakeProvider: llm.NewFakeProvider("p2", ""),
		pingErr:      errors.New("down"),
	}
	reg := llm.NewProviderRegistry(p1, p2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	// All fail → Available() should return all providers as last resort.
	avail := reg.Available()
	if len(avail) != 2 {
		t.Errorf("expected 2 providers as last resort, got %d", len(avail))
	}
}

func TestProviderRegistry_Available_FiltersByPing(t *testing.T) {
	p1 := &fakePinger{FakeProvider: llm.NewFakeProvider("p1", ""), pingErr: nil}
	p2 := &fakePinger{
		FakeProvider: llm.NewFakeProvider("p2", ""),
		pingErr:      errors.New("down"),
	}
	reg := llm.NewProviderRegistry(p1, p2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	avail := reg.Available()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available provider, got %d", len(avail))
	}
	if avail[0].ID() != "p1" {
		t.Errorf("expected p1 available, got %s", avail[0].ID())
	}
}

func TestProviderRegistry_Status(t *testing.T) {
	p1 := &fakePinger{FakeProvider: llm.NewFakeProvider("p1", ""), pingErr: nil}
	p2 := &fakePinger{
		FakeProvider: llm.NewFakeProvider("p2", ""),
		pingErr:      errors.New("down"),
	}
	reg := llm.NewProviderRegistry(p1, p2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	status := reg.Status()
	if !status["p1"] {
		t.Error("expected p1 true in status")
	}
	if status["p2"] {
		t.Error("expected p2 false in status")
	}
}

func TestProviderRegistry_NonPinger_AlwaysAvailable(t *testing.T) {
	// FakeProvider does not implement Pinger → should be assumed available.
	p1 := llm.NewFakeProvider("p1", "response")
	reg := llm.NewProviderRegistry(p1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	if !reg.IsAvailable("p1") {
		t.Error("expected non-Pinger provider to be assumed available")
	}
}

func TestProviderRegistry_WithPingInterval(t *testing.T) {
	p1 := &fakePinger{FakeProvider: llm.NewFakeProvider("p1", ""), pingErr: nil}
	reg := llm.NewProviderRegistry(p1)
	reg.WithPingInterval(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Start(ctx)

	if !reg.IsAvailable("p1") {
		t.Error("expected p1 available after Start")
	}
}

func TestProviderRegistry_Register_Select(t *testing.T) {
	high := llm.NewFakeProvider("openai-high", "")
	low := llm.NewFakeProvider("openai-low", "")
	anthropic := llm.NewFakeProvider("anthropic-main", "")

	reg := llm.NewProviderRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg.Register("openai-high", high, llm.Tags{
		"vendor": "openai", "tier": "smart", "budget": "high",
	})
	reg.Register("openai-low", low, llm.Tags{
		"vendor": "openai", "tier": "fast", "budget": "low",
	})
	reg.Register("anthropic-main", anthropic, llm.Tags{
		"vendor": "anthropic", "tier": "smart", "budget": "high",
	})
	reg.Start(ctx)

	// Select openai + high budget → only openai-high
	got := reg.Select(llm.Tag("vendor", "openai"), llm.Tag("budget", "high"))
	if len(got) != 1 || got[0].ID() != "openai-high" {
		t.Errorf("expected [openai-high], got %v", got)
	}

	// Select vendor=openai → both openai entries
	got = reg.Select(llm.Tag("vendor", "openai"))
	if len(got) != 2 {
		t.Errorf("expected 2 openai providers, got %d", len(got))
	}

	// Select vendor=anthropic + budget=high → anthropic-main
	got = reg.Select(llm.Tag("vendor", "anthropic"), llm.Tag("budget", "high"))
	if len(got) != 1 || got[0].ID() != "anthropic-main" {
		t.Errorf("expected [anthropic-main], got %v", got)
	}

	// Select non-existent tag → nil
	got = reg.Select(llm.Tag("vendor", "cohere"))
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestProviderRegistry_Get(t *testing.T) {
	p := llm.NewFakeProvider("openai-high", "")
	reg := llm.NewProviderRegistry()
	reg.Register("openai-high", p, llm.Tags{"vendor": "openai"})

	got, ok := reg.Get("openai-high")
	if !ok || got.ID() != "openai-high" {
		t.Errorf("expected openai-high, got ok=%v id=%v", ok, got)
	}
	_, ok = reg.Get("missing")
	if ok {
		t.Error("expected false for missing provider")
	}
}
