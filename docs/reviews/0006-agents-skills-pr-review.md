# PR Review: Declarative Agents & Skills (ADR-0006)

**Scope:** `git diff 0ccc16b..HEAD` — `pkg/agent/**`, `pkg/skill/**` (~5087 insertions, 30 files)
**Date:** 2026-06-24
**Reviewers:** pr-review-toolkit (code-reviewer, silent-failure-hunter, pr-test-analyzer, type-design-analyzer, comment-analyzer)
**Tests:** 22/22 pass (postgres backends self-skip without live DB)

This is a review-only document. No code was modified.

---

## Critical (must fix before merge)

### C1. Router iteration counter is per-`CompiledAgent` closure state — breaks reuse + data race
`pkg/agent/compiler.go:598-599`
`createContentBasedRouter` closes over a mutable `step int` baked into the single router instance stored in the graph. ADR-0006 says agents compile once, invoke many times. Consequences:
- **Reuse bug:** the counter is never reset per invocation. The 2nd `Invoke` resumes the count; after ~`maxIterations` total calls every future invocation truncates immediately.
- **Data race:** concurrent invocations of one `CompiledAgent` mutate `step` without synchronization. Graphs are explicitly designed to be reused.

**Fix:** track iteration count in `state.Meta` (e.g. `bunshin.agent_iter`), incremented inside the node/router, not in a closure. Add a `-race` test invoking one compiled agent concurrently. *(Regression introduced by the iteration-cap fix; highest priority.)*

### C2. LLM node splits one assistant turn into N+1 messages
`pkg/agent/compiler.go:412-420`
The LLM node appends `resp.Content` as a `RoleAssistant` text message, then appends each tool call as a *separate* `RoleAssistant` message. Anthropic/OpenAI require tool_calls to live on the same assistant message as content; split history will be rejected or mis-ordered.
**Fix:** emit one assistant `Message` whose `Parts` carry both text and `PartTypeToolCall` parts.

### C3. `agent.SkillSpec` interface is unsatisfiable by the real `skill.Spec`
`pkg/agent/interfaces.go:122` declares `SkillSpec interface { SkillName() string }`. The concrete `skill.Spec` (`pkg/skill/spec.go:28`) has field `Name` but **no `SkillName()` method**. Only `FakeSkillSpec` implements it. No production path turns a stored `skill.Spec` into an `agent.SkillSpec`; the entire skill half of the compiler is wired to a test-only placeholder.
**Fix:** add `SkillName() string` to `skill.Spec`, or retype `SkillResolver` to return `*skill.Spec`. Also widen the contract to expose `Trigger`/`Body` — the compiler must distinguish `model` (load-tool) from `condition` (composer-injected) skills per ADR.

### C4. Tool execution errors silently folded into LLM-visible strings
`pkg/agent/compiler.go:507`
`fmt.Sprintf("error: %v", err)` becomes the tool result. Never logged, traced, or propagated. A broken tool (timeout/auth/panic-recover) is indistinguishable from a tool returning the literal text "error: ...". Operators get zero signal.
**Fix:** log with tool-call context + stable error ID; return a distinguishable structured result (e.g. `{"tool_error": "..."}`).

### C5. Malformed sub-agent arguments silently swallowed
`pkg/agent/compiler.go:554`
`if err := json.Unmarshal([]byte(tc.Arguments), &argMap); err == nil` — on malformed/non-object args, the raw JSON blob is passed as the sub-agent `Task` with empty args and the caller never learns.
**Fix:** on unmarshal error, return explicit `error: invalid arguments for agent %q: %v` and log.

### C6. FakeProvider cannot emit tool calls → the agent loop is structurally untested
`pkg/llm/fake.go:27`
`Complete` only returns `Response{Content}`, never `ToolCalls`. Every loop/delegation/truncation behavior is tested only in router/helper isolation, never end-to-end through `graph.Invoke`. This is why the C1 reuse bug slipped through.
**Fix:** add scripted tool-call capability (e.g. `ToolCallScript [][]llm.ToolCall` consumed per call). Then add the missing end-to-end tests (see T-block).

### Comment-rot criticals (docstrings describe unimplemented behavior)
- `pkg/agent/compiler.go:53` — Compile docstring "Step 7: Wrap in iteration cap middleware". No middleware; enforced in router closure. **Fix wording.**
- `pkg/agent/spec.go:51` — `InputSchema` "Enforced at Compile time and at Invoke time." Neither validates; `AgentState.Args` is never read. **Fix wording / implement.**
- `pkg/agent/spec.go:55` — `OutputSchema` "Enforced at Compile time; triggers a final structured turn." Nothing reads `outputSchema`. **Fix wording / implement.**

