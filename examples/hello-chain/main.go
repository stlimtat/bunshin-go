// hello-chain demonstrates a two-step chain:
// Step 1 (fast model): extract key entities from input text.
// Step 2 (reasoning model): analyse the entities and produce a summary.
//
// Both steps use the fake provider here; swap to real providers by
// replacing the Runnable implementations with LLM calls.
//
// Run:
//
//	go run ./examples/hello-chain
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/stlimtat/bunshin-go/pkg/chain"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

// extractEntities simulates a fast-model extraction step.
// In production: call an LLM with a structured extraction prompt.
var extractEntities = core.NewRunnableFunc("extract-entities", func(_ context.Context, input any) (any, error) {
	text := input.(string)
	// Naive word-count stub — replace with LLM call.
	words := strings.Fields(text)
	entities := make([]string, 0)
	for _, w := range words {
		if len(w) > 5 {
			entities = append(entities, w)
		}
	}
	return map[string]any{
		"original": text,
		"entities": entities,
	}, nil
})

// analyseEntities simulates a slow reasoning-model analysis step.
var analyseEntities = core.NewRunnableFunc("analyse-entities", func(_ context.Context, input any) (any, error) {
	data := input.(map[string]any)
	entities := data["entities"].([]string)
	return fmt.Sprintf("Found %d notable entities: %v", len(entities), entities), nil
})

func main() {
	// Build chain: fast extraction → reasoning analysis.
	c := chain.New("entity-pipeline",
		chain.SWithModel("extract", extractEntities, llm.ModelConfig{
			Provider: llm.ProviderOpenAI,
			Model:    "gpt-4o-mini",
			Tier:     llm.TierFast,
		}),
		chain.SWithModel("analyse", analyseEntities, llm.ModelConfig{
			Provider: llm.ProviderOpenAI,
			Model:    "o3",
			Tier:     llm.TierReasoning,
		}),
	)

	// Wrap with logging and retry middleware.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	wrapped := middleware.Chain(c,
		middleware.WithLogging(logger),
		middleware.WithPanicRecovery(),
	)

	input := "The quick brown fox jumps over the lazy dog near the riverside"
	result, err := wrapped.Invoke(context.Background(), input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}
