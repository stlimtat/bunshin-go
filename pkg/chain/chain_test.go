package chain_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/chain"
	"github.com/stlimtat/bunshin-go/pkg/core"
)

func inc() core.Runnable {
	return core.NewRunnableFunc("inc", func(_ context.Context, input any) (any, error) {
		return input.(int) + 1, nil
	})
}

func double() core.Runnable {
	return core.NewRunnableFunc("double", func(_ context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	})
}

var errSentinel = errors.New("sentinel")

func fail(msg string) core.Runnable {
	return core.NewRunnableFunc("fail", func(_ context.Context, _ any) (any, error) {
		return nil, fmt.Errorf("%s: %w", msg, errSentinel)
	})
}

func TestChain_Invoke_Sequential(t *testing.T) {
	// inc then double: (3+1)*2 = 8
	c := chain.New("test", chain.S("inc", inc()), chain.S("double", double()))
	out, err := c.Invoke(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 8 {
		t.Fatalf("want 8, got %v", out)
	}
}

func TestChain_Invoke_SingleStep(t *testing.T) {
	c := chain.New("single", chain.S("inc", inc()))
	out, err := c.Invoke(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 6 {
		t.Fatalf("want 6, got %v", out)
	}
}

func TestChain_Invoke_Empty(t *testing.T) {
	c := chain.New("empty")
	out, err := c.Invoke(context.Background(), "passthrough")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "passthrough" {
		t.Fatalf("want passthrough, got %v", out)
	}
}

func TestChain_Invoke_StepError(t *testing.T) {
	c := chain.New("err-chain",
		chain.S("ok", inc()),
		chain.S("fail", fail("boom")),
		chain.S("unreachable", double()),
	)
	_, err := c.Invoke(context.Background(), 1)
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
	c := chain.New("mychain", chain.S("mystep", fail("oops")))
	_, err := c.Invoke(context.Background(), nil)
	if err == nil || err.Error() == "" {
		t.Fatal("expected wrapped error")
	}
}

func TestChain_Stream_LastStepOnly(t *testing.T) {
	c := chain.New("stream-chain",
		chain.S("inc", inc()),
		chain.S("double", double()),
	)
	ch, err := c.Stream(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var chunks []core.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	// Final result: (3+1)*2 = 8
	if chunks[len(chunks)-1].Value != 8 {
		t.Fatalf("want 8 in last chunk, got %v", chunks[len(chunks)-1].Value)
	}
}

func TestChain_Stream_Empty(t *testing.T) {
	c := chain.New("empty")
	ch, err := c.Stream(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var count int
	for range ch {
		count++
	}
	if count != 0 {
		t.Fatalf("want 0 chunks from empty chain, got %d", count)
	}
}

func TestChain_ImplementsRunnable(t *testing.T) {
	// Chain must implement core.Runnable so it nests inside other chains.
	var _ core.Runnable = chain.New("inner")
}

func TestChain_Name(t *testing.T) {
	c := chain.New("my-chain")
	if c.Name() != "my-chain" {
		t.Fatalf("want my-chain, got %q", c.Name())
	}
}
