package mcp

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// FakeMCPClient is a test double for MCPClient.
type FakeMCPClient struct {
	ConnectedURL string
	FakeTools    []tools.Tool
	FakeErr      error
	CallLog      []string
}

func (f *FakeMCPClient) Connect(_ context.Context, serverURL string) error {
	f.ConnectedURL = serverURL
	return f.FakeErr
}

func (f *FakeMCPClient) Tools(_ context.Context) ([]tools.Tool, error) {
	return f.FakeTools, f.FakeErr
}

func (f *FakeMCPClient) CallTool(_ context.Context, name string, input any) (any, error) {
	f.CallLog = append(f.CallLog, name)
	return input, f.FakeErr
}

func (f *FakeMCPClient) Resources(_ context.Context) ([]Resource, error) {
	return nil, f.FakeErr
}

func (f *FakeMCPClient) ReadResource(_ context.Context, _ string) (*ResourceContent, error) {
	return nil, f.FakeErr
}

func (f *FakeMCPClient) Prompts(_ context.Context) ([]MCPPrompt, error) {
	return nil, f.FakeErr
}

func (f *FakeMCPClient) GetPrompt(_ context.Context, _ string, _ map[string]string) ([]PromptMessage, error) {
	return nil, f.FakeErr
}

func (f *FakeMCPClient) Close() error { return nil }
