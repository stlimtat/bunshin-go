// Package eip provides workflow routers modelled on the Apache Camel Enterprise
// Integration Patterns (https://camel.apache.org/components/latest/eips/).
//
// All routers operate on graph.Router[map[string]any] — the state type used by
// YAML-compiled workflows. They are registered into a workflow.RouterRegistry
// at startup.
//
// # Concurrency
//
// Routers mutate State.Meta (position counters) and State.Data (item/output
// keys) in place. The graph executor MUST NOT invoke a router concurrently on
// the same State value. Sequential node execution (the current Graph[S]
// implementation) satisfies this invariant.
//
// # V1 catalog
//
//   - content_based  — branch on a State.Meta key value
//   - filter         — conditional skip to END
//   - routing_slip   — follow an ordered list of node IDs from State.Meta
//   - recipient_list — fan-out to multiple nodes (use with aggregator)
//   - splitter       — iterate a slice from State.Data, one node call per item
//   - aggregator     — collect splitter / recipient outputs into a slice
//
// # Registration
//
//	import "github.com/stlimtat/bunshin-go/pkg/graph/router/eip"
//
//	eip.Register(myRouterRegistry)
package eip

import (
	"context"
	"fmt"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/graph"
	"github.com/stlimtat/bunshin-go/pkg/workflow"
)

// Register adds all v1 EIP router factories to reg.
func Register(reg *workflow.RouterRegistry) {
	reg.Register("content_based", contentBasedFactory)
	reg.Register("filter", filterFactory)
	reg.Register("routing_slip", routingSlipFactory)
	reg.Register("recipient_list", recipientListFactory)
	reg.Register("splitter", splitterFactory)
	reg.Register("aggregator", aggregatorFactory)
}

// ---- Content-Based Router ----

// contentBasedFactory builds a router that reads State.Meta[key] and routes
// to the node named in cases[value], or to defaultNext if no case matches.
// When the Meta key is absent the router routes to defaultNext.
//
// Config keys:
//
//	key     string            — Meta key to inspect (required)
//	cases   map[string]string — string value → node ID (required)
//	default string            — fallback node ID; defaults to graph.END
func contentBasedFactory(config map[string]any) (graph.Router[map[string]any], error) {
	key, err := requireString(config, "key")
	if err != nil {
		return nil, fmt.Errorf("content_based: %w", err)
	}
	rawCases, ok := config["cases"]
	if !ok {
		return nil, fmt.Errorf("content_based: config missing required key %q", "cases")
	}
	cases, err := toStringMap(rawCases)
	if err != nil {
		return nil, fmt.Errorf("content_based: cases: %w", err)
	}
	defaultNext := graph.END
	if v, ok := config["default"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("content_based: default must be a string")
		}
		defaultNext = s
	}

	return func(_ context.Context, state core.State[map[string]any]) (string, error) {
		v, present := state.GetMeta(key)
		if !present {
			return defaultNext, nil
		}
		next, ok := cases[fmt.Sprintf("%v", v)]
		if !ok {
			return defaultNext, nil
		}
		return next, nil
	}, nil
}

// ---- Message Filter ----

// filterFactory builds a router that evaluates a predicate and routes to END
// when the condition is false (skips remaining nodes).
//
// The "neq" operator treats an absent key as not-equal (pass).
// The "exists" and "eq" operators treat an absent key as failing (END).
//
// Config keys:
//
//	key   string — State.Data key to test (required)
//	op    string — "exists" | "eq" | "neq" | "truthy" (required)
//	value string — rhs for eq/neq (optional)
//	next  string — node ID when condition is true (required)
func filterFactory(config map[string]any) (graph.Router[map[string]any], error) {
	key, err := requireString(config, "key")
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	op, err := requireString(config, "op")
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	next, err := requireString(config, "next")
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	rhsStr, _ := config["value"].(string)

	return func(_ context.Context, state core.State[map[string]any]) (string, error) {
		v, exists := state.Data[key]
		var pass bool
		switch op {
		case "exists":
			pass = exists
		case "eq":
			pass = exists && fmt.Sprintf("%v", v) == rhsStr
		case "neq":
			// Absent key treated as not-equal; document this if surprising.
			pass = !exists || fmt.Sprintf("%v", v) != rhsStr
		case "truthy":
			pass = exists && isTruthy(v)
		default:
			return "", fmt.Errorf("filter: unknown op %q (want exists|eq|neq|truthy)", op)
		}
		if pass {
			return next, nil
		}
		return graph.END, nil
	}, nil
}

