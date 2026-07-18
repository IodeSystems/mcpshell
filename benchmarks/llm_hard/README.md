# LLM-hard problems: with vs. without mcpshell

The "how many r's in strawberry" class — exact letter counting, arithmetic on
ugly numbers, digit manipulation, precise string ops. Things LLMs historically
get wrong from memory but that are trivial with a line of code. Phrased neutrally
so the same prompt is fair with and without the tool. Model: `bonsai`.

- **[comparison.md](comparison.md)** — full table + headline
- **[with/](with/README.md)** · **[without/](without/README.md)**

## Headline

| Metric (10 problems) | With mcpshell | Without |
|----------------------|:-------------:|:-------:|
| **Solved** | **10/10** | 9/10 |
| Total turns | 24 | 10 |
| Processed tokens | 10,811 | 6,315* |
| Total time | **36s** | 89s |

\* undercounted — the one failure timed out and reported 0 tokens.

## What the numbers say — honestly

- **A capable model mostly clears these now.** bonsai (27B) counted the r's in
  strawberry (3), multiplied 3947×5821 (22975487), and summed digits of 2^20 (31)
  *without* the tool. The "LLMs can't count letters" era is fading for strong
  models, so the raw solve gap here is small: 10 vs 9.
- **But the tool is 2.5× faster and never silently wrong.** Without it the model
  grinds through chain-of-thought — 89s total vs 36s — and it *did* fail one:
  counting vowels in `floccinaucinihilipilification` (a 29-letter word), where it
  ran out its budget. With the tool every answer is exact by construction; the
  reasoning-only path is exact *until it isn't*, with no signal about which.
- **The gap widens with instance size.** The problems here are deliberately
  small. Push to longer strings, more digits, or bigger counts and the
  reasoning-only accuracy degrades while the tool stays exact and O(1) in the
  model's effort — the vowel count (the longest input) is already where the
  baseline broke.

This pillar is about **reliability and cost**, not a headline solve-rate: a
line of code turns "usually right, slowly" into "always right, fast." The
starker structural wins are [context savings](../context/README.md) and
[safe execution](../safety/README.md).
