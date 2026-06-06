package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

func nodeFunc[S any](name string, fn func(core.State[S]) core.State[S]) core.TypedRunnable[core.State[S], core.State[S]] {
	return core.TypedFunc(func(_ context.Context, s core.State[S]) (core.State[S], error) {
		return fn(s), nil
	})
}

func failNode[S any](name, msg string) core.TypedRunnable[core.State[S], core.State[S]] {
	return core.TypedFunc(func(_ context.Context, s core.State[S]) (core.State[S], error) {
		return s, errors.New(msg)
	})
}

func TestGraph_SingleNode_NoRouter(t *testing.T) {
	g := graph.New[int]("g1").
		AddNode(graph.Node[int]{
			ID:       "only",
			Runnable: nodeFunc[int]("only", func(s core.State[int]) core.State[int] { return core.NewState(s.Data * 10) }),
		}).
		SetEntry("only")

	out, err := g.Invoke(context.Background(), core.NewState(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data != 50 {
		t.Fatalf("want 50, got %v", out.Data)
	}
}

func TestGraph_TwoNodes_StaticRouter(t *testing.T) {
	g := graph.New[int]("g2").
		AddNode(graph.Node[int]{
			ID:       "A",
			Runnable: nodeFunc[int]("A", func(s core.State[int]) core.State[int] { return core.NewState(s.Data + 1) }),
			Router:   graph.StaticRouter[int]("B"),
		}).
		AddNode(graph.Node[int]{
			ID:       "B",
			Runnable: nodeFunc[int]("B", func(s core.State[int]) core.State[int] { return core.NewState(s.Data * 3) }),
		}).
		SetEntry("A")

	// (2+1)*3 = 9
	out, err := g.Invoke(context.Background(), core.NewState(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data != 9 {
		t.Fatalf("want 9, got %v", out.Data)
	}
}

func TestGraph_ConditionalRouter(t *testing.T) {
	g := graph.New[int]("conditional").
		AddNode(graph.Node[int]{
			ID:       "check",
			Runnable: nodeFunc[int]("check", func(s core.State[int]) core.State[int] { return s }),
			Router: graph.ConditionalRouter[int](
				func(s core.State[int]) string {
					if s.Data > 5 {
						return "big"
					}
					return "small"
				},
				map[string]string{"big": "bigNode", "small": "smallNode"},
			),
		}).
		AddNode(graph.Node[int]{
			ID:       "bigNode",
			Runnable: nodeFunc[int]("big", func(_ core.State[int]) core.State[int] { return core.NewState(100) }),
		}).
		AddNode(graph.Node[int]{
			ID:       "smallNode",
			Runnable: nodeFunc[int]("small", func(_ core.State[int]) core.State[int] { return core.NewState(-1) }),
		}).
		SetEntry("check")

	out, _ := g.Invoke(context.Background(), core.NewState(10))
	if out.Data != 100 {
		t.Fatalf("want 100, got %v", out.Data)
	}

	out, _ = g.Invoke(context.Background(), core.NewState(2))
	if out.Data != -1 {
		t.Fatalf("want -1, got %v", out.Data)
	}
}

func TestGraph_RouterToEND(t *testing.T) {
	g := graph.New[string]("end-test").
		AddNode(graph.Node[string]{
			ID:       "A",
			Runnable: nodeFunc[string]("A", func(_ core.State[string]) core.State[string] { return core.NewState("done") }),
			Router:   graph.StaticRouter[string](graph.END),
		}).
		SetEntry("A")

	out, err := g.Invoke(context.Background(), core.NewState(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data != "done" {
		t.Fatalf("want done, got %v", out.Data)
	}
}

func TestGraph_NoEntryPoint(t *testing.T) {
	g := graph.New[int]("no-entry")
	_, err := g.Invoke(context.Background(), core.NewState(0))
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
}

func TestGraph_NodeNotFound(t *testing.T) {
	g := graph.New[int]("missing-node").
		AddNode(graph.Node[int]{
			ID:       "A",
			Runnable: nodeFunc[int]("A", func(s core.State[int]) core.State[int] { return s }),
			Router:   graph.StaticRouter[int]("nonexistent"),
		}).
		SetEntry("A")

	_, err := g.Invoke(context.Background(), core.NewState(0))
	if err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestGraph_NodeError(t *testing.T) {
	g := graph.New[int]("err-graph").
		AddNode(graph.Node[int]{
			ID:       "fail",
			Runnable: failNode[int]("fail", "node error"),
		}).
		SetEntry("fail")

	_, err := g.Invoke(context.Background(), core.NewState(0))
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
	graph.New[int]("dup").
		AddNode(graph.Node[int]{ID: "A", Runnable: nodeFunc[int]("A", func(s core.State[int]) core.State[int] { return s })}).
		AddNode(graph.Node[int]{ID: "A", Runnable: nodeFunc[int]("A", func(s core.State[int]) core.State[int] { return s })})
}

func TestGraph_ImplementsTypedRunnable(t *testing.T) {
	var _ core.TypedRunnable[core.State[int], core.State[int]] = graph.New[int]("nested")
}

func TestGraph_PassThroughNodePreservesMeta(t *testing.T) {
	// A node that returns s unchanged preserves all Meta.
	g := graph.New[int]("meta-graph").
		AddNode(graph.Node[int]{
			ID:       "passthrough",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[int]) (core.State[int], error) { return s, nil }),
		}).
		SetEntry("passthrough")

	input := core.NewState(42).WithMeta("trace", "xyz")
	out, err := g.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := out.GetMeta("trace")
	if !ok || v != "xyz" {
		t.Fatalf("meta not preserved: got %v, ok=%v", v, ok)
	}
}

func TestGraph_AsRunnable_Roundtrip(t *testing.T) {
	g := graph.New[int]("wrap").
		AddNode(graph.Node[int]{
			ID:       "double",
			Runnable: nodeFunc[int]("double", func(s core.State[int]) core.State[int] { return core.NewState(s.Data * 2) }),
		}).
		SetEntry("double")

	r := g.AsRunnable()
	out, err := r.Invoke(context.Background(), core.NewState(7))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := out.(core.State[int])
	if !ok {
		t.Fatalf("want State[int], got %T", out)
	}
	if s.Data != 14 {
		t.Fatalf("want 14, got %v", s.Data)
	}
}
