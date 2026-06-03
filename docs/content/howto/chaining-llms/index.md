+++
title = 'Chaining LLMs in Workflow Nodes'
date = '2026-06-03'
draft = false
weight = 1
+++

# Chaining LLMs in Workflow Nodes

bunshin-go lets you assign a different LLM provider to each node or step. This guide covers four patterns: same-provider chains, mixed-provider chains, fallback chains, and graph-based routing with per-node model selection.

---

## Pattern 1: Same-provider sequential chain

Use one fast model for extraction and one smart model for reasoning — both from the same provider.

```go
package main

import (
    "context"
    "fmt"

    "github.com/stlimtat/bunshin-go/pkg/chain"
    "github.com/stlimtat/bunshin-go/pkg/core"
    "github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
    ctx := context.Background()
    key := "sk-..."

    fast := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey: key,
        Model:  "gpt-4o-mini",
    })
    smart := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey: key,
        Model:  "gpt-4o",
    })

    extractStep := core.NewRunnableFunc("extract", func(ctx context.Context, input any) (any, error) {
        resp, err := fast.Complete(ctx, &llm.Request{
            Messages: []llm.Message{{
                Role:  llm.RoleUser,
                Parts: []llm.ContentPart{{Text: fmt.Sprintf("Extract key facts from: %v", input)}},
            }},
        })
        if err != nil {
            return nil, err
        }
        return resp.Content, nil
    })

    reasonStep := core.NewRunnableFunc("reason", func(ctx context.Context, input any) (any, error) {
        resp, err := smart.Complete(ctx, &llm.Request{
            Messages: []llm.Message{{
                Role:  llm.RoleUser,
                Parts: []llm.ContentPart{{Text: fmt.Sprintf("Analyse and synthesise: %v", input)}},
            }},
        })
        if err != nil {
            return nil, err
        }
        return resp.Content, nil
    })

    pipeline := chain.New("fast-then-smart",
        chain.Step{ID: "extract", Runnable: extractStep},
        chain.Step{ID: "reason", Runnable: reasonStep},
    )

    result, err := pipeline.Invoke(ctx, "Go is a statically typed, compiled language...")
    fmt.Println(result, err)
}
```

**Why this works well**: `gpt-4o-mini` is 10× cheaper for extraction; `gpt-4o` only runs on the distilled output, keeping cost low while quality is high.

---

## Pattern 2: Mixed-provider chain (OpenAI → Anthropic)

Different providers shine at different tasks. Route document summarisation through Claude for its long context window, then pass the summary to OpenAI for structured JSON extraction.

```go
anthropic := llm.NewAnthropicProvider(llm.AnthropicConfig{
    APIKey: "sk-ant-...",
    Model:  "claude-3-5-sonnet-20241022",
})
openai := llm.NewOpenAIProvider(llm.OpenAIConfig{
    APIKey: "sk-...",
    Model:  "gpt-4o",
})

summariseStep := core.NewRunnableFunc("summarise", func(ctx context.Context, input any) (any, error) {
    resp, err := anthropic.Complete(ctx, &llm.Request{
        Messages: []llm.Message{{
            Role:  llm.RoleUser,
            Parts: []llm.ContentPart{{Text: fmt.Sprintf("Summarise this document: %v", input)}},
        }},
    })
    if err != nil {
        return nil, err
    }
    return resp.Content, nil
})

structureStep := core.NewRunnableFunc("structure", func(ctx context.Context, input any) (any, error) {
    resp, err := openai.Complete(ctx, &llm.Request{
        Messages: []llm.Message{
            {Role: llm.RoleSystem, Parts: []llm.ContentPart{{Text: "Return valid JSON only."}}},
            {Role: llm.RoleUser, Parts: []llm.ContentPart{{Text: fmt.Sprintf(
                "Extract {title, keyPoints[], sentiment} from: %v", input,
            )}}},
        },
    })
    if err != nil {
        return nil, err
    }
    return resp.Content, nil
})

pipeline := chain.New("claude-then-openai",
    chain.Step{ID: "summarise", Runnable: summariseStep},
    chain.Step{ID: "structure", Runnable: structureStep},
)
```

---

## Pattern 3: Fallback chain

