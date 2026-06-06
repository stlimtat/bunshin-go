+++
title = 'Multi-Agent Systems'
date = '2026-06-05'
draft = false
weight = 3
toc = true
+++

# Multi-Agent Systems

A multi-agent system in bunshin-go is a `Graph[OrchestratorState]` where some nodes are themselves complete `Graph[SubagentState]` instances. The orchestrator decides which subagent to invoke; each subagent runs its own internal workflow.

This mirrors LangGraph's multi-agent patterns (supervisor + workers, hierarchical agents) but with Go's static type safety at every boundary.

---

## The type boundary problem

`Graph[S]` fixes the state type for all its nodes. An orchestrator `Graph[OrchestratorState]` cannot directly call a subagent `Graph[ResearchState]` — they have different `S`. Naively wrapping the subgraph with `AsRunnable()` loses type safety and requires manual assertion.

The solution is `SubagentNode`:

```go
// SubagentNode runs Graph[Inner] as a typed node inside Graph[Outer].
// extract maps outer state to the subagent's input state.
// merge writes the subagent's output back into the outer state.
func SubagentNode[Outer, Inner any](
    g *graph.Graph[Inner],
    extract func(core.State[Outer]) core.State[Inner],
    merge   func(core.State[Outer], core.State[Inner]) core.State[Outer],
) core.TypedRunnable[core.State[Outer], core.State[Outer]]
```

> **Note**: `SubagentNode` is planned — not yet shipped. Track at `pkg/graph`.

---

## Example: orchestrator with two specialist subagents

```go
// Orchestrator state
type OrchestratorState struct {
    Task       string
    Subtask    string
    ResearchResult string
    CodeResult     string
    FinalAnswer    string
    Done           bool
}

// Subagent states
type ResearchState struct {
    Query  string
    Result string
}
type CodeState struct {
    Spec   string
    Output string
}

// Build subagents
researchAgent := buildResearchAgent() // *graph.Graph[ResearchState]
codeAgent     := buildCodeAgent()     // *graph.Graph[CodeState]

// Build orchestrator
orchestrator := graph.New[OrchestratorState]("orchestrator").
    AddNode(graph.Node[OrchestratorState]{
        ID:       "plan",
        Runnable: plannerNode,
        Router: graph.ConditionalRouter[OrchestratorState](
            func(s core.State[OrchestratorState]) string {
                switch s.Data.Subtask {
                case "research": return "research"
                case "code":     return "code"
                default:         return graph.END
                }
            },
            map[string]string{"research": "research", "code": "code"},
        ),
    }).
    AddNode(graph.Node[OrchestratorState]{
        ID: "research",
        Runnable: graph.SubagentNode(researchAgent,
            func(s core.State[OrchestratorState]) core.State[ResearchState] {
                return core.NewState(ResearchState{Query: s.Data.Task})
            },
            func(outer core.State[OrchestratorState], inner core.State[ResearchState]) core.State[OrchestratorState] {
                d := outer.Data
                d.ResearchResult = inner.Data.Result
                d.Done = true
                return core.NewState(d)
            },
        ),
        Router: graph.StaticRouter[OrchestratorState](graph.END),
    }).
    AddNode(graph.Node[OrchestratorState]{
        ID: "code",
        Runnable: graph.SubagentNode(codeAgent,
            func(s core.State[OrchestratorState]) core.State[CodeState] {
                return core.NewState(CodeState{Spec: s.Data.Task})
            },
            func(outer core.State[OrchestratorState], inner core.State[CodeState]) core.State[OrchestratorState] {
                d := outer.Data
                d.CodeResult = inner.Data.Output
                d.Done = true
                return core.NewState(d)
            },
        ),
        Router: graph.StaticRouter[OrchestratorState](graph.END),
    }).
    SetEntry("plan")
```

---

## Patterns

### Supervisor + workers

The orchestrator is a Router-only node: it reads the task, picks a worker subagent, delegates, collects the result, and routes to END or loops.

### Sequential subagents

Route `research → code → review` where each subagent's output feeds the next via the `merge` function.

### Parallel subagents (planned)

A `ParallelNode[Outer]` will run multiple `SubagentNode`s concurrently and merge all results before routing. Uses `golang.org/x/sync/errgroup` internally.

---

## State Meta flows through boundaries

`SubagentNode` automatically propagates `State.Meta` across the boundary:
- `extract` copies `Meta` from outer to inner state
- `merge` copies updated `Meta` back from inner to outer

Trace IDs, cost budgets, and session tokens flow transparently through the subagent without the subagent needing to know about them.

---

## Nesting depth

There is no enforced nesting limit. An orchestrator can call subagents that themselves contain graphs. Each level is typed independently. In practice, 2–3 levels covers most workflows; deeper nesting makes debugging harder.
