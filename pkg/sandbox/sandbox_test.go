package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/sandbox"
)

func TestMockBackend_Exec_Default(t *testing.T) {
	s := sandbox.NewMockBackend("hello")
	res, err := s.Exec(context.Background(), &sandbox.ExecRequest{Language: "python", Code: "print('hi')"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "hello" {
		t.Fatalf("want hello, got %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Fatalf("want ExitCode=0, got %d", res.ExitCode)
	}
}

func TestMockBackend_Exec_KeyedResponse(t *testing.T) {
	s := sandbox.NewMockBackend("default")
	s.Responses["print(1+1)"] = &sandbox.ExecResult{Stdout: "2", ExitCode: 0}
	res, _ := s.Exec(context.Background(), &sandbox.ExecRequest{Code: "print(1+1)"})
	if res.Stdout != "2" {
		t.Fatalf("want 2, got %q", res.Stdout)
	}
}

func TestMockBackend_Exec_Error(t *testing.T) {
	s := sandbox.NewMockBackend("")
	s.Err = errors.New("sandbox unavailable")
	_, err := s.Exec(context.Background(), &sandbox.ExecRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockBackend_Kill_ExistingSession(t *testing.T) {
	s := sandbox.NewMockBackend("ok")
	res, _ := s.Exec(context.Background(), &sandbox.ExecRequest{SessionID: "sess-1"})
	if err := s.Kill(context.Background(), res.SessionID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockBackend_Kill_MissingSession(t *testing.T) {
	s := sandbox.NewMockBackend("")
	err := s.Kill(context.Background(), "nonexistent")
	if !errors.Is(err, sandbox.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestMockBackend_SessionReuse(t *testing.T) {
	s := sandbox.NewMockBackend("output")
	req := &sandbox.ExecRequest{Code: "x=1", SessionID: "my-session"}
	res1, _ := s.Exec(context.Background(), req)
	res2, _ := s.Exec(context.Background(), req)
	if res1.SessionID != res2.SessionID {
		t.Fatalf("session IDs should match: %q vs %q", res1.SessionID, res2.SessionID)
	}
}

func TestMockBackend_CallCount(t *testing.T) {
	s := sandbox.NewMockBackend("x")
	for i := 0; i < 3; i++ {
		_, _ = s.Exec(context.Background(), &sandbox.ExecRequest{})
	}
	if s.CallCount != 3 {
		t.Fatalf("want CallCount=3, got %d", s.CallCount)
	}
}
