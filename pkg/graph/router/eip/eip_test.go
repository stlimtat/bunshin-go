package eip_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/graph/router/eip"
	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// reg returns a RouterRegistry pre-loaded with the EIP catalog.
func reg(t *testing.T) *workflow.RouterRegistry {
	t.Helper()
	r := workflow.NewRouterRegistry()
	eip.Register(r)
	return r
}

func build(t *testing.T, reg *workflow.RouterRegistry, ref *workflow.RouterRef) graph.Router[map[string]any] {
	t.Helper()
	r, err := reg.Build(ref, workflow.NewRunnableRegistry())
	if err != nil {
		t.Fatalf("Build %q: %v", ref.Type, err)
	}
	return r
}

func state(data map[string]any) core.State[map[string]any] {
	return core.NewState(data)
}

func stateWithMeta(data map[string]any, meta map[string]any) core.State[map[string]any] {
	s := core.NewState(data)
	for k, v := range meta {
		s.Meta[k] = v
	}
	return s
}

// ---- helpers ----

func TestToInt_Float64(t *testing.T) {
	// pos stored as float64 (YAML/JSON unmarshal) must advance correctly.
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	// pos as float64 1.0 → should read as position 1 → route to "b"
	s := stateWithMeta(nil, map[string]any{"slip": []string{"a", "b"}, "pos": float64(1)})
	next, _ := r(context.Background(), s)
	if next != "b" {
		t.Errorf("float64 pos should coerce to int 1, want 'b', got %q", next)
	}
}

func TestNilMeta_NoPanic(t *testing.T) {
	// Zero-value state with nil Meta must not panic in routing_slip.
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	// Meta is nil — slip_key absent → END with no panic.
	s := core.State[map[string]any]{Data: map[string]any{}}
	next, err := r(context.Background(), s)
	if err != nil || next != graph.END {
		t.Errorf("nil Meta must not panic; want END, got %q err %v", next, err)
	}
}

func TestNegativePos_RoutesToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	s := stateWithMeta(nil, map[string]any{"slip": []string{"a"}, "pos": -1})
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("negative pos must route to END, got %q", next)
	}
}

func TestSplitter_StringSlice(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "splitter",
		Config: map[string]any{
			"input_key": "chunks", "item_key": "chunk", "pos_key": "spos", "next": "process",
		},
	})
	// []string (not []any) must be accepted.
	s := stateWithMeta(map[string]any{"chunks": []string{"hello", "world"}}, map[string]any{"spos": 0})
	next, err := r(context.Background(), s)
	if err != nil || next != "process" {
		t.Errorf("[]string slice should be accepted: got %q err %v", next, err)
	}
	if s.Data["chunk"] != "hello" {
		t.Errorf("want chunk=hello, got %v", s.Data["chunk"])
	}
}

func TestContentBased_AbsentMeta_RoutesToDefault(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "content_based",
		Config: map[string]any{
			"key":   "action",
			"cases": map[string]any{"done": "end"},
		},
	})
	// Key absent → should route to defaultNext (END), not match "<nil>"
	s := core.State[map[string]any]{Data: map[string]any{}, Meta: map[string]any{}}
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("absent meta key must route to END, got %q", next)
	}
}

// ---- Register ----

func TestRegister_AllTypesPresent(t *testing.T) {
	r := workflow.NewRouterRegistry()
	eip.Register(r)
	for _, typ := range []string{"content_based", "filter", "routing_slip", "recipient_list", "splitter", "aggregator"} {
		_, err := r.Build(&workflow.RouterRef{Type: typ, Config: map[string]any{
			// minimal valid config to avoid "missing key" errors — just check type is registered
		}}, workflow.NewRunnableRegistry())
		// May error on missing config keys, but must NOT error on "unknown router type"
		if err != nil && containsStr(err.Error(), "unknown router type") {
			t.Errorf("type %q not registered: %v", typ, err)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStrHelper(s, sub))
}
func containsStrHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---- Content-Based Router ----

func TestContentBased_MatchesCase(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "content_based",
		Config: map[string]any{
			"key":   "bunshin.next_action",
			"cases": map[string]any{"done": "END", "continue": "act"},
		},
	})
	s := stateWithMeta(nil, map[string]any{"bunshin.next_action": "continue"})
	next, err := r(context.Background(), s)
	if err != nil || next != "act" {
		t.Errorf("want 'act', got %q err %v", next, err)
	}
}

func TestContentBased_DefaultsToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "content_based",
		Config: map[string]any{
			"key":   "bunshin.next_action",
			"cases": map[string]any{"done": graph.END},
		},
	})
	s := stateWithMeta(nil, map[string]any{"bunshin.next_action": "unknown"})
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("want END for unmatched case, got %q", next)
	}
}

