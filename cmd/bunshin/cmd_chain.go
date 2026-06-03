package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/pkg/chain"
	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/llm"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

func newChainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chain",
		Short: "Two-step entity extraction chain demo",
		Long: `Demonstrates a two-step sequential chain:

  Step 1 (fast model):      Extract key entities (words longer than 5 chars).
  Step 2 (reasoning model): Summarise entity count and list.

Provider and model are selected from --provider / BUNSHIN_PROVIDER.`,
		Example: `  bunshin chain --input "Apple and Google are big tech companies"
  BUNSHIN_PROVIDER=openai BUNSHIN_API_KEY=sk-... bunshin chain \
    --input "Anthropic and OpenAI are AI labs"`,
		RunE: runChain,
	}
	cmd.Flags().String("input", "The quick brown fox jumps over the lazy dog near the riverside", "Text to process")
	mustBindFlag(cmd, "chain_input", "input")
	return cmd
}

var extractEntities = core.NewRunnableFunc("extract-entities", func(_ context.Context, input any) (any, error) {
	text, ok := input.(string)
	if !ok {
		return nil, fmt.Errorf("extract-entities: expected string input, got %T", input)
	}
	words := strings.Fields(text)
	var entities []string
	for _, w := range words {
		if len(w) > 5 {
			entities = append(entities, w)
		}
	}
	return map[string]any{"original": text, "entities": entities}, nil
})

var analyseEntities = core.NewRunnableFunc("analyse-entities", func(_ context.Context, input any) (any, error) {
	data, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("analyse-entities: expected map input, got %T", input)
	}
	entities, ok := data["entities"].([]string)
	if !ok {
		return nil, fmt.Errorf("analyse-entities: entities field missing or wrong type")
	}
	return fmt.Sprintf("Found %d notable entities: %v", len(entities), entities), nil
})

func runChain(_ *cobra.Command, _ []string) error {
	cfg := loadConfig()
	input := viper.GetString("chain_input")

	providerID := llm.ProviderID(cfg.Provider)

	c := chain.New("entity-pipeline",
		chain.SWithModel("extract", extractEntities, llm.ModelConfig{
			Provider: providerID,
			Model:    llm.ModelID(cfg.Model),
			Tier:     llm.TierFast,
		}),
		chain.SWithModel("analyse", analyseEntities, llm.ModelConfig{
			Provider: providerID,
			Model:    llm.ModelID(cfg.Model),
			Tier:     llm.TierReasoning,
		}),
	)

	wrapped := middleware.Chain(c, middleware.WithLogging(log.Logger), middleware.WithPanicRecovery())

	result, err := wrapped.Invoke(context.Background(), input)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}
