package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/sandbox"
)

func TestMockSandbox_Run_Default(t *testing.T) {
	s := sandbox.NewMockSandbox("hello")
	res, err := s.Run(context.Background(), sandbox.RunRequest{Language: "python", Code: "print('hi')"})
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

func TestMockSandbox_Run_KeyedResponse(t *testing.T) {
	s := sandbox.NewMockSandbox("default")
	s.Responses["print(1+1)"] = sandbox.RunResult{Stdout: "2", ExitCode: 0}
	res, _ := s.Run(context.Background(), sandbox.RunRequest{Code: "print(1+1)"})
	if res.Stdout != "2" {
		t.Fatalf("want 2, got %q", res.Stdout)
	}
}

func TestMockSandbox_Run_Error(t *testing.T) {
	s := sandbox.NewMockSandbox("")
	s.Err = errors.New("sandbox unavailable")
	_, err := s.Run(context.Background(), sandbox.RunRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockSandbox_Session_MultipleRuns(t *testing.T) {
	s := sandbox.NewMockSandbox("output")
	sess, err := s.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	r1, _ := sess.Run(context.Background(), sandbox.RunRequest{Code: "step1"})
	r2, _ := sess.Run(context.Background(), sandbox.RunRequest{Code: "step2"})
	if r1.Stdout != "output" || r2.Stdout != "output" {
		t.Error("expected default stdout for both runs")
	}
	if s.CallCount != 2 {
		t.Errorf("expected CallCount=2, got %d", s.CallCount)
	}
}

func TestMockSandbox_Session_ClosedRejectsRun(t *testing.T) {
	s := sandbox.NewMockSandbox("x")
	sess, _ := s.Session(context.Background())
	_ = sess.Close()
	_, err := sess.Run(context.Background(), sandbox.RunRequest{})
	if !errors.Is(err, sandbox.ErrSessionClosed) {
		t.Errorf("expected ErrSessionClosed, got %v", err)
	}
}

func TestSandboxRegistry_RegisterSelect(t *testing.T) {
	reg := sandbox.NewSandboxRegistry()
	s1 := sandbox.NewMockSandbox("e2b")
	s2 := sandbox.NewMockSandbox("docker")

	reg.Register("e2b-python", s1, sandbox.Tags{"env": "e2b", "language": "python"})
	reg.Register("docker-python", s2, sandbox.Tags{"env": "docker", "language": "python"})

	got := reg.Select(sandbox.Tags{"language": "python"})
	if len(got) != 2 {
		t.Errorf("expected 2 python sandboxes, got %d", len(got))
	}

	got = reg.Select(sandbox.Tags{"env": "e2b"})
	if len(got) != 1 {
		t.Errorf("expected 1 e2b sandbox, got %d", len(got))
	}
}

func TestSandboxRegistry_Get(t *testing.T) {
	reg := sandbox.NewSandboxRegistry()
	s := sandbox.NewMockSandbox("x")
	reg.Register("my-sandbox", s, sandbox.Tags{})

	got, ok := reg.Get("my-sandbox")
	if !ok || got == nil {
		t.Error("expected to find registered sandbox")
	}
	_, ok = reg.Get("missing")
	if ok {
		t.Error("expected false for missing sandbox")
	}
}
