# Routers modelled as Enterprise Integration Patterns

YAML workflows reference routers by name from a `RouterRegistry`. Bunshin ships a v1 catalog of routers modelled on Apache Camel's Enterprise Integration Patterns ([eip.adoc](https://camel.apache.org/components/4.18.x/eips/enterprise-integration-patterns.html)). EIPs map remarkably well to LLM workflow patterns and provide a stable, well-documented vocabulary that reviewers from a Java / integration background will recognise instantly.

No LLM framework I am aware of frames its routing layer as EIPs. LangChain has the `Runnable` composition API but no first-class router catalog. LangGraph routers are arbitrary Go-like Python functions. Framing this as EIPs is a deliberate differentiator.

## v1 catalog

| EIP | YAML type | LLM use case |
|---|---|---|
| Content-Based Router | `content_based` | Branch on `State.Meta[key]` value — most common agent-loop pattern |
| Message Filter | `filter` | Conditional skip (cache hit → bypass LLM) |
| Routing Slip | `routing_slip` | LLM emits ordered tool plan; executor runs each step in order |
| Recipient List | `recipient_list` | Fan-out to multiple nodes (ensemble, multi-perspective) |
| Splitter | `splitter` | Chunk long doc, run downstream node per chunk |
| Aggregator | `aggregator` | Collect Splitter / Recipient List outputs into one slice |
| Custom | `custom` | Escape hatch — application-registered Router |

`Scatter-Gather`, `Dynamic Router` (LLM-as-router), `Wire Tap`, `Throttler`, and `Load Balancer` are not in v1 — Wire Tap / Throttler / Load Balancer are already covered by existing middleware and `ProviderRegistry`; Scatter-Gather emerges from Splitter+Aggregator+RecipientList composition; Dynamic Router is deferred to v2 because every routing decision requires a real LLM call which has cost and latency implications worth a separate design pass.

## Implementation

- Each EIP is a `Router[map[string]any]` factory in `pkg/graph/router/eip/{content_based,filter,routing_slip,recipient_list,splitter,aggregator}.go`.
- The compiler maintains a built-in `RouterRegistry` populated with these EIPs at startup.
- Application code can register additional routers (e.g. an `llm_router` for the Dynamic Router pattern) under the same registry.
- Each EIP is ~30 lines + table-driven tests; the full v1 catalog fits a single PR.

## Consequences

- The EIP names become part of the public YAML schema. Renaming `content_based` later is a breaking change.
- Splitter and Aggregator must agree on the Meta keys they share (`bunshin.split_count`, `bunshin.split_index`); future EIPs in the same family follow the same convention.
- Dynamic Router (LLM-as-router) is intentionally deferred — adding it later costs nothing in the schema (just a new `type:` entry).
