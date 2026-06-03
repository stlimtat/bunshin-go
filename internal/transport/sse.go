package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func writeSSE(w http.ResponseWriter, f http.Flusher, event StreamEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	f.Flush()
	return nil
}