---

## Important (should fix)

### Correctness / divergence from ADR
- **I1** `pkg/agent/compiler.go:381` — every turn rebuilds `messages` by prepending system+task then appending full history, re-injecting the task on later turns. **Fix:** seed `Messages` once with system+task, loop appends only.
- **I2** `pkg/agent/compiler.go:524` — model-triggered skill load returns a stub string; never injects Fragment body or mounts scripts. ADR guarantees body+manifest injection. **Fix:** wire `PromptBackend` through registries, or don't advertise the load tool until implemented (returning fake success misleads the model).
- **I3** `pkg/agent/compiler.go:98` — `OutputSchema` stored, never enforced; documented output contract silently unhonored.
- **I4** `pkg/agent/store/memory.go:149,161` — `Activate` has a dead empty `if` block; memory entry has no per-version status, so `ListVersions` reports demoted versions as `"draft"`. Diverges from postgres semantics, breaking backend interchangeability. **Fix:** track per-version status like the *skill* memory store already does.

### Concurrency / transactions in stores
- **I5** `pkg/agent/store/postgres.go:86` & `pkg/skill/store/postgres/postgres.go:81` — `Create` runs resurrect-UPDATE + INSERT as two un-transacted `Exec` calls; concurrent Delete/Activate interleaving leaves inconsistent status. **Fix:** wrap in `Begin/Commit`. (Note: `Activate` already uses a transaction — good.)
- **I6** `pkg/agent/store/git.go` — agent GitStore has **no mutex** while go-git mutates refs/worktree; the *skill* git store does have one. Concurrent Create/Activate corrupts the worktree. **Fix:** add `sync.Mutex`.

### Silent failures in stores
- **I7** `pkg/skill/store/git/git.go:202` — `List` checks stale `err` from the earlier `Worktree()` call, not the `ReadDir` result; real read failures masked as "tenant not found". **Fix:** `entries, err := wt.Filesystem.ReadDir(...)` and branch on that.
- **I8** `pkg/agent/store/git.go:152,165` — `ListVersions` does `version, _ := contentHashYAML(spec)`, discarding the error; a marshal failure records `Version: ""`, which `GetVersion`/`Activate` never match.
- **I9** `pkg/agent/store/git.go:84-102` & blob `ListVersions` (`blob.go:152`) — decode/ReadAll errors swallowed in happy-path `if err == nil && ...`; corrupt drafts silently skipped, transient read errors silently downgrade status to "draft". Also re-reads `active.txt` per version (N round-trips).

### Type design
- **I10** `pkg/agent/spec.go:42` — `Model.Tier` validated nowhere, unlike `skill.Parse`'s `Trigger` check; typos defer to a confusing "no provider found" runtime error. **Fix:** validate in `Parse` or make `type ModelTier string` with constants.
- **I11** MaxIterations default duplicated in `Parse` (`spec.go:76`) and `Compile` (`compiler.go:98`). Evidence the type can't guarantee the invariant. **Fix:** single authority (drop from Parse, or `EffectiveMaxIterations()`).
- **I12** `AgentSpec{}` struct-literal bypasses all `Parse` validation; stores return `*AgentSpec` with no guarantee it was validated. **Fix:** add `Validate()` called by `Compile` and store `Create`.

### Test gaps (all blocked on C6 FakeProvider fix)
- **T1** Multi-turn LLM↔tools loop never tested end-to-end. Add `TestCompile_Invoke_MultiTurnToolLoop`.
- **T2** Iteration-cap truncation never tested through `graph.Invoke` (only router in isolation) — would have caught C1. Add: compile with `MaxIterations:3`, scripted provider returns tool call every turn, assert termination + `Meta["bunshin.agent_truncated"]==true`.
- **T3** Agent-as-tool delegation never tested through a real parent invoke.
- **T4** `TestInvokeSubAgent_MetaPropagation` asserts **nothing** (comment admits it). Make a recording fake; assert `agent_depth==parentDepth+1` and trace/tenant/cost copied.
- **T5** Agent store `blob`, `git`, `postgres` backends have **zero tests** (only `memory_test.go`). Skill side is fully tested with `memblob`/go-git memory storage — mirror it.
- **T6** Cycle detector tested only for trivial A→B→A. Add A→B→C→A and self-cycle A→A (exercises the risky `findCycleDFS` path extraction).

