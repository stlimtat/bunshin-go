package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func TestNewTextMessage(t *testing.T) {
	m := llm.NewTextMessage(llm.RoleUser, "hello")
	if m.Role != llm.RoleUser {
		t.Fatalf("want role=user, got %q", m.Role)
	}
	if m.Text() != "hello" {
		t.Fatalf("want text=hello, got %q", m.Text())
	}
}

func TestMessage_Text_MultiPart(t *testing.T) {
	m := llm.Message{
		Role: llm.RoleAssistant,
		Parts: []llm.ContentPart{
			{Type: llm.PartTypeText, Text: "foo"},
			{Type: llm.PartTypeImage, Media: &llm.MediaRef{URL: "http://example.com/img.png"}},
			{Type: llm.PartTypeText, Text: "bar"},
		},
	}
	if got := m.Text(); got != "foobar" {
		t.Fatalf("want foobar, got %q", got)
	}
}

func TestMessage_NativeCache(t *testing.T) {
	m := llm.NewTextMessage(llm.RoleUser, "x")
	m.CacheNative(llm.ProviderOpenAI, "native-repr")

	v, ok := m.Native(llm.ProviderOpenAI)
	if !ok {
		t.Fatal("expected cached native, got miss")
	}
	if v != "native-repr" {
		t.Fatalf("want native-repr, got %v", v)
	}

	_, ok = m.Native(llm.ProviderAnthropic)
	if ok {
		t.Fatal("expected cache miss for Anthropic")
	}
}

func TestFakeProvider_Complete(t *testing.T) {
	fp := llm.NewFakeProvider(llm.ProviderOpenAI, "default answer")
	fp.Responses["ping"] = "pong"

	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "ping")}}
	resp, err := fp.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "pong" {
		t.Fatalf("want pong, got %q", resp.Content)
	}
	if fp.CallCount != 1 {
		t.Fatalf("want CallCount=1, got %d", fp.CallCount)
	}
}

func TestFakeProvider_Complete_DefaultResponse(t *testing.T) {
	fp := llm.NewFakeProvider(llm.ProviderAnthropic, "fallback")
	req := &llm.Request{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "unknown")}}
	resp, _ := fp.Complete(context.Background(), req)
	if resp.Content != "fallback" {
		t.Fatalf("want fallback, got %q", resp.Content)
	}
}

func TestFakeProvider_Complete_Error(t *testing.T) {
	fp := llm.NewFakeProvider(llm.ProviderOpenAI, "")
	fp.Err = errors.New("rate limited")
	_, err := fp.Complete(context.Background(), &llm.Request{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFakeProvider_Stream(t *testing.T) {
	fp := llm.NewFakeProvider(llm.ProviderOpenAI, "streamed")
	ch, err := fp.StreamComplete(context.Background(), &llm.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []llm.Chunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks (content + done), got %d", len(chunks))
	}
	if chunks[0].Delta != "streamed" {
		t.Fatalf("want delta=streamed, got %q", chunks[0].Delta)
	}
	if !chunks[1].Done {
		t.Fatal("last chunk should have Done=true")
	}
}

func TestFakeProvider_CanTransferContext(t *testing.T) {
	a := llm.NewFakeProvider(llm.ProviderOpenAI, "")
	b := llm.NewFakeProvider(llm.ProviderOpenAI, "")
	c := llm.NewFakeProvider(llm.ProviderAnthropic, "")

	if !a.CanTransferContext(b) {
		t.Fatal("same provider should CanTransferContext=true")
	}
	if a.CanTransferContext(c) {
		t.Fatal("different provider should CanTransferContext=false")
	}
}

func TestModelTier_Constants(t *testing.T) {
	tiers := []llm.ModelTier{llm.TierFast, llm.TierSmart, llm.TierReasoning}
	seen := make(map[llm.ModelTier]bool)
	for _, tier := range tiers {
		if tier == "" {
			t.Fatal("empty ModelTier constant")
		}
		if seen[tier] {
			t.Fatalf("duplicate ModelTier: %q", tier)
		}
		seen[tier] = true
	}
}
