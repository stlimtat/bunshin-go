+++
title = 'Examples'
date = '2026-06-03'
draft = false
toc = true
weight = 3
+++

# Examples

Working examples showing how to use bunshin-go features. Each example is self-contained and runnable from the CLI.

---

## CLI examples

The `bunshin` CLI ships several built-in demo commands.

### Single LLM call

Call any provider directly:

```bash
# OpenAI
OPENAI_API_KEY=sk-... bunshin llm --provider openai --message "Explain goroutines"

# Anthropic
ANTHROPIC_API_KEY=sk-ant-... bunshin llm --provider anthropic --message "Explain goroutines"

# Google Gemini
GOOGLE_API_KEY=... bunshin llm --provider google --message "Explain goroutines"
```

### Two-step chain

```bash
bunshin chain --question "What is the capital of France?"
```

This runs: fast model extracts a concise answer → smart model explains the reasoning.

### Agent with tool use

```bash
bunshin agent --question "What is 42 * 37?"
```

The agent loop classifies the question, calls the `calc` tool, then formats the result.

### MCP + Sandbox demo

```bash
bunshin mcp-sandbox
```

Demonstrates: MCP tool discovery → sandboxed Python execution → result returned to LLM.

---

## Templates

Prompt templates decouple content from code. Fragments are individually versioned, testable, and reusable across multiple templates.

### Define fragments

```go
import "github.com/stlimtat/bunshin-go/pkg/prompt"

backend := prompt.NewMemoryBackend()

backend.Store(prompt.Fragment{
    ID:        "system-persona",
    Content:   "You are {{.Name}}, a helpful assistant specialised in {{.Domain}}.",
    Variables: []string{"Name", "Domain"},
    Tags:      []string{"system"},
})

backend.Store(prompt.Fragment{
    ID:        "task-summarise",
    Content:   "Summarise the following text in {{.MaxSentences}} sentences:\n\n{{.Text}}",
    Variables: []string{"MaxSentences", "Text"},
    Tags:      []string{"task"},
})

backend.Store(prompt.Fragment{
    ID:        "format-json",
    Content:   "Return your response as valid JSON matching this schema: {{.Schema}}",
    Variables: []string{"Schema"},
    Tags:      []string{"format"},
})
```

### Compose a template

```go
t := prompt.PromptTemplate{
    Fragments: []prompt.FragmentRef{
        {ID: "system-persona"},
        {ID: "task-summarise"},
        {
            ID:        "format-json",
            Condition: "{{.WantJSON}}",  // Only include when WantJSON is truthy
        },
    },
    Separator: "\n\n",
}

composer := prompt.NewPromptComposer(backend)
rendered, err := composer.Render(ctx, t, map[string]any{
    "Name":         "Aria",
    "Domain":       "Go programming",
    "MaxSentences": 3,
    "Text":         "Go is a statically typed, compiled language...",
    "WantJSON":     true,
    "Schema":       `{"summary": "string", "keyPoints": ["string"]}`,
})
```

### Use rendered prompt with an LLM

```go
provider := llm.NewOpenAIProvider(llm.OpenAIConfig{APIKey: key, Model: "gpt-4o-mini"})

resp, err := provider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role:  llm.RoleUser,
        Parts: []llm.ContentPart{{Text: rendered}},
    }},
})
fmt.Println(resp.Content)
```

### Load fragments from embedded files

```go
import "embed"

//go:embed prompts/*.tmpl
var promptFS embed.FS

backend := prompt.NewEmbedBackend(promptFS, "prompts")
// Loads all .tmpl files from prompts/ at binary build time
```

### Versioned fragments (A/B testing)

```go
backend.StoreVersion(prompt.Fragment{
    ID:      "task-summarise",
    Version: "v2",
    Content: "Provide a {{.MaxSentences}}-sentence summary. Be direct and factual.\n\n{{.Text}}",
    Variables: []string{"MaxSentences", "Text"},
})

// Pin a template to a specific version
t := prompt.PromptTemplate{
    Fragments: []prompt.FragmentRef{
        {ID: "task-summarise", Overrides: map[string]any{"_version": "v2"}},
    },
}
```

---

## MCP (Model Context Protocol)

MCP lets your bunshin-go application discover and call tools on any MCP-compatible server, and optionally expose its own tools as an MCP server.

### Consume an MCP server

```go
import "github.com/stlimtat/bunshin-go/pkg/mcp"

// Connect to any MCP server (local process or remote HTTP)
client := mcp.NewFakeClient()  // Replace with mcp.NewStdioClient() or mcp.NewHTTPClient()
if err := client.Connect(ctx, "http://localhost:3000"); err != nil {
    panic(err)
}
defer client.Close()

// Discover available tools
mcpTools, err := client.Tools(ctx)
fmt.Printf("Found %d tools\n", len(mcpTools))

// Each MCP tool is a first-class bunshin-go Tool — use it in a chain or agent
for _, t := range mcpTools {
    fmt.Printf("  - %s: %s\n", t.Schema().Name, t.Schema().Description)
}

// Call a specific tool directly
result, err := client.CallTool(ctx, "get_weather", map[string]any{
    "location": "Singapore",
    "unit":     "celsius",
})
fmt.Println(result)
```

### Read MCP resources

```go
// List available resources (documentation, databases, files)
resources, err := client.Resources(ctx)
for _, r := range resources {
    fmt.Printf("  - %s (%s)\n", r.Name, r.URI)
}

// Read a specific resource
content, err := client.ReadResource(ctx, "file:///workspace/README.md")
fmt.Println(content.Text)
```

### Use MCP prompts

