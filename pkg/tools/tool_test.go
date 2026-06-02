package tools_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func makeEchoTool(name string) tools.Tool {
	return tools.NewFuncTool(
		tools.ToolSchema{Name: name, Description: "echoes input"},
		func(_ context.Context, input any) (any, error) { return input, nil },
	)
}

func TestToolRegistry_RegisterAndGet(t *testing.T) {
	reg := tools.NewToolRegistry()
	tool := makeEchoTool("echo")
	if err := reg.Register(tool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := reg.Get("echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "echo" {
		t.Fatalf("want echo, got %q", got.Name())
	}
}

func TestToolRegistry_DuplicateRegister(t *testing.T) {
	reg := tools.NewToolRegistry()
	_ = reg.Register(makeEchoTool("dup"))
	err := reg.Register(makeEchoTool("dup"))
	if err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestToolRegistry_GetMissing(t *testing.T) {
	reg := tools.NewToolRegistry()
	_, err := reg.Get("missing")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestToolRegistry_List(t *testing.T) {
	reg := tools.NewToolRegistry()
	_ = reg.Register(makeEchoTool("a"))
	_ = reg.Register(makeEchoTool("b"))
	schemas := reg.List()
	if len(schemas) != 2 {
		t.Fatalf("want 2 schemas, got %d", len(schemas))
	}
}

func TestFuncTool_Invoke(t *testing.T) {
	tool := makeEchoTool("ping")
	out, err := tool.Invoke(context.Background(), "pong")
	if err != nil || out != "pong" {
		t.Fatalf("want pong nil, got %v %v", out, err)
	}
}

func TestFuncTool_Stream(t *testing.T) {
	tool := makeEchoTool("stream-echo")
	ch, err := tool.Stream(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks int
	for range ch {
		chunks++
	}
	if chunks != 1 {
		t.Fatalf("want 1 chunk, got %d", chunks)
	}
}
