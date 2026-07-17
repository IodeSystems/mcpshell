# Composition: with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

| Problem | With | Without | Tool calls | Tool ms | Model ms |
|---------|:----:|:-------:|:----------:|--------:|---------:|
| compose_core_csv_top_dept | ✅ | ✅ | 1 | 77 | 4255 |
| compose_core_flatten_even_sum | ✅ | ✅ | 2 | 51 | 5549 |
| compose_core_pipeline_stats | ✅ | ✅ | 1 | 13 | 3659 |
| compose_core_top_region | ✅ | ✅ | 4 | 195 | 11159 |
| compose_core_wordfreq_top | ✅ | ✅ | 2 | 87 | 5698 |

**Self-contained totals:** with **5/5**, without **5/5**.

## Tool-only (no baseline — needs data/state the model can't have)

| Problem | With | Tool calls | Tool ms | Model ms |
|---------|:----:|:----------:|--------:|---------:|
| compose_sql_region_of_gizmo | ✅ | 2 | 3 | 4096 |
| compose_sql_top_month | ✅ | 2 | 2 | 3573 |
| compose_sql_top_product | ✅ | 1 | 1 | 2211 |
| compose_sql_top_region | ✅ | 1 | 1 | 2178 |
