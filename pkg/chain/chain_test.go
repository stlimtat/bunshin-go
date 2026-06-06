package chain_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/chain"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func inc() chain.Step[int] {
	return chain.Func[int]("inc", func(_ context.Context, s core.State[int]) (core.State[int], error) {
		return core.NewState(s.Data + 1), nil
	})
}

func double() chain.Step[int] {
	return chain.Func[int]("double", func(_ context.Context, s core.State[int]) (core.State[int], error) {
		return core.NewState(s.Data * 2), nil
	})
}

var errSentinel = errors.New("sentinel")

func fail(msg string) chain.Step[int] {
	return chain.Func[int]("fail", func(_ context.Context, _ core.State[int]) (core.State[int], error) {
		return core.State[int]{}, fmt.Errorf("%s: %w", msg, errSentinel)
	})
}

func TestChain_Invoke_Sequential(t *testing.T) {
	// inc then double: (3+1)*2 = 8
	c := chain.New[int]("test", inc(), double())
	out, err := c.Invoke(context.Background(), core.NewState(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data != 8 {
		t.Fatalf("want 8, got %v", out.Data)
	}
}

func TestChain_Invoke_SingleStep(t *testing.T) {
	c := chain.New[int]("single", inc())
	out, err := c.Invoke(context.Background(), core.NewState(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data != 6 {
		t.Fatalf("want 6, got %v", out.Data)
	}
}

func TestChain_Invoke_Empty(t *testing.T) {
	c := chain.New[string]("empty")
	out, err := c.Invoke(context.Background(), core.NewState("passthrough"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data != "passthrough" {
		t.Fatalf("want passthrough, got %v", out.Data)
	}
}

func TestChain_Invoke_StepError(t *testing.T) {
	c := chain.New[int]("err-chain", inc(), fail("boom"), double())
	_, err := c.Invoke(context.Background(), core.NewState(1))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errSentinel) {
		t.Fatalf("expected sentinel in error chain, got: %v", err)
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Fatalf("expected step id in error message, got: %v", err)
	}
}

func TestChain_Invoke_ErrorWrapsStepID(t *testing.T) {
	c := chain.New[int]("mychain", fail("oops"))
	_, err := c.Invoke(context.Background(), core.NewState(0))
	if err == nil || err.Error() == "" {
		t.Fatal("expected wrapped error")
	}
}

func TestChain_Invoke_MetaPreserved(t *testing.T) {
	// A step that explicitly propagates Meta preserves it through the chain.
	passWithMeta := chain.Func[int]("pass", func(_ context.Context, s core.State[int]) (core.State[int], error) {
		return s, nil // return s unchanged — Meta is preserved
	})
	c := chain.New[int]("meta-chain", passWithMeta)
	input := core.NewState(1).WithMeta("trace", "abc")
	out, err := c.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := out.GetMeta("trace")
	if !ok || v != "abc" {
		t.Fatalf("meta not preserved: got %v, ok=%v", v, ok)
	}
}

func TestChain_ImplementsTypedRunnable(t *testing.T) {
	var _ core.TypedRunnable[core.State[int], core.State[int]] = chain.New[int]("inner")
}

func TestChain_AsRunnable_Roundtrip(t *testing.T) {
	c := chain.New[int]("wrap", inc())
	r := c.AsRunnable()
	out, err := r.Invoke(context.Background(), core.NewState(9))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := out.(core.State[int])
	if !ok {
		t.Fatalf("want State[int], got %T", out)
	}
	if s.Data != 10 {
		t.Fatalf("want 10, got %v", s.Data)
	}
}

func TestChain_Name(t *testing.T) {
	c := chain.New[string]("my-chain")
	if c.Name() != "my-chain" {
		t.Fatalf("want my-chain, got %q", c.Name())
	}
}

func TestChain_Steps(t *testing.T) {
	s1 := inc()
	s2 := double()
	c := chain.New[int]("two-step", s1, s2)
	steps := c.Steps()
	if len(steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(steps))
	}
	if steps[0].ID != s1.ID || steps[1].ID != s2.ID {
		t.Fatalf("step IDs mismatch: %q %q", steps[0].ID, steps[1].ID)
	}
}

func TestChain_FuncWithModel(t *testing.T) {
	step := chain.FuncWithModel[int]("step", func(_ context.Context, s core.State[int]) (core.State[int], error) {
		return s, nil
	}, llm.ModelConfig{Model: "gpt-4o"})
	if step.ID != "step" {
		t.Fatalf("want step ID 'step', got %q", step.ID)
	}
	if step.Model == nil || step.Model.Model != "gpt-4o" {
		t.Fatalf("expected Model to be set to gpt-4o, got %v", step.Model)
	}
	if step.Runnable == nil {
		t.Fatal("expected Runnable to be set")
	}
}
