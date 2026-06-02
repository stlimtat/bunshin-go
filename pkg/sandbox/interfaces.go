// Package sandbox provides pluggable sandboxed code execution.
//
// The Sandbox interface abstracts over E2B cloud sandboxes, local Docker
// containers, and WebAssembly runtimes. Backends are interchangeable via config.
//
// The DockerBackend is OpenSandbox-compatible: any OCI image that follows the
// open sandbox container spec works as a drop-in execution environment.
//
// Use MockBackend in tests for deterministic, zero-latency responses.
package sandbox

import "context"

// Sandbox executes code in an isolated environment.
type Sandbox interface {
	// Exec runs the given code and returns its output.
	Exec(ctx context.Context, req *ExecRequest) (*ExecResult, error)
	// Kill terminates a running or warm session by ID.
	Kill(ctx context.Context, sessionID string) error
	// Close releases any resources held by the sandbox backend.
	Close() error
}