```go
// List prompt templates the server exposes
prompts, err := client.Prompts(ctx)
for _, p := range prompts {
    fmt.Printf("  - %s: %s\n", p.Name, p.Description)
}

// Render a prompt with arguments
messages, err := client.GetPrompt(ctx, "code-review", map[string]string{
    "language": "go",
    "code":     "func add(a, b int) int { return a + b }",
})
// messages is []PromptMessage ready to pass to any LLM provider
```

### Wire MCP tools into an agent graph

```go
import (
    "github.com/stlimtat/bunshin-go/pkg/graph"
    "github.com/stlimtat/bunshin-go/pkg/tools"
)

// Register discovered MCP tools in a ToolRegistry
registry := tools.NewToolRegistry()
for _, t := range mcpTools {
    registry.Register(t)
}

// Build an agent graph that can call any registered tool
agentNode := graph.Node{
    ID: "agent",
    Runnable: core.NewRunnableFunc("agent", func(ctx context.Context, input any) (any, error) {
        // LLM decides which tool to call based on input
        schema := registry.List()
        resp, err := provider.Complete(ctx, &llm.Request{
            Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.ContentPart{{Text: fmt.Sprint(input)}}}},
            Tools:    schema,
        })
        return resp, err
    }),
}
```

---

## Sandbox (Secure Code Execution)

The sandbox interface provides isolated code execution. Use it when your LLM or tool needs to run untrusted code.

### Run Python code

```go
import "github.com/stlimtat/bunshin-go/pkg/sandbox"

// In tests, use the fake backend (deterministic, no I/O)
sb := sandbox.NewFakeSandbox()

// In production, use Docker or E2B backends:
// sb := sandbox.NewDockerSandbox(sandbox.DockerConfig{Image: "python:3.12-slim"})
// sb := sandbox.NewE2BSandbox(sandbox.E2BConfig{APIKey: e2bKey})

result, err := sb.Exec(ctx, &sandbox.ExecRequest{
    Language: "python",
    Code: `
import json
data = [1, 2, 3, 4, 5]
print(json.dumps({"sum": sum(data), "mean": sum(data)/len(data)}))
`,
    Timeout: 10 * time.Second,
})
if err != nil {
    panic(err)
}
fmt.Println(result.Stdout) // {"sum": 15, "mean": 3.0}
```

### Pass files into the sandbox

```go
result, err := sb.Exec(ctx, &sandbox.ExecRequest{
    Language: "python",
    Code: `
import json
with open("data.json") as f:
    data = json.load(f)
print(f"Processed {len(data['items'])} items")
`,
    Files: map[string][]byte{
        "data.json": []byte(`{"items": [1, 2, 3]}`),
    },
    Timeout: 30 * time.Second,
})
// result.Files contains any files the code wrote to disk
```

### Warm sessions for multi-step execution

```go
// Start a session
step1, err := sb.Exec(ctx, &sandbox.ExecRequest{
    Language:  "python",
    Code:      "x = 42",
    SessionID: "my-session",
})

// Reuse the same interpreter — x is still in scope
step2, err := sb.Exec(ctx, &sandbox.ExecRequest{
    Language:  "python",
    Code:      "print(x * 2)",
    SessionID: step1.SessionID,
})
fmt.Println(step2.Stdout) // 84

// Clean up when done
sb.Kill(ctx, step1.SessionID)
```

### Code execution as a Runnable tool

Expose the sandbox as a `Tool` so an LLM agent can call it:

```go
import "github.com/stlimtat/bunshin-go/pkg/tools"

codeExecTool := tools.NewFuncTool(
    tools.ToolSchema{
        Name:        "exec_python",
        Description: "Execute Python code and return stdout. Use for calculations, data processing, or any code the user requests.",
        Parameters:  codeSchema, // JSON schema: {code: string}
    },
    func(ctx context.Context, input any) (any, error) {
        params := input.(map[string]any)
        result, err := sb.Exec(ctx, &sandbox.ExecRequest{
            Language: "python",
            Code:     params["code"].(string),
            Timeout:  30 * time.Second,
        })
        if err != nil {
            return nil, err
        }
        return map[string]any{
            "stdout": result.Stdout,
            "stderr": result.Stderr,
            "exit_code": result.ExitCode,
        }, nil
    },
)

registry := tools.NewToolRegistry()
registry.Register(codeExecTool)
```

### MCP + Sandbox: end-to-end

Connect an MCP client, get tool schemas, then let the LLM decide when to run sandboxed code:

```go
// 1. MCP client discovers available tools
mcpTools, _ := client.Tools(ctx)

// 2. Build a tool registry with both MCP tools and the sandbox tool
registry := tools.NewToolRegistry()
for _, t := range mcpTools {
    registry.Register(t)
}
registry.Register(codeExecTool)

// 3. Agent loop: LLM selects tools, sandbox executes code, results feed back
g := graph.New("mcp-sandbox-agent")
g.AddNode(graph.Node{
    ID: "llm",
    Runnable: core.NewRunnableFunc("llm", func(ctx context.Context, input any) (any, error) {
        return provider.Complete(ctx, &llm.Request{
            Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.ContentPart{{Text: fmt.Sprint(input)}}}},
            Tools:    registry.List(),
        })
    }),
    Router: func(ctx context.Context, out any) (string, error) {
        resp := out.(*llm.Response)
        if len(resp.ToolCalls) > 0 {
            return "tool", nil
        }
        return graph.END, nil
    },
})
g.AddNode(graph.Node{
    ID: "tool",
    Runnable: core.NewRunnableFunc("tool", func(ctx context.Context, input any) (any, error) {
        resp := input.(*llm.Response)
        call := resp.ToolCalls[0]
        return registry.Get(call.Name).Invoke(ctx, call.Arguments)
    }),
    Router: func(_ context.Context, _ any) (string, error) {
        return "llm", nil // Feed result back to LLM
    },
})
g.SetEntry("llm")
```
