package tools_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/sandbox"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func makeRegistry(stdout string) *sandbox.SandboxRegistry {
	reg := sandbox.NewSandboxRegistry()
	reg.Register("mock", sandbox.NewMockSandbox(stdout), sandbox.Tags{"env": "test"})
	return reg
}

func TestCodeExecTool_Schema(t *testing.T) {
	tool := tools.NewCodeExecTool(makeRegistry(""))
	s := tool.Schema()
	if s.Name != "code_exec" {
		t.Fatalf("want code_exec, got %q", s.Name)
	}
	if s.Parameters == nil {
		t.Fatal("schema must have parameters")
	}
}

func TestCodeExecTool_Invoke_Success(t *testing.T) {
	reg := makeRegistry("hello world\n")
	tool := tools.NewCodeExecTool(reg)

	out, err := tool.Invoke(context.Background(), map[string]any{
		"language": "python",
		"code":     "print('hello world')",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, ok := out.(tools.CodeExecOutput)
	if !ok {
		t.Fatalf("want CodeExecOutput, got %T", out)
	}
	if result.Stdout != "hello world\n" {
		t.Fatalf("want 'hello world\\n', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("want exit 0, got %d", result.ExitCode)
	}
}

func TestCodeExecTool_Invoke_NoSandbox(t *testing.T) {
	reg := sandbox.NewSandboxRegistry()
	tool := tools.NewCodeExecTool(reg, sandbox.Tags{"env": "nonexistent"})

	_, err := tool.Invoke(context.Background(), map[string]any{
		"language": "python",
		"code":     "pass",
	})
	if err == nil {
		t.Fatal("expected error when no sandbox matches")
	}
}

func TestCodeExecTool_ImplementsTool(t *testing.T) {
	var _ tools.Tool = tools.NewCodeExecTool(makeRegistry(""))
}

func TestCodeExecTool_Name(t *testing.T) {
	tool := tools.NewCodeExecTool(makeRegistry(""))
	if tool.Name() != "code_exec" {
		t.Errorf("expected code_exec, got %q", tool.Name())
	}
}

func TestCodeExecTool_Stream(t *testing.T) {
	reg := makeRegistry("streamed\n")
	tool := tools.NewCodeExecTool(reg)

	ch, err := tool.Stream(context.Background(), map[string]any{
		"language": "python",
		"code":     "print('streamed')",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	chunk, ok := <-ch
	if !ok {
		t.Fatal("expected at least one chunk")
	}
	if chunk.Err != nil {
		t.Fatalf("unexpected chunk error: %v", chunk.Err)
	}
	result, ok := chunk.Value.(tools.CodeExecOutput)
	if !ok {
		t.Fatalf("want CodeExecOutput in chunk, got %T", chunk.Value)
	}
	if result.Stdout != "streamed\n" {
		t.Errorf("expected 'streamed\\n', got %q", result.Stdout)
	}
}