func TestContentBased_ExplicitDefault(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "content_based",
		Config: map[string]any{
			"key":     "k",
			"cases":   map[string]any{"yes": "node-a"},
			"default": "node-b",
		},
	})
	s := stateWithMeta(nil, map[string]any{"k": "no"})
	next, _ := r(context.Background(), s)
	if next != "node-b" {
		t.Errorf("want node-b default, got %q", next)
	}
}

func TestContentBased_MissingKey_Error(t *testing.T) {
	_, err := reg(t).Build(&workflow.RouterRef{
		Type:   "content_based",
		Config: map[string]any{"cases": map[string]any{}},
	}, workflow.NewRunnableRegistry())
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestContentBased_MissingCases_Error(t *testing.T) {
	_, err := reg(t).Build(&workflow.RouterRef{
		Type:   "content_based",
		Config: map[string]any{"key": "k"},
	}, workflow.NewRunnableRegistry())
	if err == nil {
		t.Error("expected error for missing cases")
	}
}

func TestContentBased_MissingMeta_RoutesToDefault(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "content_based",
		Config: map[string]any{
			"key":   "bunshin.next_action",
			"cases": map[string]any{"done": "end-node"},
		},
	})
	// Meta key absent — should route to default (END)
	next, _ := r(context.Background(), state(nil))
	if next != graph.END {
		t.Errorf("want END for absent meta key, got %q", next)
	}
}

// ---- Message Filter ----

func TestFilter_Exists_Pass(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "result", "op": "exists", "next": "process"},
	})
	next, _ := r(context.Background(), state(map[string]any{"result": "ok"}))
	if next != "process" {
		t.Errorf("want process, got %q", next)
	}
}

func TestFilter_Exists_Fail_RoutesToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "result", "op": "exists", "next": "process"},
	})
	next, _ := r(context.Background(), state(map[string]any{}))
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestFilter_Eq_Pass(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "status", "op": "eq", "value": "ready", "next": "go"},
	})
	next, _ := r(context.Background(), state(map[string]any{"status": "ready"}))
	if next != "go" {
		t.Errorf("want go, got %q", next)
	}
}

func TestFilter_Neq_Pass(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "status", "op": "neq", "value": "error", "next": "go"},
	})
	next, _ := r(context.Background(), state(map[string]any{"status": "ok"}))
	if next != "go" {
		t.Errorf("want go, got %q", next)
	}
}

func TestFilter_Truthy_Pass(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "flag", "op": "truthy", "next": "go"},
	})
	next, _ := r(context.Background(), state(map[string]any{"flag": true}))
	if next != "go" {
		t.Errorf("want go, got %q", next)
	}
}

func TestFilter_Truthy_False_RoutesToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "flag", "op": "truthy", "next": "go"},
	})
	next, _ := r(context.Background(), state(map[string]any{"flag": false}))
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestFilter_UnknownOp_Error(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"key": "k", "op": "bogus", "next": "n"},
	})
	_, err := r(context.Background(), state(map[string]any{"k": "v"}))
	if err == nil {
		t.Error("expected error for unknown op")
	}
}

func TestFilter_MissingKey_Error(t *testing.T) {
	_, err := reg(t).Build(&workflow.RouterRef{
		Type:   "filter",
		Config: map[string]any{"op": "exists", "next": "n"},
	}, workflow.NewRunnableRegistry())
	if err == nil {
		t.Error("expected error for missing key")
	}
}

// ---- Routing Slip ----

func TestRoutingSlip_AdvancesThrough(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	s := stateWithMeta(nil, map[string]any{"slip": []string{"a", "b", "c"}, "pos": 0})
	next, _ := r(context.Background(), s)
	if next != "a" {
		t.Errorf("want a, got %q", next)
	}
	if s.Meta["pos"] != 1 {
		t.Errorf("want pos=1, got %v", s.Meta["pos"])
	}
}

func TestRoutingSlip_EndsAtEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	s := stateWithMeta(nil, map[string]any{"slip": []string{"a"}, "pos": 1})
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestRoutingSlip_NoSlip_RoutesToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	next, _ := r(context.Background(), stateWithMeta(nil, nil))
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestRoutingSlip_SliceOfAny(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "routing_slip",
		Config: map[string]any{"slip_key": "slip", "pos_key": "pos"},
	})
	s := stateWithMeta(nil, map[string]any{"slip": []any{"x", "y"}, "pos": 0})
	next, _ := r(context.Background(), s)
	if next != "x" {
		t.Errorf("want x, got %q", next)
	}
}

// ---- Recipient List ----

func TestRecipientList_AdvancesThrough(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "recipient_list",
		Config: map[string]any{"recipients": []any{"summarizer", "translator"}, "pos_key": "rpos"},
	})
	s := stateWithMeta(nil, map[string]any{"rpos": 0})
	next, _ := r(context.Background(), s)
	if next != "summarizer" {
		t.Errorf("want summarizer, got %q", next)
	}
	if s.Meta["rpos"] != 1 {
		t.Errorf("want rpos=1, got %v", s.Meta["rpos"])
	}
}

