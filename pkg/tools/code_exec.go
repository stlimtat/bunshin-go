package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/sandbox"
)

// CodeExecInput is the JSON-decoded input to CodeExecTool.
type CodeExecInput struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Timeout  int    `json:"timeout_seconds,omitempty"`
}

// CodeExecOutput is the structured result of CodeExecTool.
type CodeExecOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// CodeExecTool wraps a SandboxRegistry as a Tool. The LLM invokes it by passing
// {"language":"python","code":"print('hello')"} as the tool call arguments.
// Sandbox selection is delegated to the registry — callers can inject tags at
// construction time to target a specific backend (e.g. E2B vs Docker).
type CodeExecTool struct {
	registry *sandbox.SandboxRegistry
	filters  []sandbox.Tags
}

// NewCodeExecTool constructs a CodeExecTool that selects a sandbox matching filters.
// If filters is empty, any registered sandbox is eligible.
func NewCodeExecTool(registry *sandbox.SandboxRegistry, filters ...sandbox.Tags) *CodeExecTool {
	return &CodeExecTool{registry: registry, filters: filters}
}

// Name returns the tool name used in LLM function-calling APIs.
func (t *CodeExecTool) Name() string { return "code_exec" }

// Schema returns the JSON Schema that LLMs use to construct valid tool calls.
func (t *CodeExecTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "code_exec",
		Description: "Execute code in a sandboxed environment and return stdout, stderr, and exit code.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language (e.g. python, javascript, bash).",
				},
				"code": map[string]any{
					"type":        "string",
					"description": "Source code to execute.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Execution timeout in seconds (default 30).",
					"default":     30,
				},
			},
			"required": []string{"language", "code"},
		},
	}
}

// Invoke decodes the input JSON, selects a sandbox, runs the code, and returns the result JSON.
func (t *CodeExecTool) Invoke(ctx context.Context, input any) (any, error) {
	raw, err := jsonBytes(input)
	if err != nil {
		return nil, fmt.Errorf("code_exec: marshal input: %w", err)
	}
	var req CodeExecInput
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("code_exec: decode input: %w", err)
	}

	candidates := t.registry.Select(t.filters...)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("code_exec: no sandbox registered matching filters")
	}
	sb := candidates[0]

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	result, err := sb.Run(ctx, sandbox.RunRequest{
		Language: req.Language,
		Code:     req.Code,
		Timeout:  timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("code_exec: run: %w", err)
	}

	return CodeExecOutput{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, nil
}

// Stream wraps Invoke in a single-chunk channel (code exec is not streaming).
func (t *CodeExecTool) Stream(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk, 1)
	go func() {
		out, err := t.Invoke(ctx, input)
		ch <- core.StreamChunk{Value: out, Err: err}
		close(ch)
	}()
	return ch, nil
}

func jsonBytes(v any) ([]byte, error) {
	if b, ok := v.([]byte); ok {
		return b, nil
	}
	if s, ok := v.(string); ok {
		return []byte(s), nil
	}
	return json.Marshal(v)
}
