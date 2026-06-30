+++
title = 'Quickstart'
date = '2026-06-03'
draft = false
toc = false
weight = 1
+++

# Quickstart

Make a real LLM call in under five minutes.

## Prerequisites

- Go 1.25+
- OpenAI API key — [platform.openai.com](https://platform.openai.com/api-keys)

## Clone

```bash
git clone https://github.com/stlimtat/bunshin-go
cd bunshin-go
```

## The code

`examples/llm/main.go`:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/stlimtat/bunshin-go/pkg/llm"
)

func main() {
    key := os.Getenv("OPENAI_API_KEY")
    if key == "" {
        fmt.Fprintln(os.Stderr, "OPENAI_API_KEY not set")
        os.Exit(1)
    }

    provider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey:    key,
        Model:     "gpt-4o-mini",
        MaxTokens: 256,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "provider: %v\n", err)
        os.Exit(1)
    }

    req := &llm.Request{
        Messages: []llm.Message{
            llm.NewTextMessage(llm.RoleUser, "Say hello in three words."),
        },
    }

    resp, err := provider.Complete(context.Background(), req)
    if err != nil {
        fmt.Fprintf(os.Stderr, "invoke: %v\n", err)
        os.Exit(1)
    }

    fmt.Println("Response:", resp.Content)
}
```

## Run

```bash
OPENAI_API_KEY=sk-... go run ./examples/llm
```

```
Response: Hello, world! 🌍
```

## Next steps

- [Chain two LLMs](/howto/chaining-llms/) — mix providers per workflow node
- [Agent loop](/howto/agent-loops/) — iteration counts and token budgets
- [All examples](/examples/) — MCP tools, sandbox, prompt templates
