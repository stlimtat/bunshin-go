+++
title = 'Core Concepts'
date = '2026-06-03'
draft = false
weight = 2
+++

# Core Concepts

bunshin-go is built around four primitive abstractions. Everything else composes from them.

---

## Runnable

`Runnable` is the fundamental unit of work. Every chain step, LLM call, tool, and prompt renderer implements this interface.

```go
type Runnable interface {
    Name() string
    Invoke(ctx context.Context, input any) (any, error)
    Stream(ctx context.Context, input any) (<-chan StreamChunk, error)
}
```

The **two-layer design** separates type safety from composability:

| Layer | Interface | Purpose |
|-------|-----------|---------|
| Inner | `TypedRunnable[In, Out]` | Type-safe at definition time |
| Outer | `Runnable` | Untyped shell for chains and middleware |

Use `AsRunnable` to bridge:

```go
type MyStep struct{}
func (s *MyStep) Invoke(ctx context.Context, input string) (string, error) {
    return strings.ToUpper(input), nil
}

r := core.AsRunnable("upper", &MyStep{})  // Runnable wrapping TypedRunnable
```

Create a one-off runnable without a struct:

```go
r := core.NewRunnableFunc("echo", func(_ context.Context, input any) (any, error) {
    return input, nil
})
```

---

## State

`State[S]` is the typed envelope that flows between runnable steps in a workflow. It carries both your domain struct and a free-form metadata bag.

```go
type State[S any] struct {
    Data S
    Meta map[string]any
}
```

Use `State` when steps need to pass structured data alongside trace metadata:

```go
type DocState struct {
    RawText   string
    Summary   string
    Embedding []float32
}

state := core.State[DocState]{
    Data: DocState{RawText: "..."},
    Meta: map[string]any{"doc_id": "123"},
}
```

---

## Chain

`Chain` executes steps sequentially. Output of step N becomes input of step N+1.

```go
pipeline := chain.New("my-chain",
    chain.Step{ID: "extract", Runnable: extractRunnable},
    chain.Step{ID: "reason",  Runnable: reasonRunnable},
    chain.Step{ID: "format",  Runnable: formatRunnable},
)

result, err := pipeline.Invoke(ctx, input)
```

`Chain` itself implements `Runnable`, so chains nest inside other chains or graph nodes.

`Stream` on a chain runs all but the last step via `Invoke`, then streams the final step.

---

## Graph

`Graph` executes a DAG of named nodes. Each node optionally has a `Router` that inspects the node's output and returns the ID of the next node to execute (or `graph.END` to terminate).

```go
g := graph.New("agent-loop")
g.AddNode(graph.Node{
    ID:      "classify",
    Runnable: classifyRunnable,
    Router: func(ctx context.Context, out any) (string, error) {
        if out.(string) == "tool" {
            return "tool_call", nil
        }
        return graph.END, nil
    },
})
g.AddNode(graph.Node{ID: "tool_call", Runnable: toolRunnable})
g.SetEntry("classify")

result, err := g.Invoke(ctx, input)
```

Graphs enable:
- **Agent loops** — LLM decides tool calls; graph routes back to LLM after each tool
- **Conditional branching** — route to different nodes based on content classification
- **Human-in-the-loop** — pause at a node waiting for approval before continuing

---

## LLM Providers

All model adapters implement `LLMProvider`:

```go
type LLMProvider interface {
    ID() ProviderID
    Complete(ctx context.Context, req *Request) (*Response, error)
    StreamComplete(ctx context.Context, req *Request) (<-chan Chunk, error)
    CanTransferContext(from LLMProvider) bool
    NativeMessages(msgs []Message) (any, error)
}
```

Built-in providers:

| Provider | ProviderID | Constructor |
|----------|------------|-------------|
| OpenAI | `openai` | `llm.NewOpenAIProvider` |
| Anthropic | `anthropic` | `llm.NewAnthropicProvider` |
| Google Gemini | `google` | `llm.NewGoogleProvider` |
| Azure OpenAI | `azure-openai` | `llm.NewAzureOpenAIProvider` |

**FallbackProvider** tries providers in order and returns the first success:

```go
fallback := llm.NewFallbackProvider(primary, secondary, tertiary)
```

**ProviderRegistry** manages multiple providers, background-pings them every 30 seconds, and exposes only healthy ones via `Available()`:

```go
registry := llm.NewProviderRegistry(openaiProvider, anthropicProvider)
registry.Start(ctx)
live := registry.Available()
```

---

## MessageStore

`MessageStore` holds conversation history with a sliding window. It stores references and cursors, not raw bytes — safe for 2M-token contexts.

```go
type MessageStore interface {
    Append(ctx context.Context, msg Message) error
    Window(ctx context.Context, maxTokens int) ([]Message, error)
    WindowFor(ctx context.Context, p LLMProvider, maxTokens int) (*Request, error)
}
```

`WindowFor` uses same-provider fast-path when consecutive steps share a provider, bypassing canonical translation overhead.

---

## Middleware

Middleware wraps any `Runnable`:

```go
type Middleware func(Runnable) Runnable

func Chain(r Runnable, mw ...Middleware) Runnable
```

Five interception levels match the workflow stack:

| Level | Where | Built-in middleware |
|-------|-------|---------------------|
| L1 Workflow | Outer boundary | auth, rate limit, budget |
| L2 Runnable | Each step | logging, retry, timing |
| L3 Prompt | Before LLM | PII scrub, injection guard |
| L4 LLM call | Around provider | cache, model fallback |
| L5 Tool | Tool execution | authz, sandbox, validation |

---

## Prompt Templates

Prompts are composed from **Fragments** — individually versioned, tag-queryable content blocks:

```go
frag := prompt.Fragment{
    ID:        "system-intro",
    Content:   "You are {{.Role}}, a helpful assistant specialised in {{.Domain}}.",
    Variables: []string{"Role", "Domain"},
}
```

Templates assemble fragments in order:

```go
t := prompt.PromptTemplate{
    Fragments: []prompt.FragmentRef{
        {ID: "system-intro"},
        {ID: "task-description", Condition: "{{.IncludeTask}}"},
    },
    Separator: "\n\n",
}
rendered, err := composer.Render(ctx, t, map[string]any{
    "Role": "Aria", "Domain": "Go programming", "IncludeTask": true,
})
```

Backends: `EmbedBackend` (go:embed), `FileBackend`, `MemoryBackend`.

---

## Checkpoint and Recovery

Long-running workflows can checkpoint state after each step and resume from the last checkpoint on failure:

```go
type RecoveryConfig struct {
    Strategy            RecoveryStrategy  // "resume" or "replay"
    CheckpointFrequency CheckpointFreq    // AfterEachStep | AfterEachN | OnCompletion
    MaxRetries          int
}
```

`ThreadID` is the coordination key — any node loading the same `ThreadID` resumes correctly, enabling horizontal scaling.
