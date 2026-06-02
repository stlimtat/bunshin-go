package mcp

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// ToolAdapter wraps an MCP tool definition as a bunshin-go Tool.
// Calls are forwarded to the MCPClient.CallTool method.
type ToolAdapter struct {
	schema tools.ToolSchema
	client MCPClient
}

func (a *ToolAdapter) Name() string             { return a.schema.Name }
func (a *ToolAdapter) Schema() tools.ToolSchema { return a.schema }

func (a *ToolAdapter) Invoke(ctx context.Context, input any) (any, error) {
	return a.client.CallTool(ctx, a.schema.Name, input)
}

func (a *ToolAdapter) Stream(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := a.Invoke(ctx, input)
		ch <- core.StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}
