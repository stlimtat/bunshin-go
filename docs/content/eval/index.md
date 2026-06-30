+++
title = 'Evaluation'
date = '2026-06-30'
draft = false
toc = true
weight = 8
+++

# Evaluation

`pkg/eval` provides an evaluation harness for measuring LLM pipeline quality. Plug in a dataset of (input, expected-output) pairs, run them through any `Runnable`, and collect scores.

---

## Two operating modes

| Mode | When to use |
|------|-------------|
| **Standalone** | Local CI; results written to JSON. No external services required. |
| **LangSmith-synced** | Push dataset before run, push report after. Results appear as an Experiment in the LangSmith UI. |

---

## Core types

```
Dataset  →  []Example  →  EvalRunner.Run  →  []EvalRun  →  []Evaluator  →  EvalReport
```

| Type | Role |
|------|------|
| `Example` | One test case: `Input map[string]any` + `Reference map[string]any` |
| `Dataset` | Named collection of `Example`s; can be pushed to/pulled from LangSmith |
| `EvalRun` | Output of running one `Example` through a `Runnable`; includes `Actual`, `Latency`, `Err` |
| `Score` | One evaluator's verdict: `Key` (e.g. `"correctness"`), `Value` (0–1), `Comment` |
| `EvalReport` | Aggregate: mean scores per evaluator, all `EvalRun`s, per-example `ScoreMap` |
| `Evaluator` | Interface: `Name() string; Evaluate(ctx, *EvalRun) (*Score, error)` |
| `DatasetBackend` | Interface for pushing/pulling datasets (local JSON or LangSmith) |

---

## Built-in evaluators

| Evaluator | What it checks |
|-----------|---------------|
| `ExactMatch` | `Actual[key] == Reference[key]` |
| `ContainsAll` | All reference strings appear in actual output |
| `Latency` | Scores 1.0 if latency ≤ threshold, 0.0 otherwise |

---

## Quickstart

```go
import "github.com/stlimtat/bunshin-go/pkg/eval"

// 1. Define a dataset
dataset := &eval.Dataset{
    Name: "capital-cities",
    Examples: []*eval.Example{
        {
            Input:     map[string]any{"question": "Capital of France?"},
            Reference: map[string]any{"answer": "Paris"},
        },
        {
            Input:     map[string]any{"question": "Capital of Japan?"},
            Reference: map[string]any{"answer": "Tokyo"},
        },
    },
}

// 2. Wire up your Runnable (any bunshin-go pipeline)
myPipeline := core.NewRunnableFunc("qa", func(ctx context.Context, input any) (any, error) {
    in := input.(map[string]any)
    resp, err := provider.Complete(ctx, &llm.Request{
        Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, in["question"].(string))},
    })
    return map[string]any{"answer": resp.Content}, err
})

// 3. Run the evaluation
runner := eval.NewRunner(myPipeline, eval.RunnerConfig{Concurrency: 4})
report, err := runner.Run(ctx, dataset, []eval.Evaluator{
    eval.ExactMatch("answer"),
    eval.Latency(500 * time.Millisecond),
})

// 4. Inspect results
fmt.Printf("ExactMatch: %.2f\n", report.Scores["exact_match"])
fmt.Printf("Latency:    %.2f\n", report.Scores["latency"])
```

---

## LangSmith sync

```go
backend := eval.NewLangSmithBackend(eval.LangSmithConfig{
    APIKey:  os.Getenv("LANGSMITH_API_KEY"),
    Project: "bunshin-prod",
})

// Push dataset once (idempotent)
backend.Push(ctx, dataset)

// After running, push the report — appears as an Experiment in LangSmith UI
backend.PushResults(ctx, report)
```

---

## Writing a custom evaluator

```go
type SemanticSimilarity struct{ threshold float64 }

func (s SemanticSimilarity) Name() string { return "semantic_similarity" }

func (s SemanticSimilarity) Evaluate(ctx context.Context, run *eval.EvalRun) (*eval.Score, error) {
    actual    := run.Actual["answer"].(string)
    reference := run.Reference["answer"].(string)
    score := cosineSimilarity(embed(actual), embed(reference))
    return &eval.Score{
        Key:     s.Name(),
        Value:   score,
        Comment: fmt.Sprintf("%.3f ≥ %.3f: %v", score, s.threshold, score >= s.threshold),
    }, nil
}
```
