# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-17T20:48:29Z
**Score:** 9/9

| Teaser | Status | Tool Calls | Errors | Total | Tool ms | Model ms | Details |
|--------|--------|-----------|--------|-------|---------|----------|---------|
| compose_core_top_region | PASS | 4 | 3 | 11354ms | 195 | 11159 | [detail](bonsai/compose_core_top_region.md) |
| compose_core_wordfreq_top | PASS | 2 | 1 | 5785ms | 87 | 5698 | [detail](bonsai/compose_core_wordfreq_top.md) |
| compose_core_flatten_even_sum | PASS | 2 | 1 | 5600ms | 51 | 5549 | [detail](bonsai/compose_core_flatten_even_sum.md) |
| compose_core_csv_top_dept | PASS | 1 | 0 | 4332ms | 77 | 4255 | [detail](bonsai/compose_core_csv_top_dept.md) |
| compose_core_pipeline_stats | PASS | 1 | 0 | 3672ms | 13 | 3659 | [detail](bonsai/compose_core_pipeline_stats.md) |
| compose_sql_top_region | PASS | 1 | 0 | 2179ms | 1 | 2178 | [detail](bonsai/compose_sql_top_region.md) |
| compose_sql_top_product | PASS | 1 | 0 | 2212ms | 1 | 2211 | [detail](bonsai/compose_sql_top_product.md) |
| compose_sql_top_month | PASS | 2 | 0 | 3575ms | 2 | 3573 | [detail](bonsai/compose_sql_top_month.md) |
| compose_sql_region_of_gizmo | PASS | 2 | 0 | 4099ms | 3 | 4096 | [detail](bonsai/compose_sql_region_of_gizmo.md) |

## Summary

All benchmarks passed.

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 100% (9/9) |
| First-try success | 4/9 |
| Total tool calls | 16 |
| Tool errors | 5 |
| Avg tool calls/teaser | 1.8 |
| Total time | 42s |
| Avg time/teaser | 4s |
| Error recovery | 3 teaser(s) passed despite a tool error |
