# mcpshell benchmarks

mcpshell isn't a faster calculator — hand an LLM `bc` and it'll out-compute a
tree-walking interpreter every time. mcpshell earns its place on three things a
pile of discrete tools (or a raw code-exec tool) can't offer at once:

| # | Pillar | Result | |
|---|--------|--------|--|
| 1 | **[LLM-hard problems](llm_hard/README.md)** | tool is exact & **2.5× faster** (36s vs 89s) | reliability where the model is "usually right, slowly" |
| 2 | **[Context savings](context/README.md)** | **−89% to −96%** context per request | one `eval` vs. 112 discrete tool schemas |
| 3 | **[Safe execution](safety/README.md)** | every host escape blocked, by construction | run untrusted model code without exposing the host |

Plus supporting runs:

| Experiment | With mcpshell | Without | Note |
|------------|:-------------:|:-------:|------|
| [Composition](compose/README.md) — self-contained | 5/5 | 5/5 | small inline data: a wash |
| [Composition](compose/README.md) — tool-only (SQL) | **4/4** | — | no reasoning-only path |
| [General suite](results/README.md) | 33/33 | — | broad capability |

## The three pillars

1. **LLM-hard** — the strawberry class. A strong model now clears most of these
   from reasoning, so the win is *reliability and speed*: the tool is exact by
   construction and 2.5× faster, and the gap widens as inputs grow (bonsai
   already failed the longest one — vowels in a 29-letter word — without it).
2. **Context savings** — the load-bearing claim. Exposing capabilities as
   discrete MCP tools costs 8366 prompt tokens *every request* (112 tools); one
   `eval` + a compact reference is 888, and a deferred `help()` base is 341 — a
   96% cut that stays flat as you add capabilities and keeps the KV cache warm.
   Composition also cuts *turns*: a whole pipeline in one call, not one call per
   stage. Reproduce with `./bin/bench context`.
3. **Safe execution** — the precondition for the whole idea. The subset has no
   imports, `eval`, `process`, prototype chain, or ambient I/O, and enforces
   step/time/output limits, so untrusted model code can't reach or hang the
   host. Verified in `toolkit/sandbox_test.go`.

## Reading the token numbers

Reports separate **processed** tokens (non-cached prompt + generated — the real
compute cost) from **cached** (the re-sent system prompt, ~free on a
lightly-loaded server). Don't read raw totals as cost, and note reasoning-only
timeouts abort before usage is reported, so they undercount to 0 tokens. Every
run also records turns and time-to-solution; `bin/bench compare` regenerates the
per-suite `comparison.md`.
