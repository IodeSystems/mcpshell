# Project Euler: with vs. without mcpshell

Fifteen Project Euler problems (easy → compute-heavy, plus non-canonical
variants), phrased neutrally so the *same* prompt is fair with and without the
tool. Answers are exact integers. Model: `bonsai`.

- **[comparison.md](comparison.md)** — full head-to-head table + headline
- **[with/](with/README.md)** — tool-equipped run · **[without/](without/README.md)** — reasoning-only run

## Headline

| Metric (self-contained, 15 problems) | With mcpshell | Without |
|--------------------------------------|:-------------:|:-------:|
| **Solved** | **11/15** | 8/15 |
| Total turns | 60 | 15 |
| Processed tokens | 31,210 | 18,717* |
| Cached tokens (~free) | 143,959 | 0 |
| Total time | 495s | 498s |

\* **Processed** = non-cached prompt + generated (the compute cost); the ~3.8k
system prompt is cached each turn, so it lands in the free column, not here. The
without total is still undercounted — reasoning-only timeouts abort before usage
is returned, so 7 of the 15 report 0 tokens despite 30–120s of "thinking."

## What the numbers say

- **The tool computes; the baseline guesses or recalls.** With the tool, bonsai
  writes a small program and gets the exact answer (it wins 5 problems the
  baseline fails outright: `euler_02/04/06/09/v1`). Without it, it passes only
  what it can do in its head or *remembers* — and Project Euler answers are
  famously memorized (`euler_07` → 104743 in 4.5s from recall). The non-canonical
  variants (`euler_v1/v2/v3`, perturbed parameters) strip that crutch: the
  baseline solved just one of three.
- **Where the tool loses, it's the interpreter's compute ceiling, not thinking.**
  The four with-tool failures (`euler_10`, `euler_12`, `euler_14`, `euler_v3`)
  are the heaviest computations; the tree-walking interpreter can't finish inside
  the budget. On those the model also *thrashes* — `euler_14` burned 14 turns
  retrying and bumping `extendLimit`. The per-problem `tool ms` / `model ms`
  split (in each detail file) shows whether time went to interpreter runtime or
  model round-trips.
- **Two problems invert the story** (`euler_10`, `euler_14`): with the tool they
  ❌ time out grinding a 2M-prime sieve / ~1M Collatz chains; without it the model
  ✅ *recalls* the famous answers (142913828922, 837799) in seconds. The tool's
  limit is raw compute, not correctness — and the heavy-compute tail is where the
  run-to-run score wobbles (10–12/15).
- **Comparable wall time, more solved.** The reasoning-only baseline spends
  roughly the same total time (498s) — most of it reasoning toward timeouts —
  while solving 8 vs. 11.
