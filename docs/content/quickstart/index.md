+++
title = 'Quickstart'
date = '2026-06-03'
draft = false
toc = true
weight = 1
+++

# Quickstart

Get a working LLM call in under five minutes.

## Prerequisites

- Go 1.23+
- An API key from one of: OpenAI, Anthropic, Google AI, or Azure OpenAI

## Installation

```bash
go get github.com/stlimtat/bunshin-go
```

Or clone and build the CLI:

```bash
git clone https://github.com/stlimtat/bunshin-go
cd bunshin-go
go build ./cmd/bunshin
```

## Your first LLM call

Set your API key:

```bash
export OPENAI_API_KEY=sk-...
```

Call the LLM from the CLI:

```bash
bunshin llm --provider openai --message "What is Go?"
```

Or use the library directly:

```go
package main

import (
    "context"
    "fmt"

    "github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
    provider := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey: "sk-...",
        Model:  "gpt-4o-mini",
    })

    resp, err := provider.Complete(context.Background(), &llm.Request{
        Messages: []llm.Message{
            {Role: llm.RoleUser, Parts: []llm.ContentPart{{Text: "What is Go?"}}},
        },
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(resp.Content)
}
```

## Run the HTTP server

```bash
bunshin serve --addr :8080
```

Then call a workflow:

```bash
curl -X POST http://localhost:8080/v1/workflows/echo/invoke \
  -H "Content-Type: application/json" \
  -d '"hello world"'
```

Or stream tokens via SSE:

```bash
curl http://localhost:8080/v1/workflows/echo/stream
```

## Two-step chain

```go
import (
    "github.com/stlimtat/bunshin-go/pkg/chain"
    "github.com/stlimtat/bunshin-go/pkg/core"
    "github.com/stlimtat/bunshin-go/pkg/llm"
)

fast := llm.NewOpenAIProvider(llm.OpenAIConfig{Model: "gpt-4o-mini", APIKey: key})
smart := llm.NewOpenAIProvider(llm.OpenAIConfig{Model: "gpt-4o", APIKey: key})

extractStep := core.NewRunnableFunc("extract", func(ctx context.Context, input any) (any, error) {
    return fast.Complete(ctx, &llm.Request{Messages: userMsg(input)})
})

reasonStep := core.NewRunnableFunc("reason", func(ctx context.Context, input any) (any, error) {
    return smart.Complete(ctx, &llm.Request{Messages: userMsg(input)})
})

pipeline := chain.New("two-step",
    chain.Step{ID: "extract", Runnable: extractStep},
    chain.Step{ID: "reason", Runnable: reasonStep},
)

result, err := pipeline.Invoke(ctx, "Summarise this document: ...")
```

## Next steps

- [Core Concepts](/concepts/) — understand Runnable, Chain, Graph, and State
- [How-to: Chaining LLMs](/howto/chaining-llms/) — mix providers per workflow node
- [Examples](/examples/) — templates, MCP tools, and sandboxed code execution
