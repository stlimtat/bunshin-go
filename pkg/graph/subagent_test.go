package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
)

type outerState struct {
	Value  int
	Result int
}

type innerState struct {
	Input int
}

func TestSubagentNode_Basic(t *testing.T) {
	inner := graph.New[innerState]("inner").
		AddNode(graph.Node[innerState]{
			ID: "double",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[innerState]) (core.State[innerState], error) {
				return core.NewState(innerState{Input: s.Data.Input * 2}), nil
			}),
		}).
		SetEntry("double")

	sub := &graph.SubagentNode[outerState, innerState]{
		ID:       "subagent",
		Subgraph: inner,
		InjectFn: func(s core.State[outerState]) (core.State[innerState], error) {
			return core.NewState(innerState{Input: s.Data.Value}), nil
		},
		ExtractFn: func(outer core.State[outerState], innerOut core.State[innerState]) (core.State[outerState], error) {
			return core.NewState(outerState{Value: outer.Data.Value, Result: innerOut.Data.Input}), nil
		},
	}

	outer := graph.New[outerState]("outer").
		AddNode(sub.AsNode()).
		SetEntry("subagent")

	out, err := outer.Invoke(context.Background(), core.NewState(outerState{Value: 5}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data.Value != 5 {
		t.Fatalf("want Value=5, got %v", out.Data.Value)
	}
	if out.Data.Result != 10 {
		t.Fatalf("want Result=10, got %v", out.Data.Result)
	}
}

func TestSubagentNode_InjectError(t *testing.T) {
	inner := graph.New[innerState]("inner").
		AddNode(graph.Node[innerState]{
			ID:       "noop",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[innerState]) (core.State[innerState], error) { return s, nil }),
		}).
		SetEntry("noop")

	sub := &graph.SubagentNode[outerState, innerState]{
		ID:       "subagent",
		Subgraph: inner,
		InjectFn: func(s core.State[outerState]) (core.State[innerState], error) {
			return core.State[innerState]{}, errors.New("inject failed")
		},
		ExtractFn: func(outer core.State[outerState], innerOut core.State[innerState]) (core.State[outerState], error) {
			return outer, nil
		},
	}

	g := graph.New[outerState]("outer").
		AddNode(sub.AsNode()).
		SetEntry("subagent")

	_, err := g.Invoke(context.Background(), core.NewState(outerState{}))
	if err == nil {
		t.Fatal("expected inject error")
	}
}

func TestSubagentNode_ExtractError(t *testing.T) {
	inner := graph.New[innerState]("inner").
		AddNode(graph.Node[innerState]{
			ID:       "noop",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[innerState]) (core.State[innerState], error) { return s, nil }),
		}).
		SetEntry("noop")

	sub := &graph.SubagentNode[outerState, innerState]{
		ID:       "subagent",
		Subgraph: inner,
		InjectFn: func(s core.State[outerState]) (core.State[innerState], error) {
			return core.NewState(innerState{}), nil
		},
		ExtractFn: func(outer core.State[outerState], innerOut core.State[innerState]) (core.State[outerState], error) {
			return outer, errors.New("extract failed")
		},
	}

	g := graph.New[outerState]("outer").
		AddNode(sub.AsNode()).
		SetEntry("subagent")

	_, err := g.Invoke(context.Background(), core.NewState(outerState{}))
	if err == nil {
		t.Fatal("expected extract error")
	}
}

func TestSubagentNode_MetaPreserved(t *testing.T) {
	inner := graph.New[innerState]("inner").
		AddNode(graph.Node[innerState]{
			ID:       "noop",
			Runnable: core.TypedFunc(func(_ context.Context, s core.State[innerState]) (core.State[innerState], error) { return s, nil }),
		}).
		SetEntry("noop")

	sub := &graph.SubagentNode[outerState, innerState]{
		ID:       "subagent",
		Subgraph: inner,
		InjectFn: func(s core.State[outerState]) (core.State[innerState], error) {
			return core.NewState(innerState{}), nil
		},
		ExtractFn: func(outer core.State[outerState], _ core.State[innerState]) (core.State[outerState], error) {
			return outer, nil
		},
	}

	g := graph.New[outerState]("outer").
		AddNode(sub.AsNode()).
		SetEntry("subagent")

	input := core.NewState(outerState{}).WithMeta("trace_id", "abc")
	out, err := g.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ExtractFn returns the outer input unchanged, so meta is preserved.
	v, ok := out.GetMeta("trace_id")
	if !ok || v != "abc" {
		t.Fatalf("want trace_id=abc, got %v ok=%v", v, ok)
	}
}
