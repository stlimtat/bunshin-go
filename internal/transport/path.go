package transport

import "strings"

// IsStreamPath reports whether path ends with "/stream".
func IsStreamPath(path string) bool {
	return strings.HasSuffix(path, "/stream")
}

// WorkflowIDFromPath extracts the workflow ID from a path like /workflows/{id} or /workflows/{id}/stream.
// Returns empty string if the path has fewer than 2 non-empty segments or any segment is empty.
func WorkflowIDFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[1] == "" {
		return ""
	}
	return parts[1]
}
