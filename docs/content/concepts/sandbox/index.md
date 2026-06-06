+++
title = 'Sandbox: Code Execution'
date = '2026-06-05'
draft = false
weight = 4
toc = true
+++

# Sandbox: Code Execution

`pkg/sandbox` provides sandboxed execution of untrusted code. LLM agents call it through `CodeExecTool` in `pkg/tools` — a thin adapter that bridges the `Tool` interface to the `Sandbox` interface.

Three backends ship: **E2B** (cloud-hosted), **Docker** (local container), **WASM** (in-process). Backends are registered in a `SandboxRegistry` and selected at call time by tags — the same pattern as `ProviderRegistry` for LLM providers.

---

## SandboxRegistry — backend and resource selection

Register backends with tags that describe both selection criteria and resource limits:

```go
sandboxRegistry.Register("e2b-python-high",
    e2b.New(e2b.WithAPIKey(os.Getenv("E2B_API_KEY"))),
    sandbox.Tags{
        "language":        "python",
        "env":             "e2b",
        "memory_mb":       "4096",
        "cpu_millicores":  "2000",
        "tenant_tier":     "high",
    },
)

sandboxRegistry.Register("docker-python-low",
    docker.New(),
    sandbox.Tags{
        "language":       "python",
        "env":            "docker",
        "memory_mb":      "512",
        "cpu_millicores": "500",
        "tenant_tier":    "low",
    },
)
```

Tags serve dual purpose:

1. **Selection** — `CodeExecTool` resolves the right backend at call time using `{"language": "python", "tenant_tier": "high"}`.
2. **Configuration** — each backend reads its own resource tags at registration and applies them as container/runtime limits.

Resource limits are immutable per registry entry. A caller cannot request more resources than their registered entry allows — the selection itself enforces the budget.

### Per-tenant sandbox tiers

In multi-tenant deployments, register sandbox entries per tier and tag with `tenant_tier`. Auth middleware writes the tenant's tier into `Principal`; `CodeExecTool` reads it from context when resolving the backend:

```go
// High-tier tenant: E2B cloud, 4GB RAM
sandboxRegistry.Register("e2b-python-high", e2bBackend, sandbox.Tags{
    "language": "python", "tenant_tier": "high", "memory_mb": "4096",
})

// Standard tenant: Docker local, 512MB RAM
sandboxRegistry.Register("docker-python-std", dockerBackend, sandbox.Tags{
    "language": "python", "tenant_tier": "standard", "memory_mb": "512",
})
```

---

## Sandbox interface

```go
type Sandbox interface {
    // Session opens a persistent execution environment.
    // Multiple Run calls share filesystem and installed packages.
    Session(ctx context.Context) (Session, error)

    // Run is a convenience method: opens a session, executes once, closes.
    Run(ctx context.Context, req RunRequest) (RunResult, error)
}

type Session interface {
    Run(ctx context.Context, req RunRequest) (RunResult, error)
    Close() error
}
```

### Ephemeral execution (single tool call)

```go
result, err := sandbox.Run(ctx, sandbox.RunRequest{
    Language: "python",
    Code:     "print('hello')",
    Timeout:  10 * time.Second,
})
fmt.Println(result.Stdout) // "hello\n"
```

### Persistent sessions (agent loops)

Use `Session` when multiple tool calls must share state — installed packages, written files, environment variables:

```go
session, err := sandbox.Session(ctx)
if err != nil { ... }
defer session.Close()

// Step 1: install dependency
session.Run(ctx, sandbox.RunRequest{
    Language: "python",
    Code:     "import subprocess; subprocess.run(['pip', 'install', 'pandas'])",
    Timeout:  60 * time.Second,
})

// Step 2: run analysis using the installed package
result, err := session.Run(ctx, sandbox.RunRequest{
    Language: "python",
    Code:     "import pandas as pd; df = pd.read_csv('data.csv'); print(df.describe())",
    Timeout:  30 * time.Second,
})
```

