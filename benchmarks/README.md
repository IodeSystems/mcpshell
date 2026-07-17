# mcpshell benchmarks

**Headline: an LLM given mcpshell solves more — and unlocks problems it can't
touch without a tool.** Same model (`bonsai`), same problems, scored with the
`mcpshell` eval tool available vs. reasoning-only.

On the head-to-head, self-contained problems (the model *could* answer either
way), the tool-equipped agent wins on correctness; on data/state problems it
wins outright, because the reasoning-only baseline simply cannot reach the data.

| Experiment | With mcpshell | Without | What it shows |
|------------|:-------------:|:-------:|---------------|
| [Project Euler](euler/README.md) (non-canonical) | **9/12** | **0/12** | Compute vs. can't |
| [Composition](compose/README.md) — self-contained | 5/5 | 5/5 | Small inline data: tie |
| [Composition](compose/README.md) — tool-only (SQL) | **4/4** | — | Impossible without tool access |
| [General suite](results/README.md) | 33/33 | — | Broad mcpshell capability |

**The Euler problems use perturbed constants, so their answers aren't famous
numbers a model can recall — and there the gap is stark: 9/12 with the tool vs.
0/12 without.** (An earlier canonical run flattered the baseline to 8/15, but
those were memorized answers.) On composition, small inline data is a wash
(5/5 either way), but the 4 SQL problems have no reasoning-only path at all.
Net: the tool is decisive exactly where the task needs real computation or data
the model can't reach; on trivial inline arithmetic it doesn't matter.

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
  tool vs. 11k without** (+147k cached ≈ free) — and the tool solved 9 while the
  baseline solved 0.
- **Without-tool token totals are undercounted anyway.** A reasoning-only
  completion that hits the time budget is aborted mid-generation, so the API
  returns *no* usage — most no-tool Euler problems show **0 tokens** despite
  30–120s of "thinking." So even 11k overstates what it accomplished (nothing).
- **The tool is also faster on the Euler suite** (390s vs 642s): the baseline
  burns *more* wall time reasoning toward failures than the tool spends solving.

See each experiment's README for the full tables and per-problem detail.
