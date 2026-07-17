# Project Euler (non-canonical): with vs. without mcpshell

Twelve Euler-style problems with **perturbed constants** — the answers are *not*
the famous, memorized Project Euler numbers, so the reasoning-only baseline has
to actually compute rather than recall. Phrased neutrally so the same prompt is
fair with and without the tool. Answers are exact integers. Model: `bonsai`.

- **[comparison.md](comparison.md)** — full head-to-head table + headline
- **[with/](with/README.md)** — tool-equipped run · **[without/](without/README.md)** — reasoning-only run

## Headline

| Metric (12 problems) | With mcpshell | Without |
|----------------------|:-------------:|:-------:|
| **Solved** | **9/12** | **0/12** |
| Total turns | 57 | 12 |
| Processed tokens | 31,257 | 11,103 |
| Cached tokens (~free) | 147,453 | 43 |
| Total time | 390s | 642s |

## What the numbers say

- **Remove memorization and the baseline goes to zero.** With the tool, bonsai
  writes a small program and gets the exact answer (9/12). Without it — and with
  no famous answer to recall — it solves **none**: it either times out reasoning
  (0 tokens, aborted) or, on the few it attempts, computes a *wrong* value
  (`euler_07` spent 60s and 6.5k tokens to miss the 9001st prime). An earlier run
  on the *canonical* problems scored the baseline 8/15 — but those "wins" were
  `euler_07/10/14` recited from memory. Perturbing the constants strips that
  crutch and the true gap shows: **9 vs 0**.
- **The tool is also faster here.** 390s with vs 642s without — the baseline
  spends *more* wall time reasoning toward failures than the tool spends solving.
- **Where the tool fails, it's the interpreter's compute ceiling.** The three
  misses (`euler_10` 1.5M-prime sieve, `euler_12` divisor search, `euler_14`
  700k Collatz) are the heaviest computations; the tree-walking interpreter
  can't finish them in budget. Per-problem `tool ms` / `model ms` splits (in each
  detail file) confirm the time goes to interpreter runtime.
- **Turns: ~2 per solve, plus thrash on the hard tail.** No-tool is always 1
  turn (one completion). With the tool, 9 of 12 solve in 2 turns (call `eval`,
  read result); the 57 total is inflated by retries on the compute-heavy
  problems — `euler_14` 14 turns, `euler_06` 11, `euler_10` 7, `euler_12` 6 —
  where the model keeps bumping `extendLimit` against a computation that won't
  finish. Those extra round-trips are cheap (seconds); the cost is interpreter
  runtime, not thinking.
