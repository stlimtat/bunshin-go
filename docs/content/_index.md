+++
title = 'bunshin-go'
date = '2026-06-06'
draft = false
+++

# bunshin-go

A production-grade Go implementation of the LangChain / LangGraph / LangSmith stack.

- **Type-safe** — `TypedRunnable[In, Out]` catches schema drift at compile time, not runtime
- **Concurrent** — native goroutines; no GIL, no thread pools
- **Single-binary** — one `bunshin` binary handles serve, eval, embed, and ops
- **Observable** — LangSmith traces per LLM call + OTEL spans for infrastructure

---

## Quickstart

```bash
go get github.com/stlimtat/bunshin-go

export OPENAI_API_KEY=sk-...
bunshin workflow run --workflow echo --input '{"message":"hello"}'
```

→ [Full quickstart guide](/quickstart/)

---

## Sections

| | |
|---|---|
| [Quickstart](/quickstart/) | Get a working LLM call in five minutes |
| [Core Concepts](/concepts/) | Runnable, Chain, Graph, State, ProviderRegistry |
| [How-to guides](/howto/) | Chaining LLMs, agent loops, multimodal, scheduling |
| [Examples](/examples/) | CLI demos, library snippets, MCP + sandbox |
| [Architecture](/architecture/) | Package layout, key design decisions |