// ---- Routing Slip ----

// routingSlipFactory builds a router that reads an ordered list of node IDs
// from State.Meta[slip_key] and advances through them one at a time using a
// position counter stored in State.Meta[pos_key].
//
// State.Meta must be non-nil (use core.NewState to construct state).
// pos_key accepts int, int64, and float64 (JSON/YAML unmarshal produces float64).
//
// Config keys:
//
//	slip_key string — Meta key holding []string of node IDs (required)
//	pos_key  string — Meta key for the current position counter (required)
func routingSlipFactory(config map[string]any) (graph.Router[map[string]any], error) {
	slipKey, err := requireString(config, "slip_key")
	if err != nil {
		return nil, fmt.Errorf("routing_slip: %w", err)
	}
	posKey, err := requireString(config, "pos_key")
	if err != nil {
		return nil, fmt.Errorf("routing_slip: %w", err)
	}

	return func(_ context.Context, state core.State[map[string]any]) (string, error) {
		raw, ok := state.GetMeta(slipKey)
		if !ok {
			return graph.END, nil
		}
		slip, err := toAnyStringSlice(raw)
		if err != nil {
			return "", fmt.Errorf("routing_slip: slip_key: %w", err)
		}

		pos := toInt(state.Meta[posKey])
		if pos < 0 || pos >= len(slip) {
			return graph.END, nil
		}
		ensureMeta(&state)
		state.Meta[posKey] = pos + 1
		return slip[pos], nil
	}, nil
}

// ---- Recipient List ----

// recipientListFactory builds a router that fans out to multiple recipients in
// sequence (one at a time, not concurrently). State.Meta[pos_key] tracks which
// recipient is next.
//
// State.Meta must be non-nil (use core.NewState to construct state).
// pos_key accepts int, int64, and float64.
//
// Config keys:
//
//	recipients []string — ordered node IDs to visit (required)
//	pos_key    string   — Meta key for position tracking (required)
func recipientListFactory(config map[string]any) (graph.Router[map[string]any], error) {
	rawR, ok := config["recipients"]
	if !ok {
		return nil, fmt.Errorf("recipient_list: config missing required key %q", "recipients")
	}
	recips, err := toStringSlice(rawR)
	if err != nil {
		return nil, fmt.Errorf("recipient_list: recipients: %w", err)
	}
	// Copy to prevent external mutation of captured slice.
	recips = append([]string(nil), recips...)

	posKey, err := requireString(config, "pos_key")
	if err != nil {
		return nil, fmt.Errorf("recipient_list: %w", err)
	}

	return func(_ context.Context, state core.State[map[string]any]) (string, error) {
		pos := toInt(state.Meta[posKey])
		if pos < 0 || pos >= len(recips) {
			return graph.END, nil
		}
		ensureMeta(&state)
		state.Meta[posKey] = pos + 1
		return recips[pos], nil
	}, nil
}

// ---- Splitter ----

// splitterFactory builds a router that iterates a slice from State.Data[input_key],
// routing to next_node once per item (setting item_key to the current item).
//
// Accepted slice types: []any, []string, []map[string]any. Other types return an error.
// State.Meta must be non-nil (use core.NewState to construct state).
// pos_key accepts int, int64, and float64.
//
// Config keys:
//
//	input_key string — Data key holding the slice to split (required)
//	item_key  string — Data key to write the current item into (required)
//	pos_key   string — Meta key for position tracking (required)
//	next      string — node ID to execute for each item (required)
func splitterFactory(config map[string]any) (graph.Router[map[string]any], error) {
	inputKey, err := requireString(config, "input_key")
	if err != nil {
		return nil, fmt.Errorf("splitter: %w", err)
	}
	itemKey, err := requireString(config, "item_key")
	if err != nil {
		return nil, fmt.Errorf("splitter: %w", err)
	}
	posKey, err := requireString(config, "pos_key")
	if err != nil {
		return nil, fmt.Errorf("splitter: %w", err)
	}
	next, err := requireString(config, "next")
	if err != nil {
		return nil, fmt.Errorf("splitter: %w", err)
	}

	return func(_ context.Context, state core.State[map[string]any]) (string, error) {
		raw, ok := state.Data[inputKey]
		if !ok {
			return graph.END, nil
		}
		items, err := toAnySlice(raw)
		if err != nil {
			return "", fmt.Errorf("splitter: %s: %w", inputKey, err)
		}

		pos := toInt(state.Meta[posKey])
		if pos < 0 || pos >= len(items) {
			return graph.END, nil
		}
		if state.Data == nil {
			state.Data = make(map[string]any)
		}
		state.Data[itemKey] = items[pos]
		ensureMeta(&state)
		state.Meta[posKey] = pos + 1
		return next, nil
	}, nil
}

