# Declarative Agents and Skills: systematic subagents over Fragments

bunshin gains two new first-class, declarative concepts layered on the existing prompt system: **Agent** (a subagent with its own system prompt, tools, model, and isolated loop) and **Skill** (a named capability injected into a prompt on demand). Both are *envelopes that reference a `Fragment`* rather than new kinds of prompt storage — "a prompt used as an agent or skill." This replaces hand-written `SubagentNode` with a systematic, spec-driven mechanism.

## Why two distinct types, not one

An Agent and a Skill have genuinely different lifecycles:

- An **Agent** is *invoked* — it runs its own tool-calling loop in a fresh, isolated context and returns a result. Heavyweight: own model, own tools, own message history.
- A **Skill** is *composed* — it has no loop and no model. It contributes instructions + files into a *consuming* agent's (or composer's) context and borrows that consumer's sandbox to run scripts. Lightweight.

Collapsing them (one type with a mode flag, or "a skill is just a no-tool agent") muddies both. They share only their substrate — the body is a `Fragment` in `PromptBackend` — and diverge entirely at invocation. So: distinct `AgentSpec` and `SkillSpec`, each with its own store.

## AgentSpec mirrors WorkflowSpec

An Agent needs more than text (tools, model, iteration cap, output contract), so it cannot *be* a `Fragment` — it *references* one for its system prompt. The proven pattern in this repo is `WorkflowSpec` + `workflow.Store`, so `AgentSpec` copies it:

- YAML definition, `draft → active` lifecycle, content-hash version (`"sha256:" + hex16(canonical_yaml)`), idempotent `Create`.
- Tenant-scoped `agent.Store`; backends in `pkg/agent/store/{memory,postgres,blob,git}`.
- `/v1/agents/...` route table mirroring `/v1/workflows`; `bunshin agent` CLI group.
- Registries resolved at **compile time** — missing tool/agent/skill/prompt refs are compile errors, not runtime panics.

```yaml
name: investigator
description: Locates code and reports file:line references.
system_prompt: { slug: investigator-system }
model: { tier: smart, tags: { budget: high } }
tools: [grep, read_file]
agents: [summarizer]      # agent-as-tool delegation
skills: [code-search]     # injected capability
max_iterations: 8
output_schema: { ... }    # optional; forces structured final turn
```

## One compile, three invocation surfaces

`agent.Compile(spec, registries) → *CompiledAgent`. `CompiledAgent` is a `Graph[AgentState]` that implements **both** `core.Runnable` and `tools.Tool`. From that, three surfaces fall out:

1. **As a Tool** (primary) — the agent advertises a `ToolSchema` from its name/description/input contract. A parent Orchestrator lists other agents in its `agents:` allowlist; the parent LLM delegates dynamically via function-calling. *This is what makes subagents systematic* — no bespoke routing code.
2. **As a YAML graph node** — `WorkflowNode` gains `type: agent`; task comes from `input_key`, result lands at `output_key` (same convention as other nodes). The declarative replacement for `SubagentNode`.
3. **As a top-level Runnable** — `POST /v1/agents/{name}/invoke`, `bunshin agent run`.

The compiled graph shape is `llm → [content-based router] → tools → llm … → END`, reusing the existing EIP `Content-Based Router` (keyed on "did the last message contain tool calls"). No new graph primitive.

## Context isolation and I/O contract

An Agent runs on a **fresh** `AgentState` with its own message list. Input: a task string + optional structured args validated against `input_schema`. Output: the final assistant text, plus structured output if `output_schema` is declared. `Meta` (trace IDs, cost budget, tenant, depth) flows *in* so telemetry and billing stitch together; the agent's internal turns never flow *out*. This mirrors `SubagentNode`'s `InjectFn`/`ExtractFn`, but generated from the spec instead of hand-written.

`max_iterations` (default 8) exceeded → truncate-and-return with `Meta["bunshin.agent_truncated"] = true`, not a hard error — a parent can inspect and decide; a hard error discards completed work.

## Nested agents: eager topological compile + dual cycle guard

Because agents reference other agents (agent-as-tool), the reference graph can cycle (A → B → A). Resolution is **eager and topological**: the whole reachable agent graph compiles up front. Topological order (Kahn's algorithm) *is* the cycle detector — it fails iff a cycle exists — and it surfaces missing refs at compile time. A second **runtime** guard caps delegation depth via `Meta["bunshin.agent_depth"]` (default 8), returning an error to the calling LLM when exceeded.

Tool and agent names share one lookup namespace at compile time (`Tools` first, then `Agents`); because a `CompiledAgent` satisfies `tools.Tool`, the LLM node treats agents and tools uniformly with no special case.

## Skills: progressive disclosure, two triggers

A `SkillSpec` is `{name, description, body FragmentRef, files []FileRef, trigger}` with its own tenant-scoped `skill.Store` (same lifecycle/versioning as `agent.Store`; files version atomically). `trigger` decides the loading mechanism:

- **`model`** (progressive disclosure) — name+description advertised as a synthetic `load_skill_<name>` tool; full body + file manifest inject only when the model calls it. Token cost paid on demand. Requires a tool-calling loop, so only available inside an Agent.
- **`condition`** — attached to a `PromptTemplate` as a conditional `FragmentRef`; `PromptComposer` injects matching bodies deterministically at render time. Works in a bare composer (no loop needed).

An `AgentSpec`'s `skills:` list is one allowlist; each skill's own `trigger` decides whether it surfaces as a load-tool or a conditional fragment — the agent author doesn't choose the mechanism. A bare `PromptComposer` accepts `condition`-triggered skills only.

## Bundled files: skill never executes itself

`FileRef` files are stored as `MediaRef` (inline `< inline_max_bytes`, else MinIO/S3). Two kinds, two paths:

- **Reference docs** — listed in a manifest injected with the skill body; read on demand.
- **Executable scripts** — mounted into the *consuming Agent's* sandbox `Session` (`Meta["bunshin.sandbox_session"]`); the Agent's `CodeExecTool` runs them.

The invariant: **a Skill never executes anything.** Execution always happens in the consuming Agent's sandbox, preserving "Skill = no loop, no model." A skill loaded into a bare LLM call (no sandbox) degrades gracefully — docs inject, scripts are listed-but-unavailable, and the composer sets `Meta["bunshin.skill_scripts_skipped"]`.

## Consequences

- New packages: `pkg/agent` (spec, compiler, store + 4 backends, store_test) and `pkg/skill` (spec, store + 4 backends). Mirrors `pkg/workflow` layout.
- `WorkflowNode` gains `type: agent`; the workflow compiler resolves it against `agent.Store`.
- `PromptComposer` learns a `skills` input (condition-triggered only).
- New `Meta` keys: `bunshin.agent_depth`, `bunshin.agent_truncated`, `bunshin.skill_scripts_skipped`.
- `SubagentNode` is **not** removed — it remains the typed (Go-generic) escape hatch for subagents over a narrower non-`map[string]any` state. `AgentSpec` is the declarative path for the common case.
- OpenAPI spec gains `/v1/agents` and `/v1/skills` paths; `api-client` regenerates.
