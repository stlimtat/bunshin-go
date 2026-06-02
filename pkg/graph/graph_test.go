package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

func runnable(name string, fn func(any) any) core.Runnable {
	return core.NewRunnableFunc(name, func(_ context.Context, input any) (any, error) {
		return fn(input), nil
	})
}

func failRunnable(name, msg string) core.Runnable {
	return core.NewRunnableFunc(name, func(_ context.Context, _ any) (any, error) {
		return nil, errors.New(msg)
	})
}

func TestGraph_SingleNode_NoRouter(t *testing.T) {
	g := graph.New("g1").
		AddNode(graph.Node{
			ID:       "only",
			Runnable: runnable("only", func(in any) any { return in.(int) * 10 }),
		}).
		SetEntry("only")

	out, err := g.Invoke(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 50 {
		t.Fatalf("want 50, got %v", out)
	}
}

func TestGraph_TwoNodes_StaticRouter(t *testing.T) {
	g := graph.New("g2").
		AddNode(graph.Node{
			ID:       "A",
			Runnable: runnable("A", func(in any) any { return in.(int) + 1 }),
			Router:   graph.StaticRouter("B"),
		}).
		AddNode(graph.Node{
			ID:       "B",
			Runnable: runnable("B", func(in any) any { return in.(int) * 3 }),
		}).
		SetEntry("A")

	// (2+1)*3 = 9
	out, err := g.Invoke(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 9 {
		t.Fatalf("want 9, got %v", out)
	}
}

func TestGraph_ConditionalRouter(t *testing.T) {
	g := graph.New("conditional").
		AddNode(graph.Node{
			ID:       "check",
			Runnable: runnable("check", func(in any) any { return in }),
			Router: graph.ConditionalRouter(
				func(out any) string {
					if out.(int) > 5 {
						return "big"
					}
					return "small"
				},
				map[string]string{"big": "bigNode", "small": "smallNode"},
			),
		}).
		AddNode(graph.Node{
			ID:       "bigNode",
			Runnable: runnable("big", func(_ any) any { return "big" }),
		}).
		AddNode(graph.Node{
			ID:       "smallNode",
			Runnable: runnable("small", func(_ any) any { return "small" }),
		}).
		SetEntry("check")

	out, _ := g.Invoke(context.Background(), 10)
	if out != "big" {
		t.Fatalf("want big, got %v", out)
	}

	out, _ = g.Invoke(context.Background(), 2)
	if out != "small" {
		t.Fatalf("want small, got %v", out)
	}
}

func TestGraph_RouterToEND(t *testing.T) {
	g := graph.New("end-test").
		AddNode(graph.Node{
			ID:       "A",
			Runnable: runnable("A", func(in any) any { return "done" }),
			Router:   graph.StaticRouter(graph.END),
		}).
		SetEntry("A")

	out, err := g.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("want done, got %v", out)
	}
}

func TestGraph_NoEntryPoint(t *testing.T) {
	g := graph.New("no-entry")
	_, err := g.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
}

func TestGraph_NodeNotFound(t *testing.T) {
	g := graph.New("missing-node").
		AddNode(graph.Node{
			ID:       "A",
			Runnable: runnable("A", func(in any) any { return in }),
			Router:   graph.StaticRouter("nonexistent"),
		}).
		SetEntry("A")

	_, err := g.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestGraph_NodeError(t *testing.T) {
	g := graph.New("err-graph").
		AddNode(graph.Node{
			ID:       "fail",
			Runnable: failRunnable("fail", "node error"),
		}).
		SetEntry("fail")

	_, err := g.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGraph_DuplicateNodePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for duplicate node")
		}
	}()
	graph.New("dup").
		AddNode(graph.Node{ID: "A", Runnable: runnable("A", func(in any) any { return in })}).
		AddNode(graph.Node{ID: "A", Runnable: runnable("A", func(in any) any { return in })})
}

func TestGraph_ImplementsRunnable(t *testing.T) {
	var _ core.Runnable = graph.New("nested")
}
