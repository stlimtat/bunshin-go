// hello-mcp-sandbox demonstrates MCP tool discovery and sandboxed code execution.
//
// The example:
//  1. Creates a fake MCP client pre-loaded with a "run_python" tool.
//  2. Wraps the MCP tool as a bunshin-go Tool via mcp.ToolAdapter.
//  3. Routes tool calls through a MockBackend sandbox.
//  4. Injects a per-request API credential via context.
//
// Run:
//
//	go run ./examples/hello-mcp-sandbox
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stlimtat/bunshin-go/internal/credentials"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/mcp"
	"github.com/stlimtat/bunshin-go/pkg/sandbox"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func main() {
	ctx := context.Background()

	// Inject an API credential into the context — available to any component
	// downstream that calls credentials.FromContext(ctx, "mcp-server").
	ctx = credentials.WithCredential(ctx, "mcp-server", credentials.APIKeyCredential("demo-key-123"))

	// Build a sandbox backend (MockBackend for this demo).
	sb := sandbox.NewMockBackend("Hello from sandbox!")
	sb.Responses["print('hi')"] = &sandbox.ExecResult{
		Stdout:    "hi\n",
		ExitCode:  0,
		SessionID: "session-1",
	}

	// Build a "run_python" tool backed by the sandbox.
	runPython := tools.NewFuncTool(
		tools.ToolSchema{
			Name:        "run_python",
			Description: "Execute Python code in a secure sandbox",
			Parameters:  map[string]any{"type": "string"},
		},
		func(ctx context.Context, input any) (any, error) {
			code, _ := input.(string)
			result, err := sb.Exec(ctx, &sandbox.ExecRequest{
				Language: "python",
				Code:     code,
			})
			if err != nil {
				return nil, err
			}
			return result.Stdout, nil
		},
	)

	// Wrap it in a fake MCP client as if it arrived from a remote MCP server.
	fakeMCP := &mcp.FakeMCPClient{
		FakeTools: []tools.Tool{runPython},
	}
	if err := fakeMCP.Connect(ctx, "http://localhost:8811"); err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}

	// List tools from the MCP server.
	mcpTools, err := fakeMCP.Tools(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list tools: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("MCP tools available: %d\n", len(mcpTools))
	for _, t := range mcpTools {
		fmt.Printf("  - %s: %s\n", t.Schema().Name, t.Schema().Description)
	}

	// Build a Runnable that calls the first MCP tool.
	runner := core.NewRunnableFunc("mcp-exec", func(ctx context.Context, input any) (any, error) {
		// Retrieve injected credential to show it's accessible downstream.
		if cred, ok := credentials.FromContext(ctx, "mcp-server"); ok {
			fmt.Printf("Using credential: %s...\n", cred.APIKey[:8])
		}
		// Call the tool via the MCP fake client.
		return fakeMCP.CallTool(ctx, "run_python", input)
	})

	// Execute two code snippets.
	for _, code := range []string{"print('hi')", "print('hello world')"} {
		out, err := runner.Invoke(ctx, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "exec %q: %v\n", code, err)
			continue
		}
		fmt.Printf("Output of %q: %v\n", code, out)
	}
}
