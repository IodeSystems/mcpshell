# Composition: with vs. without mcpshell

Nine problems that reward composing a whole pipeline into **one `eval`** rather
than round-tripping. Five are self-contained (data inline in the prompt — fair
with and without the tool); four query a seeded SQLite `shop` fixture and are
**tool-only** (the model can't know the rows otherwise). Model: `bonsai`.

- **[comparison.md](comparison.md)** — full table + headline
- **[with/](with/README.md)** — tool-equipped run · **[without/](without/README.md)** — reasoning-only run

## Headline

| Metric | With mcpshell | Without |
|--------|:-------------:|:-------:|
| Self-contained solved | 5/5 | 5/5 |
| Tool-only (SQL) solved | **4/4** | — (impossible) |
| Self-contained turns | 17 | 5 |
| Self-contained processed tokens | 7,678 | 4,272 |
| Self-contained cached (~free) | 38,734 | 215 |

## What the numbers say

- **On small inline data, the tool doesn't change correctness** — bonsai groups,
  sums, and sorts a six-row array in its head as well as in code (5/5 either
  way). The tool costs a few more turns and ~2k more *processed* tokens here
  (7.7k vs 4.3k; the ~39k of re-sent system prompt is cached, ~free) — the
  overhead of *having* a tool, and the wrong place to look for its value.
- **The value shows on the SQL problems.** Composing `shop.query(...)` with core
  pipes (`groupBy` / `sum` / `sort`) solves revenue-by-region, top-product, and
  per-region argmax in **one or two evals** — and the reasoning-only baseline
  can't attempt them at all, because the data lives only behind the tool.
- **Composition keeps tool calls low.** The self-contained problems solve in 1–2
  tool calls; a good composer does the whole pipeline in a single expression
  instead of a call per stage. (Adding `sum`/`avg`/`product` to the core toolkit
  removed the `reduce((a,x)=>a+x,0)` boilerplate that earlier made the model
  thrash on the CSV problem.)
