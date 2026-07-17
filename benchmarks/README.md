# mcpshell benchmarks

**Headline: an LLM given mcpshell solves more — and unlocks problems it can't
touch without a tool.** Same model (`bonsai`), same problems, scored with the
`mcpshell` eval tool available vs. reasoning-only.

On the head-to-head, self-contained problems (the model *could* answer either
way), the tool-equipped agent wins on correctness; on data/state problems it
wins outright, because the reasoning-only baseline simply cannot reach the data.

| Experiment | With mcpshell | Without | What it shows |
|------------|:-------------:|:-------:|---------------|
| [Project Euler](euler/README.md) | **11/15** | 8/15 | Compute exactly vs. guess/recall |
| [Composition](compose/README.md) — self-contained | 5/5 | 5/5 | Small inline data: tie |
| [Composition](compose/README.md) — tool-only (SQL) | **4/4** | — | Impossible without tool access |
| [General suite](results/README.md) | 33/33 | — | Broad mcpshell capability |

**Self-contained head-to-head: 16/20 with vs. 13/20 without**, plus 4 tool-only
problems the tool-equipped agent solves that have no reasoning-only path. (The
heavy-compute Euler tail is noisy — the tool-equipped score varies 10–12/15
run to run as the slowest problems sit on the interpreter's timeout edge.)

## What's measured

Each run records, per problem: pass/fail, **turns** (model round-trips),
**tokens** (processed + generated), **time-to-solution**, and a **tool-runtime
vs. model-time** split. Raw per-run records live in `results.json` next to each
markdown index; `bin/bench compare <with> <without>` regenerates the
`comparison.md` files.

### Reading the cost numbers honestly

- **Count processed tokens, not cached.** The mcpshell system prompt (the
  language reference, ~3.8k tokens) is re-sent every turn but served from the
  prompt/KV cache — essentially free on a lightly-loaded server. What actually
  costs compute is *processed* = non-cached prompt + generated. Reports show
  `processed(cached)`; a typical one-shot solve is only ~700–1000 processed
  tokens against ~3.8k cached. Over the Euler suite: **31k processed with the
  tool vs. 19k without** (+144k cached ≈ free) — not the 14× gap the raw totals
  suggest.
- **Without-tool token totals are undercounted anyway.** A reasoning-only
  completion that hits the time budget is aborted mid-generation, so the API
  returns *no* usage — those problems show **0 tokens** despite 30–120s of
  "thinking." So even 19k understates the baseline. Read **correctness**.
- **Total wall time is comparable** even though the tool-equipped agent solves
  more (the baseline burns its time reasoning toward timeouts).

See each experiment's README for the full tables and per-problem detail.
