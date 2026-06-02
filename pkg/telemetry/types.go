package telemetry

import (
	"time"

	"github.com/google/uuid"
)

// RunType classifies a LangSmith run.
type RunType string

const (
	RunTypeChain     RunType = "chain"
	RunTypeLLM       RunType = "llm"
	RunTypeTool      RunType = "tool"
	RunTypeRetriever RunType = "retriever"
	RunTypePrompt    RunType = "prompt"
)

// Run is a single unit of LangSmith tracing.
// Parent-child relationships form the run tree visible in the LangSmith UI.
type Run struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Name        string
	RunType     RunType
	Inputs      map[string]any
	Outputs     map[string]any
	Tags        []string
	Usage       *TokenUsage
	Error       *string
	StartTime   time.Time
	EndTime     *time.Time
	SessionName string
}

// TokenUsage reports token consumption for an LLM run.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

// Feedback is human or automated feedback on a specific run.
type Feedback struct {
	Key     string
	Score   *float64
	Comment string
	Source  string // "human", "model", "automated"
}