---

## Suggestions (nice to have)

- **S1** `pkg/agent/compiler.go:362,388` — `selectProvider` returns `providers[0]` and `toolDefs` ranges over maps → non-deterministic provider choice and tool ordering. Defeats prompt caching, makes traces noisy. **Sort by name.**
- **S2** `pkg/agent/compiler.go:543` — depth guard reads `.(int)`; a JSON round-trip storing depth as `int64`/`float64` silently resets to 0, defeating the guard. Coerce numeric types.
- **S3** `pkg/agent/compiler.go:587` — `invokeSubAgent` returns `""` on failed type-assert or zero messages; indistinguishable from a successful empty answer. Return a diagnostic + log.
- **S4** `pkg/agent/store/memory.go:64` — redundant `bucket[spec.Name] = e` (already stored, `e` is a pointer). Dead code.
- **S5** `pkg/agent/store/memory.go:138` — `ListVersions` fabricates `CreatedAt: time.Now()` for every version; time ordering meaningless. Store real creation time.
- **S6** `pkg/skill/store/postgres/postgres.go:256` — `Delete` returns bare error without the `skill/postgres:` wrapping used elsewhere.
- **S7** `pkg/skill/store/blob/blob.go:163` — `Delete` of a never-activated skill surfaces a raw blob not-found instead of `skill.ErrNotFound`. Translate via `gcerrors.Code`.
- **S8** `pkg/agent/spec.go:23` — `SystemPrompt struct{ Slug string }` inline anonymous struct is awkward to construct and can't be named. The repo already has `prompt.FragmentRef`; `skill.Spec.Body` uses it. **Replace for consistency** (gains Overrides/Condition for free). Likewise extract `Model` into `type ModelSelector struct`.
- **S9** version-metadata types diverge: `agent/store.AgentVersion` (struct, "draft/active/deleted", newest-first) vs `skill/store.SkillVersion` (`[]string`, "draft/active", oldest-first). Align return type, status enum, ordering.
- **S10** `Status` is a free `string`; make it a named type with the package constants as its only values.
- **S11** `CompiledAgent.AgentNames()` returns the backing slice by reference; defensive-copy gap.
- **S12** Godoc missing on `NewFakeSkillSpec`/`FakeSkillSpec` (`pkg/agent/fake.go:8`) — CLAUDE.md requires godoc on every exported symbol.
- **S13** Store package-doc hash descriptions wrong: skill `interfaces.go:4` says "sha256:<hex16> of canonical YAML" but code emits 32 hex chars of canonical **JSON**; agent `interfaces.go:6` `hex(canonical_yaml[:32])` misreads as slicing YAML not the digest. Align both docs.
- **S14** `pkg/agent/compiler.go:578` — tenant flows via `Meta["bunshin.tenant_id"]`; if a parent forgets to set it, sub-agents resolve under empty tenant. Consider stamping tenant into `CompiledAgent` at compile time.

---

## Strengths

- **Cycle detection** is solid: Kahn's for detection + DFS reconstructing the closed cycle path for useful errors; BFS walks the transitive `AgentNames()` graph so A→B→A is caught even when B compiled independently.
- **`CompiledAgent`** is exemplary type design — fully unexported fields, behavior-only surface, satisfies three interfaces, unforgeable outside the package (only `Compile` produces one). The spec types should aspire to this.
- **Content-hash versioning** consistent and deterministic across backends; skill store's sorted-key `canonicalJSON` is the right call for reproducibility.
- **Skill memory store** correctly tracks per-version status and demotes the prior active on `Activate` — the agent memory store should copy this.
- **Postgres `Activate`** (both packages) uses a proper transaction with existence check + demote + promote.
- **Tenant scoping** threaded explicitly through every store method — no implicit global tenant.
- **Skill store tests** run hermetically against in-memory fakes (`memblob`, go-git memory storage) with strong negative-case and tenant-isolation coverage.

---

## Recommended action plan

1. **C1** (router counter → state.Meta) and **C6** (FakeProvider tool calls) first — they unblock the truthful tests (T2) that prove the rest.
2. **C2** (assistant message shape) — without it, no real provider will run the loop.
3. **C3** (`SkillName()` / resolver retype) — unblocks the only unsatisfiable interface; the skill half is currently test-only.
4. **C4/C5** error visibility, then the **I-block** (store transactions/mutexes, ADR divergences, comment rot).
5. Add the **T-block** tests; re-run review.
6. **S-block** as polish.