Wrap providers in `FallbackProvider` so if the primary is unavailable, the chain continues with the secondary.

```go
primary := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: primaryKey, Model: "gpt-4o"})
secondary := llm.NewAnthropicProvider(llm.AnthropicConfig{APIKey: anthropicKey, Model: "claude-3-5-sonnet-20241022"})
fallback := llm.NewFallbackProvider(primary, secondary)

// fallback.Complete tries primary first; on error, tries secondary
step := core.NewRunnableFunc("llm-with-fallback", func(ctx context.Context, input any) (any, error) {
    resp, err := fallback.Complete(ctx, &llm.Request{
        Messages: []llm.Message{{
            Role:  llm.RoleUser,
            Parts: []llm.ContentPart{{Text: fmt.Sprintf("%v", input)}},
        }},
    })
    if err != nil {
        return nil, err
    }
    return resp.Content, nil
})
```

For production use, combine with `ProviderRegistry` which background-pings providers and skips unhealthy ones automatically:

```go
registry := llm.NewProviderRegistry(primary, secondary, tertiary)
registry.Start(ctx)

// Available() returns only providers that passed their last ping
live := registry.Available()
if len(live) == 0 {
    return errors.New("no LLM providers available")
}
activeProvider := live[0]
```

---

## Pattern 4: Graph with per-node model selection

A graph lets each node independently choose its provider. This is the natural pattern for agent loops where routing, tool calling, and response generation have different latency/cost trade-offs.

```go
import "github.com/stlimtat/bunshin-go/pkg/graph"

// Classify intent with a fast model
classifyNode := graph.Node{
    ID: "classify",
    Runnable: core.NewRunnableFunc("classify", func(ctx context.Context, input any) (any, error) {
        resp, err := fastModel.Complete(ctx, &llm.Request{
            Messages: []llm.Message{{
                Role:  llm.RoleUser,
                Parts: []llm.ContentPart{{Text: fmt.Sprintf("Classify: tool_call or answer? Input: %v", input)}},
            }},
        })
        if err != nil {
            return nil, err
        }
        // Return a map so the router can inspect both the classification and original input
        return map[string]any{"intent": resp.Content, "input": input}, nil
    }),
    Router: func(ctx context.Context, out any) (string, error) {
        m := out.(map[string]any)
        if strings.Contains(fmt.Sprint(m["intent"]), "tool_call") {
            return "tool_exec", nil
        }
        return "answer", nil
    },
}

// Execute tool with medium model
toolNode := graph.Node{
    ID: "tool_exec",
    Runnable: core.NewRunnableFunc("tool", func(ctx context.Context, input any) (any, error) {
        // ... run tool, return result
        return toolResult, nil
    }),
    Router: func(_ context.Context, _ any) (string, error) {
        return "answer", nil // Always route to answer after tool
    },
}

// Generate final answer with smart model
answerNode := graph.Node{
    ID: "answer",
    Runnable: core.NewRunnableFunc("answer", func(ctx context.Context, input any) (any, error) {
        resp, err := smartModel.Complete(ctx, &llm.Request{
            Messages: []llm.Message{{
                Role:  llm.RoleUser,
                Parts: []llm.ContentPart{{Text: fmt.Sprintf("Answer based on: %v", input)}},
            }},
        })
        if err != nil {
            return nil, err
        }
        return resp.Content, nil
    }),
    // No Router = terminal node
}

g := graph.New("agent")
g.AddNode(classifyNode)
g.AddNode(toolNode)
g.AddNode(answerNode)
g.SetEntry("classify")

result, err := g.Invoke(ctx, "What is the weather in Singapore?")
```

---

## Cost optimisation tips

1. **Fast model for routing** — use `gpt-4o-mini` or `claude-haiku` for classification; save smart models for generation.
2. **Streaming last step only** — `chain.Stream` runs all but the last step via `Invoke`. Only the last step pays streaming overhead.
3. **Same-provider fast-path** — consecutive steps on the same provider skip message translation. Keep provider consistent within a sub-chain when possible.
4. **FallbackProvider vs ProviderRegistry** — use `FallbackProvider` for request-level fallback (primary fails → try secondary); use `ProviderRegistry` for health-check-based routing (only route to providers confirmed healthy).
