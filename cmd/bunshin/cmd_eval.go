package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stlimtat/bunshin-go/pkg/eval"
	"github.com/stlimtat/bunshin-go/pkg/llm"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Manage and run eval suites",
	}
	cmd.AddCommand(
		newEvalListCmd(),
		newEvalShowCmd(),
		newEvalCreateCmd(),
		newEvalUpdateCmd(),
		newEvalDeleteCmd(),
		newEvalRunCmd(),
	)
	return cmd
}

func newEvalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List eval suites",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("eval list: not yet implemented")
			return nil
		},
	}
}

func newEvalShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show eval config and last results",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval show %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newEvalCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a new eval suite",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("eval create: not yet implemented")
			return nil
		},
	}
}

func newEvalUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <id>",
		Short: "Update eval configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval update %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newEvalDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft delete an eval suite",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fmt.Printf("eval delete %q: not yet implemented\n", args[0])
			return nil
		},
	}
}

func newEvalRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an eval suite from a JSONL dataset and print results",
		Long: `Runs every example in a JSONL dataset through the configured LLM provider,
scores with ExactMatch and ContainsAll evaluators, then prints a report.

Each JSONL line must have "input.message" for the user prompt and
"reference.output" for the expected LLM response.

Example dataset line:
  {"input":{"message":"Say hello"},"reference":{"output":"hello"}}`,
		Example: `  bunshin eval run --dataset testdata/greet.jsonl
  bunshin eval run --dataset testdata/greet.jsonl --concurrency 10`,
		RunE: runEvalRun,
	}
	cmd.Flags().String("dataset", "", "Path to JSONL dataset file (required)")
	cmd.Flags().Int("concurrency", 5, "Number of examples to run concurrently")
	_ = cmd.MarkFlagRequired("dataset")
	mustBindFlag(cmd, "eval_dataset", "dataset")
	return cmd
}

func runEvalRun(cmd *cobra.Command, _ []string) error {
	datasetPath, _ := cmd.Flags().GetString("dataset")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	cfg := loadConfig()
	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}

	// Load dataset from JSONL.
	dir := filepath.Dir(datasetPath)
	name := strings.TrimSuffix(filepath.Base(datasetPath), ".jsonl")
	backend := eval.NewJSONLDatasetBackend(dir)
	ds, err := backend.Pull(context.Background(), name)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}

	fmt.Printf("loaded %d examples from %q\n", len(ds.Examples), datasetPath)

	// Build the evaluation function: send input.message to the LLM.
	fn := func(ctx context.Context, input map[string]any) (map[string]any, error) {
		msg, _ := input["message"].(string)
		if msg == "" {
			return map[string]any{"output": ""}, nil
		}
		req := &llm.Request{
			Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, msg)},
		}
		resp, err := provider.Complete(ctx, req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"output": resp.Content}, nil
	}

	// Use built-in evaluators. Both check run.Actual["output"] vs run.Reference["output"].
	evaluators := []eval.Evaluator{
		&eval.ExactMatch{Key: "output"},
	}

	runner := eval.NewEvalRunner(fn, evaluators, concurrency)
	report, err := runner.Run(context.Background(), ds)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	// Print summary.
	fmt.Printf("\n=== Eval Report ===\n")
	fmt.Printf("Examples:  %d\n", len(report.Runs))
	fmt.Printf("Duration:  %v\n", report.EndTime.Sub(report.StartTime))
	fmt.Printf("\nScores:\n")
	for k, v := range report.Scores {
		fmt.Printf("  %-20s %.3f\n", k, v)
	}

	// Count errors.
	errs := 0
	for _, r := range report.Runs {
		if r.Err != nil {
			errs++
		}
	}
	if errs > 0 {
		fmt.Printf("\nErrors: %d / %d examples failed\n", errs, len(report.Runs))
	}

	// Persist results alongside the dataset.
	_ = backend.PushResults(context.Background(), report)

	// Optionally print full results as JSON.
	if viper.GetBool("eval_verbose") {
		out, _ := json.MarshalIndent(report.ScoreMap, "", "  ")
		fmt.Println(string(out))
	}
	return nil
}
