# mcpshell benchmarks

mcpshell isn't a faster calculator — hand an LLM `bc` and it'll out-compute a
tree-walking interpreter every time. mcpshell earns its place on three things a
pile of discrete tools (or a raw code-exec tool) can't offer at once:

Results are per model. The bonsai runs below (a local 27B model) and the
[DeepSeek v4p1 flash runs](deepseek/README.md) disagree on speed: a fast model
solves small problems from reasoning sooner than it writes and runs code.

| # | Pillar | Result | |
|---|--------|--------|--|
| 1 | **[LLM-hard problems](llm_hard/README.md)** | bonsai: tool is exact & **3.3× faster** (27s vs 89s); DeepSeek: 10/10 either way, tool slower (17.6s vs 9.6s) | reliability where the model is "usually right, slowly" |
| 2 | **[Context savings](context/README.md)** | **−89% to −96%** context per request | one `eval` vs. 113 discrete tool schemas |
| 3 | **[Safe execution](safety/README.md)** | every host escape blocked, by construction | run untrusted model code without exposing the host |

Plus supporting runs:

| Experiment | With mcpshell | Without | Note |
|------------|:-------------:|:-------:|------|
| [Composition](compose/README.md) — self-contained | 5/5 | 5/5 | small inline data: a wash |
| [Composition](compose/README.md) — tool-only (SQL) | **4/4** | — | no reasoning-only path |
| [General suite](results/README.md) | 33/33 | — | broad capability |
| [DeepSeek: hosted 102-tool server](deepseek/README.md) | 8/8, 64 s, 132k tokens, 51 turns | 8/8, 113 s, 211k tokens, 29 turns | without = the 102 tools as discrete tools |
| [DeepSeek: suite](deepseek/README.md) | 48/48 | 46/48 | tool slower, 105 vs 48 turns |

## The three pillars

1. **LLM-hard** — the strawberry class. A strong model now clears most of these
   from reasoning, so the win is *reliability*: the tool is exact by
   construction. On bonsai it was also 3.3× faster and the baseline failed the
   longest input (vowels in a 29-letter word); DeepSeek solved all 10 without
   the tool, faster than with it.
2. **Context savings** — the load-bearing claim. Exposing capabilities as
   discrete MCP tools costs 8453 prompt tokens *every request* (113 tools); one
   `eval` + a compact reference is 1001, and a deferred `help()` base is 341 — a
   96% cut that stays flat as you add capabilities and keeps the KV cache warm.
   Reproduce with `./bin/bench context`. Measured end to end on DeepSeek with a
   102-tool server, hosting it in mcpshell used 38% fewer processed tokens and
   43% less time than the same tools as discrete tools — and more turns (51 vs
   29), spent looking tools up with `help()`.
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