func TestRecipientList_EndsAtEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type:   "recipient_list",
		Config: map[string]any{"recipients": []any{"a"}, "pos_key": "rpos"},
	})
	s := stateWithMeta(nil, map[string]any{"rpos": 1})
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestRecipientList_MissingRecipients_Error(t *testing.T) {
	_, err := reg(t).Build(&workflow.RouterRef{
		Type:   "recipient_list",
		Config: map[string]any{"pos_key": "p"},
	}, workflow.NewRunnableRegistry())
	if err == nil {
		t.Error("expected error for missing recipients")
	}
}

// ---- Splitter ----

func TestSplitter_YieldsItems(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "splitter",
		Config: map[string]any{
			"input_key": "chunks", "item_key": "chunk", "pos_key": "spos", "next": "process",
		},
	})
	s := stateWithMeta(map[string]any{"chunks": []any{"a", "b"}}, map[string]any{"spos": 0})
	next, _ := r(context.Background(), s)
	if next != "process" {
		t.Errorf("want process, got %q", next)
	}
	if s.Data["chunk"] != "a" {
		t.Errorf("want chunk=a, got %v", s.Data["chunk"])
	}
	if s.Meta["spos"] != 1 {
		t.Errorf("want spos=1, got %v", s.Meta["spos"])
	}
}

func TestSplitter_EndsAtEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "splitter",
		Config: map[string]any{
			"input_key": "chunks", "item_key": "chunk", "pos_key": "spos", "next": "process",
		},
	})
	s := stateWithMeta(map[string]any{"chunks": []any{"a"}}, map[string]any{"spos": 1})
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestSplitter_MissingInputKey_RoutesToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "splitter",
		Config: map[string]any{
			"input_key": "missing", "item_key": "item", "pos_key": "p", "next": "n",
		},
	})
	next, _ := r(context.Background(), state(map[string]any{}))
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestSplitter_WrongType_Error(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "splitter",
		Config: map[string]any{
			"input_key": "chunks", "item_key": "item", "pos_key": "p", "next": "n",
		},
	})
	s := state(map[string]any{"chunks": "not-a-slice"})
	_, err := r(context.Background(), s)
	if err == nil {
		t.Error("expected error for non-slice input")
	}
}

// ---- Aggregator ----

func TestAggregator_CollectsItems(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "aggregator",
		Config: map[string]any{
			"item_key": "item", "output_key": "results", "count_key": "total", "next": "done",
		},
	})
	// First item — count=2, only 1 collected → not done yet
	s := stateWithMeta(map[string]any{"item": "first"}, map[string]any{"total": 2})
	next, _ := r(context.Background(), s)
	if next != graph.END {
		t.Errorf("want END (not done yet), got %q", next)
	}
	acc := s.Data["results"].([]any)
	if len(acc) != 1 || acc[0] != "first" {
		t.Errorf("unexpected accumulator: %v", acc)
	}

	// Second item — count=2, 2 collected → route to done
	s.Data["item"] = "second"
	next, _ = r(context.Background(), s)
	if next != "done" {
		t.Errorf("want done after 2 items, got %q", next)
	}
	acc2 := s.Data["results"].([]any)
	if len(acc2) != 2 {
		t.Errorf("want 2 items, got %v", acc2)
	}
}

func TestAggregator_MissingItem_RoutesToEND(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "aggregator",
		Config: map[string]any{
			"item_key": "item", "output_key": "out", "count_key": "cnt", "next": "done",
		},
	})
	next, _ := r(context.Background(), state(map[string]any{}))
	if next != graph.END {
		t.Errorf("want END, got %q", next)
	}
}

func TestAggregator_WrongOutputType_Error(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "aggregator",
		Config: map[string]any{
			"item_key": "item", "output_key": "out", "count_key": "total", "next": "done",
		},
	})
	// Existing output is []string, not []any — should error.
	s := stateWithMeta(map[string]any{"item": "x", "out": []string{"existing"}}, map[string]any{"total": 2})
	_, err := r(context.Background(), s)
	if err == nil {
		t.Error("expected error when output_key exists with wrong type")
	}
}

func TestAggregator_MissingCountKey_NeverDone(t *testing.T) {
	r := build(t, reg(t), &workflow.RouterRef{
		Type: "aggregator",
		Config: map[string]any{
			"item_key": "item", "output_key": "out", "count_key": "missing_cnt", "next": "done",
		},
	})
	s := state(map[string]any{"item": "x"})
	next, _ := r(context.Background(), s)
	// count_key absent → expected 0 → never done → END
	if next != graph.END {
		t.Errorf("want END when count unknown, got %q", next)
	}
}
