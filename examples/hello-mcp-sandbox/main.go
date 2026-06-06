// hello-mcp-sandbox demonstrates how to register a mock sandbox via SandboxRegistry
// and invoke it through a CodeExecTool, as an MCP-compatible tool that LLMs can call.
//
// In production replace MockSandbox with a real E2B or Docker sandbox.
//
// Usage:
//
//	go run ./examples/hello-mcp-sandbox
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stlimtat/bunshin-go/pkg/sandbox"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func main() {
	// Register a mock sandbox with a default stdout response.
	registry := sandbox.NewSandboxRegistry()
	registry.Register(
		"mock-python",
		sandbox.NewMockSandbox("42\n"),
		sandbox.Tags{"lang": "python", "env": "demo"},
	)

	// Create the CodeExecTool selecting any sandbox tagged lang=python.
	tool := tools.NewCodeExecTool(registry, sandbox.Tags{"lang": "python"})

	// Print the JSON Schema the LLM would receive.
	schema := tool.Schema()
	schemaJSON, _ := json.MarshalIndent(schema.Parameters, "", "  ")
	fmt.Println("Tool schema parameters:")
	fmt.Println(string(schemaJSON))
	fmt.Println()

	// Invoke the tool as an LLM would (JSON-encoded input).
	input := map[string]any{
		"language": "python",
		"code":     "print(6 * 7)",
	}
	out, err := tool.Invoke(context.Background(), input)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	result := out.(tools.CodeExecOutput)
	fmt.Printf("Exit code: %d\n", result.ExitCode)
	fmt.Printf("Stdout:    %q\n", result.Stdout)
}
