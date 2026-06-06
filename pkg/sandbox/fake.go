package sandbox

import (
	"context"
	"sync"
)

// MockSandbox is a deterministic Sandbox for use in tests.
// Responses are keyed by code string. If no match, DefaultResult is returned.
type MockSandbox struct {
	mu            sync.Mutex
	Responses     map[string]RunResult
	DefaultResult RunResult
	Err           error
	CallCount     int
	LastRequest   RunRequest
}

// NewMockSandbox constructs a MockSandbox with a fixed stdout response.
func NewMockSandbox(defaultStdout string) *MockSandbox {
	return &MockSandbox{
		Responses:     make(map[string]RunResult),
		DefaultResult: RunResult{Stdout: defaultStdout, ExitCode: 0},
	}
}

func (m *MockSandbox) Session(_ context.Context) (Session, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &mockSession{parent: m}, nil
}

func (m *MockSandbox) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	s, err := m.Session(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer s.Close()
	return s.Run(ctx, req)
}

type mockSession struct {
	mu     sync.Mutex
	parent *MockSandbox
	closed bool
}

func (s *mockSession) Run(_ context.Context, req RunRequest) (RunResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RunResult{}, ErrSessionClosed
	}
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	s.parent.CallCount++
	s.parent.LastRequest = req
	if s.parent.Err != nil {
		return RunResult{}, s.parent.Err
	}
	if r, ok := s.parent.Responses[req.Code]; ok {
		return r, nil
	}
	return s.parent.DefaultResult, nil
}

func (s *mockSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