Sessions are stored in `State.Meta["bunshin.sandbox_session"]` by `CodeExecTool` so they persist across tool calls within one graph invocation. A middleware finaliser calls `Close()` when the invocation ends.

---

## RunRequest

```go
type RunRequest struct {
    Language string                 // "python", "go", "javascript", "bash"
    Code     string                 // source to execute
    Timeout  time.Duration          // hard kill after this duration
    Stdin    string                 // optional stdin
    Files    map[string][]byte      // files to inject into the sandbox filesystem
    EnvVars  map[string]string      // environment variables
}
```

---

## RunResult and large file outputs

```go
type RunResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
    Duration time.Duration
    Files    map[string]*llm.MediaRef  // output files — inline reader or remote URL
}
```

`Files` uses `llm.MediaRef` — the same type used for multimodal message content — so sandbox outputs slot directly into a `ContentPart` for the next LLM call.

### Small files (below threshold)

Files below `inline_max_bytes` (default 10 MB, configurable per registry entry via tag) are returned as inline streaming readers:

```go
ref := result.Files["report.csv"]
// ref.Data is an io.Reader — read once, then closed
data, _ := io.ReadAll(ref.Data)
```

### Large files (above threshold)

For large outputs — transcoded video, bulk exports, rendered images — the sandbox backend uploads directly to the configured blob store (MinIO or S3) during execution and returns a remote reference:

```go
ref := result.Files["output.mp4"]
// ref.URL  = "s3://bunshin-sandbox/runs/abc123/output.mp4"
// ref.Size = 429_496_729_600
// ref.MimeType = "video/mp4"
// ref.Data is nil — content is remote
```

The agent passes `ref` directly into a `ContentPart` or hands it to a downstream tool (e.g., a streaming upload tool). The file is never materialised locally.

Configure the threshold per registry entry:

```go
sandboxRegistry.Register("docker-media", dockerBackend, sandbox.Tags{
    "language":         "bash",
    "env":              "docker",
    "inline_max_bytes": "10485760", // 10 MB
    "memory_mb":        "8192",
})
```

---

## Backends

| Backend | Isolation | Requires | Best for |
|---------|-----------|----------|----------|
| E2B | Cloud VM | E2B API key | Trusted users, long sessions, pre-installed packages |
| Docker | Container | Docker daemon | Self-hosted, controlled images, local dev |
| WASM | In-process | None | Untrusted scripts, zero external deps, edge deployments |

### E2B

```go
e2b.New(e2b.WithAPIKey(os.Getenv("E2B_API_KEY")))
```

Supports persistent sessions natively. Session maps 1:1 to an E2B sandbox instance. Billing is per execution-second.

### Docker

```go
docker.New(docker.WithImage("python:3.12-slim"))
```

Each `Session` maps to a container. Resource limits (`memory_mb`, `cpu_millicores`) are applied as `docker run --memory --cpus` flags. Requires a Docker daemon accessible from the bunshin process.

### WASM

```go
wasm.New(wasm.WithRuntime(wasm.Wasmtime))
```

Runs code in-process via a WASM runtime. No external dependencies. Network access is disabled by default; filesystem access is scoped to a WASI pre-open directory. Best for sandboxing untrusted user scripts where Docker is unavailable.

---

## CodeExecTool

`CodeExecTool` in `pkg/tools` is the LLM-callable face of `pkg/sandbox`. It is a thin adapter — ~20 lines — that resolves a backend from `SandboxRegistry` using the caller's tags, manages session lifecycle in `State.Meta`, and translates `RunResult.Files` into `ContentPart` values for the agent's next message.

```go
tool := tools.NewCodeExecTool(sandboxRegistry,
    tools.WithSandboxTag("env", "docker"),
)
toolRegistry.Register(tool)
```

The LLM calls `code_exec` with `{"language": "python", "code": "..."}`. The tool handles backend resolution, session reuse, and result formatting transparently.
