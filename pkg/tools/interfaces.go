// Package tools defines the Tool interface and ToolRegistry.
//
// A Tool is a Runnable with a declared schema. The schema lets LLMs discover
// and call tools via function-calling APIs, and lets the MCP server advertise
// tools to external clients.
package tools

import "github.com/stlimtat/bunshin-go/pkg/core"

// Tool is a Runnable with a declared schema.
// Every tool that agents can call must implement this interface.
type Tool interface {
	core.Runnable
	// Schema returns the tool's identity and input contract.
	Schema() ToolSchema
}
