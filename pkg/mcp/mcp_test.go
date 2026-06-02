package mcp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/mcp"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func TestFakeMCPClient_Connect(t *testing.T) {
	c := &mcp.FakeMCPClient{}
	if err := c.Connect(context.Background(), "http://mcp.local"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ConnectedURL != "http://mcp.local" {
		t.Fatalf("want http://mcp.local, got %q", c.ConnectedURL)
	}
}

func TestFakeMCPClient_Connect_Error(t *testing.T) {
	c := &mcp.FakeMCPClient{FakeErr: errors.New("unreachable")}
	err := c.Connect(context.Background(), "http://bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFakeMCPClient_Tools(t *testing.T) {
	fakeTool := tools.NewFuncTool(
		tools.ToolSchema{Name: "calc", Description: "calculator"},
		func(_ context.Context, input any) (any, error) { return input, nil },
	)
	c := &mcp.FakeMCPClient{FakeTools: []tools.Tool{fakeTool}}
	ts, err := c.Tools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ts) != 1 || ts[0].Name() != "calc" {
		t.Fatalf("unexpected tools: %v", ts)
	}
}

func TestFakeMCPClient_CallTool_LogsName(t *testing.T) {
	c := &mcp.FakeMCPClient{}
	_, _ = c.CallTool(context.Background(), "search", "query")
	_, _ = c.CallTool(context.Background(), "calc", "1+1")
	if len(c.CallLog) != 2 {
		t.Fatalf("want 2 calls logged, got %d", len(c.CallLog))
	}
	if c.CallLog[0] != "search" || c.CallLog[1] != "calc" {
		t.Fatalf("unexpected call log: %v", c.CallLog)
	}
}

func TestFakeMCPClient_CallTool_ReturnsInput(t *testing.T) {
	c := &mcp.FakeMCPClient{}
	out, err := c.CallTool(context.Background(), "echo", "hello")
	if err != nil || out != "hello" {
		t.Fatalf("want hello nil, got %v %v", out, err)
	}
}
