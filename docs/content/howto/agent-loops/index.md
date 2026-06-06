+++
title = 'Agent Loops: Iterations and Token Budgets'
date = '2026-06-05'
draft = false
weight = 3
toc = true
+++

# Agent Loops: Iterations and Token Budgets

An agent loop is a `Graph[S]` where the Router can route back to an earlier node. The LLM runs, decides whether to call a tool, the tool executes, and the LLM runs again — until a termination condition is met.

This page covers how to bound that loop with both iteration counts and token budgets.

---

## How iteration counting works

Iteration count lives in your state struct — not in the graph executor. The Router reads it and routes to `END` when the limit is reached.

```go
type AgentState struct {
    Messages   []llm.Message
    ToolCalls  []llm.ToolCall
    Iteration  int
    Done       bool
}
```

The Router increments `Iteration` and terminates at the limit:

```go
graph.ConditionalRouter[AgentState](
    func(s core.State[AgentState]) string {
        if s.Data.Done || s.Data.Iteration >= 10 {
            return "end"
        }
        return "tool"
    },
    map[string]string{"tool": "tool", "end": graph.END},
)
```

The LLM node increments `Iteration` on each pass:

```go
llmNode := core.TypedFunc(func(ctx context.Context, s core.State[AgentState]) (core.State[AgentState], error) {
    d := s.Data
    d.Iteration++
    // ... call LLM, append response to d.Messages ...
    return core.NewState(d), nil
})
```

One iteration = one full pass through the cycle (LLM → tool → LLM counts as iteration 2 after the second LLM call). Nodes before the cycle starts do not count.

---

## MaxIterations middleware

For a reusable guard that terminates any agent loop without modifying state, use `WithMaxIterations`:

```go
import "github.com/stlimtat/bunshin-go/pkg/middleware"

agent := middleware.Chain(g.AsRunnable(),
    middleware.WithMaxIterations[AgentState](10, func(s core.State[AgentState]) int {
        return s.Data.Iteration
    }),
    middleware.WithLogging(logger),
)
```

`WithMaxIterations` wraps the graph's `Invoke` and returns an error if the iteration counter in state exceeds the limit before execution completes. The accessor function extracts the count from your specific state type.

> **Note**: `WithMaxIterations` is planned middleware — not yet shipped. Track at `pkg/middleware`.

---

## Token budget across providers

Token counting is orthogonal to iteration counting. Use `WithTokenBudget` middleware to aggregate tokens across all LLM calls regardless of provider:

```go
agent := middleware.Chain(g.AsRunnable(),
    middleware.WithMaxIterations[AgentState](10, iterationAccessor),
    middleware.WithTokenBudget(100_000), // total tokens across openai + anthropic
    middleware.WithLogging(logger),
)
```

`WithTokenBudget` intercepts each `Response.Usage.TotalTokens` via context propagation and returns a budget-exceeded error when the running total crosses the limit. Because bunshin normalises all providers to `TokenUsage`, OpenAI and Anthropic tokens aggregate in the same counter.

> **Note**: `WithTokenBudget` is planned middleware — not yet shipped. Track at `pkg/middleware`.

---

## Per-node vs aggregate token counting

| Concern | Where to track | How |
|---------|---------------|-----|
| Loop termination | `State[S].Data.Iteration` | Router reads and routes to END |
| Total token spend | Context via middleware | `WithTokenBudget` accumulates across all nodes |
| Per-node token debug | `State[S].Meta["bunshin.token_usage"]` | Each LLM node writes its own usage |

A two-node loop `nodeA (OpenAI) → nodeB (Sonnet)` counts as 2 LLM calls per iteration. `WithTokenBudget(50_000)` caps the combined spend; `WithMaxIterations(10, ...)` caps the loop count. Both can run together.

---

## Full example: bounded agent loop

```go
type AgentState struct {
    Question  string
    Messages  []llm.Message
    ToolCalls []llm.ToolCall
    Answer    string
    Iteration int
    Done      bool
}

g := graph.New[AgentState]("bounded-agent").
    AddNode(graph.Node[AgentState]{
        ID:       "llm",
        Runnable: llmNode,
        Router: graph.ConditionalRouter[AgentState](
            func(s core.State[AgentState]) string {
                d := s.Data
                if d.Done || d.Iteration >= 10 {
                    return "end"
                }
                if len(d.ToolCalls) > 0 {
                    return "tools"
                }
                return "end"
            },
            map[string]string{"tools": "tools", "end": graph.END},
        ),
    }).
    AddNode(graph.Node[AgentState]{
        ID:       "tools",
        Runnable: toolExecutorNode,
        Router:   graph.StaticRouter[AgentState]("llm"),
    }).
    SetEntry("llm")

agent := middleware.Chain(g.AsRunnable(),
    middleware.WithLogging(logger),
    middleware.WithPanicRecovery(),
)

result, err := agent.Invoke(ctx, core.NewState(AgentState{Question: "What is 2+2?"}))
```
