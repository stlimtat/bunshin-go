// Package mcp implements the Model Context Protocol (MCP) client and server.
//
// MCP is Anthropic's open standard for connecting LLMs to external tools,
// resources, and prompts. bunshin-go supports both sides:
//
//   - MCPClient: connect to any MCP server and surface its tools/resources/prompts
//     as first-class bunshin-go primitives (Tool, MessageStore entries, Fragment).
//
//   - MCPServer: expose bunshin-go's ToolRegistry and PromptBackend as an MCP
//     server so that external clients (Claude Desktop, Cursor, etc.) can call
//     bunshin-go tools directly.
//
// Transport is pluggable: stdio (default for local tools), HTTP/SSE, WebSocket.
package mcp

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/tools"
)

// MCPClient connects to an MCP server and surfaces its capabilities.
type MCPClient interface {
	// Connect establishes the connection to the MCP server at serverURL.
	Connect(ctx context.Context, serverURL string) error

	// Tools returns all tools exposed by the server as bunshin-go Tools.
	Tools(ctx context.Context) ([]tools.Tool, error)

	// CallTool invokes a named tool on the server.
	CallTool(ctx context.Context, name string, input any) (any, error)

	// Resources returns all resources exposed by the server.
	Resources(ctx context.Context) ([]Resource, error)

	// ReadResource fetches the content of a resource by URI.
	ReadResource(ctx context.Context, uri string) (*ResourceContent, error)

	// Prompts returns all prompt templates exposed by the server.
	Prompts(ctx context.Context) ([]MCPPrompt, error)

	// GetPrompt renders a named prompt with the given arguments.
	GetPrompt(ctx context.Context, name string, args map[string]string) ([]PromptMessage, error)

	// Close terminates the connection.
	Close() error
}

// MCPServer exposes bunshin-go capabilities as an MCP server.
type MCPServer interface {
	// RegisterTools makes the registry's tools available via the MCP tools API.
	RegisterTools(registry *tools.ToolRegistry)

	// Serve starts the MCP server and blocks until ctx is cancelled.
	Serve(ctx context.Context) error
}
