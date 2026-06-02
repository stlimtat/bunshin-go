package sandbox

import "context"

// MockBackend is a deterministic Sandbox for use in tests.
// Responses are keyed by code string. If no match exists, DefaultResult is returned.
type MockBackend struct {
	Responses     map[string]*ExecResult
	DefaultResult *ExecResult
	Err           error
	CallCount     int
	LastRequest   *ExecRequest
	sessions      map[string]bool
}

// NewMockBackend constructs a MockBackend with a fixed stdout response.
func NewMockBackend(defaultStdout string) *MockBackend {
	return &MockBackend{
		Responses:     make(map[string]*ExecResult),
		DefaultResult: &ExecResult{Stdout: defaultStdout, ExitCode: 0},
		sessions:      make(map[string]bool),
	}
}

func (m *MockBackend) Exec(_ context.Context, req *ExecRequest) (*ExecResult, error) {
	m.CallCount++
	m.LastRequest = req
	if m.Err != nil {
		return nil, m.Err
	}
	if r, ok := m.Responses[req.Code]; ok {
		return r, nil
	}
	sid := req.SessionID
	if sid == "" {
		sid = "mock-session"
	}
	m.sessions[sid] = true
	result := *m.DefaultResult
	result.SessionID = sid
	return &result, nil
}

func (m *MockBackend) Kill(_ context.Context, sessionID string) error {
	if !m.sessions[sessionID] {
		return ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	return nil
}

func (m *MockBackend) Close() error { return nil }
