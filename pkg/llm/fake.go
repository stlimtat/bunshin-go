package llm

import (
	"context"
	"fmt"
)

// FakeProvider is a deterministic LLMProvider for use in tests.
// Responses are keyed by the text of the last user message.
// If no matching key exists, it returns the DefaultResponse.
type FakeProvider struct {
	ProviderID    ProviderID
	Responses     map[string]string // last user message text → response content
	DefaultResp   string
	Err           error // if non-nil, every call returns this error
	CallCount     int
	LastRequest   *Request
}

// NewFakeProvider constructs a FakeProvider with a fixed response.
func NewFakeProvider(id ProviderID, defaultResp string) *FakeProvider {
	return &FakeProvider{ProviderID: id, DefaultResp: defaultResp, Responses: make(map[string]string)}
}

func (f *FakeProvider) ID() ProviderID { return f.ProviderID }

func (f *FakeProvider) Complete(_ context.Context, req *Request) (*Response, error) {
	f.CallCount++
	f.LastRequest = req
	if f.Err != nil {
		return nil, f.Err
	}
	content := f.DefaultResp
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1].Text()
		if r, ok := f.Responses[last]; ok {
			content = r
		}
	}
	return &Response{Content: content, Model: ModelID(fmt.Sprintf("%s-fake", f.ProviderID))}, nil
}

func (f *FakeProvider) StreamComplete(_ context.Context, req *Request) (<-chan Chunk, error) {
	f.CallCount++
	f.LastRequest = req
	if f.Err != nil {
		return nil, f.Err
	}
	ch := make(chan Chunk, 2)
	content := f.DefaultResp
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1].Text()
		if r, ok := f.Responses[last]; ok {
			content = r
		}
	}
	go func() {
		defer close(ch)
		ch <- Chunk{Delta: content}
		ch <- Chunk{Done: true, Usage: &TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}
	}()
	return ch, nil
}

func (f *FakeProvider) CanTransferContext(from LLMProvider) bool {
	return from.ID() == f.ProviderID
}

func (f *FakeProvider) NativeMessages(msgs []Message) (any, error) {
	// Return as-is for test purposes — no wire format transformation.
	return msgs, nil
}
