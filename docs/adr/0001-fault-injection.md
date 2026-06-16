# Fault injection middleware design

Fault injection lives in a sub-package `pkg/testing/fault` (not flat `pkg/testing`) to avoid a name collision with the stdlib `testing` package and to leave the namespace free for sibling helpers (`golden`, `replay`, etc.). The API is two independently composable middleware factories — `fault.ErrorRate(p, err)` and `fault.LatencyP50(median, max)` — chained via the existing `middleware.Chain`, rather than a single config-struct constructor; this lets a test inject only latency or only errors without dummy values.

`LatencyP50` samples from a **triangular distribution** on `[0, max]` with the peak at `median`. The realistic alternatives were uniform (which would make the `median` parameter a lie — uniform `[0, max]` has median `max/2`) and log-normal (a more accurate model of real-world LLM tail latency, but overkill for chaos testing and requires extra math). Triangular hits a useful middle: the `median` parameter is the true P50, calls cluster near `median`, occasional ones approach `max`, and sampling is one line on a seeded `*rand.Rand`.

## Consequences

- Tests that depend on observed latency distribution shape (e.g. histogram assertions) must account for triangular, not uniform — changing to a different distribution later would silently invalidate them.
- The `pkg/testing/fault` import path is part of the public API surface; flattening to `pkg/testing` later is a breaking change.
- `ErrorRate` does **not** call the wrapped Runnable when it triggers — wrapped-side counters and telemetry stay clean. Callers wanting post-call faults must reorder middleware.
