// Package sandbox provides pluggable sandboxed code execution.
//
// The Sandbox interface abstracts over E2B cloud sandboxes, local Docker
// containers, and WebAssembly runtimes. Backends are interchangeable.
//
// Use SandboxRegistry to register named backends with tags that encode both
// selection criteria and resource limits. This mirrors ProviderRegistry in
// pkg/llm and is a key differentiator of bunshin-go for multi-tenant deployments.
//
// # Session lifecycle
//
// Run is a convenience method for single-shot execution. For multi-step agent
// loops where state must persist between tool calls (e.g. install package →
// run analysis), use Session:
//
//	session, _ := sandbox.Session(ctx)
//	defer session.Close()
//	session.Run(ctx, sandbox.RunRequest{Language: "python", Code: "import pandas"})
//	result, _ := session.Run(ctx, sandbox.RunRequest{Language: "python", Code: "print(pd.__version__)"})
//
// Use MockSandbox in tests for deterministic, zero-latency responses.
package sandbox

import (
	"context"
	"time"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// Sandbox executes code in an isolated environment.
type Sandbox interface {
	// Session opens a persistent execution environment.
	// Multiple Run calls share filesystem and installed packages.
	// Callers must call Session.Close when done.
	Session(ctx context.Context) (Session, error)

	// Run is a convenience method: opens a session, executes once, closes.
	// Use Session for multi-step execution.
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// Session is a persistent sandbox execution context.
// Multiple Run calls share state. Callers must call Close when done.
type Session interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
	Close() error
}

// RunRequest describes a code execution job.
type RunRequest struct {
	// Language is the execution runtime (python, javascript, bash, go).
	Language string
	// Code is the source to execute.
	Code string
	// Timeout limits the execution time. Zero means no limit.
	Timeout time.Duration
	// Stdin is optional standard input.
	Stdin string
	// Files are input files made available in the sandbox working directory.
	Files map[string][]byte
	// EnvVars are environment variables set for the execution.
	EnvVars map[string]string
}

// RunResult holds the output of a code execution.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	// Files are output files written by the execution.
	// Small files (below inline_max_bytes registry tag, default 10MB) are returned
	// as inline streaming readers via MediaRef.Data.
	// Large files are uploaded to the configured blob store and returned as
	// remote refs via MediaRef.URL. The agent passes these directly into
	// ContentPart for the next LLM call.
	Files    map[string]*llm.MediaRef
	Duration time.Duration
}
