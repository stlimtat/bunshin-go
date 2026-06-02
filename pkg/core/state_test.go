package core_test

import (
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

type chatData struct {
	UserID  string
	Message string
}

func TestNewState_MetaInitialised(t *testing.T) {
	s := core.NewState(chatData{UserID: "u1"})
	if s.Meta == nil {
		t.Fatal("Meta should be initialised, got nil")
	}
	if s.Data.UserID != "u1" {
		t.Fatalf("want UserID=u1, got %q", s.Data.UserID)
	}
}

func TestState_WithMeta_ImmutableCopy(t *testing.T) {
	s := core.NewState(chatData{})
	s2 := s.WithMeta("key", "val")

	// Original must not be mutated.
	if _, ok := s.Meta["key"]; ok {
		t.Fatal("original State.Meta was mutated by WithMeta")
	}
	if v, _ := s2.GetMeta("key"); v != "val" {
		t.Fatalf("want val, got %v", v)
	}
}

func TestState_WithMeta_PreservesExisting(t *testing.T) {
	s := core.NewState(chatData{})
	s = s.WithMeta("a", 1)
	s2 := s.WithMeta("b", 2)

	if v, _ := s2.GetMeta("a"); v != 1 {
		t.Fatalf("key a lost after second WithMeta, got %v", v)
	}
	if v, _ := s2.GetMeta("b"); v != 2 {
		t.Fatalf("key b not set, got %v", v)
	}
}

func TestState_GetMeta_Missing(t *testing.T) {
	s := core.NewState(chatData{})
	_, ok := s.GetMeta("nonexistent")
	if ok {
		t.Fatal("expected ok=false for missing key")
	}
}

func TestState_WellKnownKeys(t *testing.T) {
	// Smoke: well-known key constants are non-empty strings.
	keys := []string{
		core.MetaTraceID,
		core.MetaSessionID,
		core.MetaRunID,
		core.MetaThreadID,
		core.MetaCostBudget,
	}
	seen := make(map[string]bool)
	for _, k := range keys {
		if k == "" {
			t.Fatal("empty Meta key constant")
		}
		if seen[k] {
			t.Fatalf("duplicate Meta key constant: %q", k)
		}
		seen[k] = true
	}
}
