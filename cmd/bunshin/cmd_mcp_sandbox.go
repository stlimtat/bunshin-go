package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/internal/credentials"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/mcp"
	"github.com/stlimtat/bunshin-go/pkg/sandbox"
	"github.com/stlimtat/bunshin-go/pkg/tools"
)

func newMCPSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-sandbox",
		Short: "MCP tool discovery + sandboxed code execution demo",
		Long: `Demonstrates MCP tool discovery and sandboxed code execution:

  1. Creates a fake MCP client pre-loaded with a "run_python" tool.
  2. Routes tool calls through a MockBackend sandbox.
  3. Injects an API credential per-request via context (credentials package).

In production, replace FakeMCPClient with a real MCPClient pointing at an
MCP server, and replace MockBackend with E2BBackend or DockerBackend.

Environment variables:
  BUNSHIN_API_KEY      API key injected as the MCP server credential (default: demo-key-123)
  BUNSHIN_SERVER_URL   MCP server URL`,
		Example: `  bunshin mcp-sandbox
  bunshin mcp-sandbox --server-url http://mcp.example.com
  BUNSHIN_API_KEY=my-real-key bunshin mcp-sandbox`,
		RunE: runMCPSandbox,
	}
	cmd.Flags().String("server-url", "http://localhost:8811", "MCP server URL")
	mustBindFlag(cmd, "server_url", "server-url")
	return cmd
}

func runMCPSandbox(_ *cobra.Command, _ []string) error {
	cfg := loadConfig()
	serverURL := viper.GetString("server_url")

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = "demo-key-123"
	}

	ctx := credentials.WithCredential(context.Background(), "mcp-server", credentials.APIKeyCredential(apiKey))

	sb := sandbox.NewMockBackend("Hello from sandbox!")
	sb.Responses["print('hi')"] = &sandbox.ExecResult{
		Stdout:    "hi\n",
		ExitCode:  0,
		SessionID: "session-1",
	}

	runPython := tools.NewFuncTool(
		tools.ToolSchema{
			Name:        "run_python",
			Description: "Execute Python code in a secure sandbox",
			Parameters:  map[string]any{"type": "string"},
		},
		func(ctx context.Context, input any) (any, error) {
			code, ok := input.(string)
			if !ok {
				return nil, fmt.Errorf("run_python: expected string code, got %T", input)
			}
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

	fakeMCP := &mcp.FakeMCPClient{FakeTools: []tools.Tool{runPython}}
	if err := fakeMCP.Connect(ctx, serverURL); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	mcpTools, err := fakeMCP.Tools(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	fmt.Printf("MCP tools available: %d\n", len(mcpTools))
	for _, t := range mcpTools {
		fmt.Printf("  - %s: %s\n", t.Schema().Name, t.Schema().Description)
	}

	runner := core.NewRunnableFunc("mcp-exec", func(ctx context.Context, input any) (any, error) {
		if cred, ok := credentials.FromContext(ctx, "mcp-server"); ok {
			preview := cred.APIKey
			if len(preview) > 8 {
				preview = preview[:8]
			}
			fmt.Printf("Using credential: %s...\n", preview)
		}
		return fakeMCP.CallTool(ctx, "run_python", input)
	})

	for _, code := range []string{"print('hi')", "print('hello world')"} {
		out, err := runner.Invoke(ctx, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "exec %q: %v\n", code, err)
			continue
		}
		fmt.Printf("Output of %q: %v\n", code, out)
	}
	return nil
}
