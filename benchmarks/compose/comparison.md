# Composition: with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| compose_core_csv_top_dept | ✅ | ✅ | 2/1 | 1133(3821)/976(0) | 4.3/8.2s |
| compose_core_flatten_even_sum | ✅ | ✅ | 3/1 | 1203(6032)/672(0) | 5.6/5.8s |
| compose_core_pipeline_stats | ✅ | ✅ | 2/1 | 860(3770)/1684(0) | 3.7/15.1s |
| compose_core_top_region | ✅ | ✅ | 5/1 | 2294(11633)/716(0) | 11.1/5.9s |
| compose_core_wordfreq_top | ✅ | ✅ | 3/1 | 1330(6103)/439(0) | 5.8/3.7s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **5/5** | 5/5 |
| Total turns | 15 | 5 |
| Processed tokens | 6820 | 4487 |
| Cached tokens (~free) | 31359 | 0 |
| Total time | 31s | 39s |

Plus 4 tool-only problem(s) the tool-equipped agent solves that are impossible without it (below).

## Tool-only (no baseline — needs data/state the model can't have)

| Problem | With | Tool calls | Tool ms | Model ms |
|---------|:----:|:----------:|--------:|---------:|
| compose_sql_region_of_gizmo | ✅ | 2 | 2 | 4058 |
| compose_sql_top_month | ✅ | 2 | 2 | 3585 |
| compose_sql_top_product | ✅ | 1 | 1 | 2269 |
| compose_sql_top_region | ✅ | 1 | 1 | 2213 |
