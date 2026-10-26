# DeepSeek v4p1 flash

Model `fireworks-deepseek-v4p1-flash-1017`, through corrallm, 2026-10-25.
Each problem ran once, so small differences are within run-to-run noise.

## Hosted upstream: mcpshell vs. discrete tools

The case mcpshell is built for: a company-sized MCP server. `bench --hosted`
serves a synthetic Acme backend — 17 entities, 102 list/get/search/create/
update/delete tools, fixed seed — and asks 8 questions that need joins and
pagination. **With** hosts it in mcpshell as the `acme` namespace (one `eval`
tool); **without** gives the model the same 102 tools directly.
[comparison.md](hosted/comparison.md)

| 8 tasks | mcpshell | 102 discrete tools |
|---------|:--------:|:------------------:|
| Solved | 8/8 | 8/8 |
| Turns | 51 | **29** |
| Tool calls | 43 | 56 |
| Processed tokens | **131,667** | 211,393 |
| Total time | **64 s** | 113 s |

mcpshell used 38% fewer processed tokens and 43% less time, and **more
turns**: it looks tools up with `help()` (8 calls over 8 tasks) where the
discrete arm sees every schema up front and calls several per turn. The
largest gap is the 480-row join (top product over delivered orders): 17.6k vs
59.4k processed tokens, 12.4 s vs 68.2 s, in the same 7 turns. On the small
lookups the discrete arm is faster.

How the mcpshell arm got here, same model and tasks:

| Run | Turns | help() calls | Error turns | Processed tokens | Time |
|-----|:-----:|:------------:|:-----------:|:----------------:|:----:|
| v0.5.1 | 83 | 31 | 14 | 172,003 | 86 s |
| + help parameter detail, full tool list, warnings | 60 | 10 | 11 | 205,202 | 84 s |
| + named arguments for tools | 51 | 8 | 8 | 131,667 | 64 s |

The discrete arm ran twice with the same code: 219,119 and 211,393 processed
tokens, 94 s and 113 s — the size of the run-to-run noise.

## Suite: mcpshell vs. no tool

The 48 self-contained problems of `bench` (LLM-hard, composition, general),
with the tool and from reasoning alone. [comparison.md](suite/comparison.md)

| Problems | Solved with / without | Turns | Processed tokens | Time |
|----------|:---------------------:|:-----:|:----------------:|:----:|
| LLM-hard (10) | 10 / 10 | 20 / 10 | 13,540 / 2,160 | 17.6 s / 9.6 s |
| Composition (5) | 5 / 5 | 12 / 5 | 11,245 / 1,009 | 12.7 s / 4.1 s |
| General (33) | 33 / **31** | 73 / 33 | 75,325 / 9,389 | 91.4 s / 37.3 s |
| **All 48** | **48 / 46** | 105 / 48 | 100,110 / 12,558 | 122 s / 51 s |

Plus 4 SQL problems only the tool can answer: 4/4.

DeepSeek solves these small problems from reasoning, so here the tool adds
two solves (deep_clone, recursive_flatten) and costs time, turns and tokens:
the reference prompt is re-sent each turn. The bonsai results in the other
folders ("3.3× faster" on LLM-hard) are for a slower model and do not carry
over.

Reproduce:

```
./bin/bench --model <id> --hosted              # mcpshell arm
./bin/bench --model <id> --hosted --discrete   # discrete-tools arm
./bin/bench --model <id>                       # suite with the tool
./bin/bench --model <id> --no-tool             # suite without
./bin/bench compare <with_dir> <without_dir>
```
