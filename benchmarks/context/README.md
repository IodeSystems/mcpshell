# Context savings: one eval vs. a tool per capability

The usual way to give an LLM capabilities is one MCP tool per operation. Every
one of those tool schemas is re-sent on **every** request, so the tool surface
is a fixed tax on your context window (and your KV cache). mcpshell exposes a
single `eval` tool instead — capabilities are *commands inside the language*,
not tools in the prompt.

Measured with the model's own tokenizer (`bench context`, prompt tokens above an
empty baseline; core + math + web + graph only — 112 commands):

| Strategy | Tools in prompt | Tokens / request | vs. discrete |
|----------|:---------------:|:----------------:|:------------:|
| **N discrete MCP tools** | 112 | **8366** | — |
| mcpshell `eval` + full reference | 1 | 2323 | −72% |
| mcpshell `eval` + compact reference | 1 | 888 | **−89%** |
| mcpshell `eval` + deferred `help()` | 2 | **341** | **−96%** |

Reproduce: `./bin/bench context`.

## Why it compounds

- **The tax is per request, not per session.** 8366 tokens of tool schema ride
  along on every single call. The compact reference (888) or deferred base (341)
  replaces it with a fixed, tiny surface.
- **It stays flat as you add capabilities.** Register 50 more commands and the
  discrete-tools prompt grows by ~50 more schemas on every request; the `eval`
  tool schema is unchanged, and with `help()` the base stays 341 tokens —
  new commands are discovered at runtime, not front-loaded. (The four toolkits
  measured here are already 112 commands; add file/SQL/browser and the discrete
  side only grows.)
- **KV-cache friendly.** Because the tool surface doesn't change when
  capabilities change, the cached prefix stays valid — where adding or removing
  a discrete tool invalidates it. (In these runs the re-sent mcpshell reference
  is served almost entirely from cache; see the token accounting in the other
  benchmarks.)

## Fewer turns, too

Discrete tools don't just cost schema tokens — they cost **round-trips**. A
pipeline like "group orders by region, sum revenue, sort, take the top" is one
tool call per stage with discrete tools (four round-trips), but a **single
composed `eval`** with mcpshell:

```
orders |> groupBy(o => o.region)
       |> map(g => sum(g |> map(o => o.total)))
       |> sort("desc") |> at(0)
```

The composition benchmark shows the tool-equipped model solving these in 1–2
turns rather than a turn per operation — fewer round-trips, less latency, and no
intermediate results shuttled back through the context window.
