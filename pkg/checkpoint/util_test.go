package checkpoint

import "testing"

func TestMapToJSON_Nil(t *testing.T) {
	if mapToJSON(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestMapToJSON_Values(t *testing.T) {
	data := mapToJSON(map[string]any{"key": "val"})
	if string(data) != `{"key":"val"}` {
		t.Errorf("unexpected JSON: %s", data)
	}
}

func TestJsonToMap_Empty(t *testing.T) {
	m := map[string]any{}
	if err := jsonToMap(nil, &m); err != nil {
		t.Errorf("unexpected error for nil: %v", err)
	}
	if err := jsonToMap([]byte("null"), &m); err != nil {
		t.Errorf("unexpected error for null: %v", err)
	}
}

func TestJsonToMap_Values(t *testing.T) {
	m := map[string]any{}
	if err := jsonToMap([]byte(`{"x":1}`), &m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["x"] == nil {
		t.Error("expected x in map")
	}
}
