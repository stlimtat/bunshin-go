package mcp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
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

func TestFakeMCPClient_CallTool_UnknownTool_Error(t *testing.T) {
	c := &mcp.FakeMCPClient{}
	_, err := c.CallTool(context.Background(), "unknown", "input")
	if err == nil {
		t.Fatal("expected error for unregistered tool")
	}
}

func TestFakeMCPClient_CallTool_DelegatesRegisteredTool(t *testing.T) {
	fakeTool := tools.NewFuncTool(
		tools.ToolSchema{Name: "double", Description: "doubles input"},
		func(_ context.Context, input any) (any, error) { return input.(int) * 2, nil },
	)
	c := &mcp.FakeMCPClient{FakeTools: []tools.Tool{fakeTool}}
	out, err := c.CallTool(context.Background(), "double", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 10 {
		t.Fatalf("want 10, got %v", out)
	}
}

func TestToolAdapter_Invoke(t *testing.T) {
	fakeTool := tools.NewFuncTool(
		tools.ToolSchema{Name: "ping", Description: "returns input"},
		func(_ context.Context, input any) (any, error) { return input, nil },
	)
	client := &mcp.FakeMCPClient{FakeTools: []tools.Tool{fakeTool}}
	adapter := mcp.NewToolAdapter(tools.ToolSchema{Name: "ping"}, client)

	out, err := adapter.Invoke(context.Background(), "pong")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "pong" {
		t.Fatalf("want pong, got %v", out)
	}
}

func TestToolAdapter_Stream_SingleChunk(t *testing.T) {
	fakeTool := tools.NewFuncTool(
		tools.ToolSchema{Name: "inc", Description: "increment"},
		func(_ context.Context, input any) (any, error) { return input.(int) + 1, nil },
	)
	client := &mcp.FakeMCPClient{FakeTools: []tools.Tool{fakeTool}}
	adapter := mcp.NewToolAdapter(tools.ToolSchema{Name: "inc"}, client)

	ch, err := adapter.Stream(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []core.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if len(chunks) != 1 || chunks[0].Value != 4 {
		t.Fatalf("want [4], got %v", chunks)
	}
}

func TestToolAdapter_Schema(t *testing.T) {
	schema := tools.ToolSchema{Name: "tool-x", Description: "desc"}
	adapter := mcp.NewToolAdapter(schema, &mcp.FakeMCPClient{})
	if adapter.Schema().Name != "tool-x" {
		t.Fatalf("want tool-x, got %q", adapter.Schema().Name)
	}
	if adapter.Name() != "tool-x" {
		t.Fatalf("want tool-x, got %q", adapter.Name())
	}
}
