# Composition: with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| compose_core_csv_top_dept | ✅ | ✅ | 3/1 | 1687(6603)/933(43) | 7.1/8.2s |
| compose_core_flatten_even_sum | ✅ | ✅ | 3/1 | 1136(6388)/629(43) | 4.6/5.7s |
| compose_core_pipeline_stats | ✅ | ✅ | 2/1 | 930(3988)/1641(43) | 3.8/16.2s |
| compose_core_top_region | ✅ | ✅ | 6/1 | 2571(15290)/673(43) | 12.3/5.9s |
| compose_core_wordfreq_top | ✅ | ✅ | 3/1 | 1354(6465)/396(43) | 5.4/3.8s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **5/5** | 5/5 |
| Total turns | 17 | 5 |
| Processed tokens | 7678 | 4272 |
| Cached tokens (~free) | 38734 | 215 |
| Total time | 33s | 40s |

Plus 4 tool-only problem(s) the tool-equipped agent solves that are impossible without it (below).

## Tool-only (no baseline — needs data/state the model can't have)

| Problem | With | Tool calls | Tool ms | Model ms |
|---------|:----:|:----------:|--------:|---------:|
| compose_sql_region_of_gizmo | ✅ | 2 | 3 | 4092 |
| compose_sql_top_month | ✅ | 2 | 2 | 3555 |
| compose_sql_top_product | ✅ | 1 | 1 | 2612 |
| compose_sql_top_region | ✅ | 1 | 1 | 2270 |