// ---- Aggregator ----

// aggregatorFactory builds a router that collects items from State.Data[item_key]
// into a growing slice at State.Data[output_key], routing to next when the
// expected count is reached.
//
// Returns an error if State.Data[output_key] already exists but is not []any.
// Returns graph.END (not an error) when count_key is absent from Meta or is 0.
// count_key accepts int, int64, and float64.
//
// Config keys:
//
//	item_key   string — Data key holding the current item (required)
//	output_key string — Data key accumulating the collected slice (required)
//	count_key  string — Meta key holding the expected total count (required)
//	next       string — node ID to route to after all items collected (required)
func aggregatorFactory(config map[string]any) (graph.Router[map[string]any], error) {
	itemKey, err := requireString(config, "item_key")
	if err != nil {
		return nil, fmt.Errorf("aggregator: %w", err)
	}
	outputKey, err := requireString(config, "output_key")
	if err != nil {
		return nil, fmt.Errorf("aggregator: %w", err)
	}
	countKey, err := requireString(config, "count_key")
	if err != nil {
		return nil, fmt.Errorf("aggregator: %w", err)
	}
	next, err := requireString(config, "next")
	if err != nil {
		return nil, fmt.Errorf("aggregator: %w", err)
	}

	return func(_ context.Context, state core.State[map[string]any]) (string, error) {
		item, ok := state.Data[itemKey]
		if !ok {
			return graph.END, nil
		}
		// Append item to accumulator; error on unexpected existing type.
		var acc []any
		if existing, ok := state.Data[outputKey]; ok {
			a, ok := existing.([]any)
			if !ok {
				return "", fmt.Errorf("aggregator: output_key %q already set to %T, want []any", outputKey, existing)
			}
			acc = a
		}
		acc = append(acc, item)
		state.Data[outputKey] = acc

		expected := toInt(state.Meta[countKey])
		if expected > 0 && len(acc) >= expected {
			return next, nil
		}
		return graph.END, nil
	}, nil
}

// ---- helpers ----

func requireString(config map[string]any, key string) (string, error) {
	v, ok := config[key]
	if !ok {
		return "", fmt.Errorf("config missing required key %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("config key %q must be a string, got %T", key, v)
	}
	return s, nil
}

// toInt coerces common numeric types produced by YAML/JSON unmarshal into int.
// Returns 0 for nil or unrecognised types.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int32:
		return int(n)
	}
	return 0
}

// ensureMeta initialises state.Meta if nil so position writes don't panic.
func ensureMeta(state *core.State[map[string]any]) {
	if state.Meta == nil {
		state.Meta = make(map[string]any)
	}
}

func toStringMap(raw any) (map[string]string, error) {
	switch v := raw.(type) {
	case map[string]string:
		// Copy to prevent external mutation of captured map.
		result := make(map[string]string, len(v))
		for k, val := range v {
			result[k] = val
		}
		return result, nil
	case map[string]any:
		result := make(map[string]string, len(v))
		for k, val := range v {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("value for key %q must be string, got %T", k, val)
			}
			result[k] = s
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected map[string]string, got %T", raw)
	}
}

func toStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		result := make([]string, len(v))
		copy(result, v)
		return result, nil
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("element %d must be string, got %T", i, item)
			}
			result[i] = s
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected []string, got %T", raw)
	}
}

// toAnyStringSlice accepts []string or []any (containing strings).
func toAnyStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("element %d must be string, got %T", i, item)
			}
			result[i] = s
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected []string, got %T", raw)
	}
}

// toAnySlice accepts []any, []string, or []map[string]any.
func toAnySlice(raw any) ([]any, error) {
	switch v := raw.(type) {
	case []any:
		return v, nil
	case []string:
		result := make([]any, len(v))
		for i, s := range v {
			result[i] = s
		}
		return result, nil
	case []map[string]any:
		result := make([]any, len(v))
		for i, m := range v {
			result[i] = m
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected []any|[]string|[]map[string]any, got %T", raw)
	}
}

func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != "" && !strings.EqualFold(val, "false") && val != "0"
	default:
		return true
	}
}
