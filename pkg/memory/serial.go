package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/stlimtat/bunshin-go/pkg/llm"
)

// msgRow is the JSON-serializable form of llm.Message.
// The native cache and io.Reader blobs are intentionally excluded — they
// are not portable across process boundaries.
type msgRow struct {
	Role  string    `json:"role"`
	Parts []partRow `json:"parts"`
}

type partRow struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	MediaURL   string         `json:"media_url,omitempty"`
	MimeType   string         `json:"mime_type,omitempty"`
	MediaData  []byte         `json:"media_data,omitempty"`
	ToolCall   *toolCallRow   `json:"tool_call,omitempty"`
	ToolResult *toolResultRow `json:"tool_result,omitempty"`
}

type toolCallRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolResultRow struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// marshal converts a Message to JSON bytes for storage.
func marshal(msg llm.Message) ([]byte, error) {
	row := msgRow{Role: string(msg.Role)}
	for _, p := range msg.Parts {
		pr := partRow{Type: string(p.Type), Text: p.Text}
		if p.Media != nil {
			pr.MediaURL = p.Media.URL
			pr.MimeType = p.Media.MimeType
			if p.Media.Data != nil {
				data, err := io.ReadAll(p.Media.Data)
				if err != nil {
					return nil, fmt.Errorf("memory: marshal media: %w", err)
				}
				pr.MediaData = data
				// Restore the reader so the original message remains usable.
				p.Media.Data = bytes.NewReader(data)
			}
		}
		if p.ToolCall != nil {
			pr.ToolCall = &toolCallRow{
				ID:        p.ToolCall.ID,
				Name:      p.ToolCall.Name,
				Arguments: p.ToolCall.Arguments,
			}
		}
		if p.ToolResult != nil {
			pr.ToolResult = &toolResultRow{
				ToolCallID: p.ToolResult.ToolCallID,
				Content:    p.ToolResult.Content,
			}
		}
		row.Parts = append(row.Parts, pr)
	}
	return json.Marshal(row)
}

// unmarshal reconstructs a Message from its JSON representation.
func unmarshal(data []byte) (llm.Message, error) {
	var row msgRow
	if err := json.Unmarshal(data, &row); err != nil {
		return llm.Message{}, fmt.Errorf("memory: unmarshal: %w", err)
	}

	msg := llm.Message{Role: llm.Role(row.Role)}
	for _, pr := range row.Parts {
		p := llm.ContentPart{
			Type: llm.ContentPartType(pr.Type),
			Text: pr.Text,
		}
		if pr.MediaURL != "" || len(pr.MediaData) > 0 {
			ref := &llm.MediaRef{
				URL:      pr.MediaURL,
				MimeType: pr.MimeType,
			}
			if len(pr.MediaData) > 0 {
				ref.Data = bytes.NewReader(pr.MediaData)
				ref.Size = int64(len(pr.MediaData))
			}
			p.Media = ref
		}
		if pr.ToolCall != nil {
			p.ToolCall = &llm.ToolCall{
				ID:        pr.ToolCall.ID,
				Name:      pr.ToolCall.Name,
				Arguments: pr.ToolCall.Arguments,
			}
		}
		if pr.ToolResult != nil {
			p.ToolResult = &llm.ToolResult{
				ToolCallID: pr.ToolResult.ToolCallID,
				Content:    pr.ToolResult.Content,
			}
		}
		msg.Parts = append(msg.Parts, p)
	}
	return msg, nil
}
