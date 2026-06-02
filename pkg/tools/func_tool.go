package tools

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
)

// FuncTool wraps a function as a Tool.
type FuncTool struct {
	schema ToolSchema
	fn     func(ctx context.Context, input any) (any, error)
}

// NewFuncTool constructs a Tool from a schema and an invoke function.
func NewFuncTool(schema ToolSchema, fn func(ctx context.Context, input any) (any, error)) *FuncTool {
	return &FuncTool{schema: schema, fn: fn}
}

func (t *FuncTool) Name() string        { return t.schema.Name }
func (t *FuncTool) Schema() ToolSchema  { return t.schema }

func (t *FuncTool) Invoke(ctx context.Context, input any) (any, error) {
	return t.fn(ctx, input)
}

func (t *FuncTool) Stream(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk, 1)
	go func() {
		defer close(ch)
		out, err := t.fn(ctx, input)
		ch <- core.StreamChunk{Value: out, Err: err}
	}()
	return ch, nil
}
