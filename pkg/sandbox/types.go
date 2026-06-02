package sandbox

import (
	"errors"
	"time"
)

// ExecRequest describes a code execution job.
type ExecRequest struct {
	// Language is the execution runtime (python, javascript, bash, go).
	Language string
	// Code is the source to execute.
	Code string
	// Files are input files made available in the sandbox working directory.
	Files map[string][]byte
	// Timeout limits the execution time. Zero means no limit.
	Timeout time.Duration
	// SessionID identifies a warm session to reuse. Empty starts a fresh sandbox.
	// Reusing a session preserves interpreter state across multi-step code execution.
	SessionID string
}

// ExecResult holds the output of a code execution.
type ExecResult struct {
	Stdout string
	Stderr string
	// Files are output files written by the executed code.
	Files    map[string][]byte
	ExitCode int
	// SessionID is the session that executed the code.
	// Use this in subsequent ExecRequests to reuse the warm session.
	SessionID string
}

// ErrSessionNotFound is returned when Kill is called with an unknown session ID.
var ErrSessionNotFound = errors.New("session not found")

// ErrUnsupportedLanguage is returned when the backend cannot run the requested language.
var ErrUnsupportedLanguage = errors.New("unsupported language")
