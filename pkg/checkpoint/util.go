package checkpoint

import "encoding/json"

func mapToJSON(m map[string]any) []byte {
	if m == nil {
		return nil
	}
	data, _ := json.Marshal(m)
	return data
}

func jsonToMap(data []byte, m *map[string]any) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, m)
}
