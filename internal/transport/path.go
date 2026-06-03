package transport

// IsStreamPath reports whether path ends with "/stream".
func IsStreamPath(path string) bool {
	return len(path) > 7 && path[len(path)-7:] == "/stream"
}

// WorkflowIDFromPath extracts the workflow ID from a path like /workflows/{id} or /workflows/{id}/stream.
func WorkflowIDFromPath(path string) string {
	parts := splitPath(path)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func splitPath(path string) []string {
	var parts []string
	start := 0
	for i, c := range path {
		if c == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}
