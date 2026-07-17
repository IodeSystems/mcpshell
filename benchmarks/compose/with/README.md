# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-17T23:13:54Z
**Score:** 9/9

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| compose_core_top_region | PASS | 5 | 2294(11633) | 11067ms | 180 | 10887 | [detail](bonsai/compose_core_top_region.md) |
| compose_core_wordfreq_top | PASS | 3 | 1330(6103) | 5782ms | 101 | 5681 | [detail](bonsai/compose_core_wordfreq_top.md) |
| compose_core_flatten_even_sum | PASS | 3 | 1203(6032) | 5640ms | 50 | 5590 | [detail](bonsai/compose_core_flatten_even_sum.md) |
| compose_core_csv_top_dept | PASS | 2 | 1133(3821) | 4345ms | 81 | 4264 | [detail](bonsai/compose_core_csv_top_dept.md) |
| compose_core_pipeline_stats | PASS | 2 | 860(3770) | 3712ms | 12 | 3700 | [detail](bonsai/compose_core_pipeline_stats.md) |
| compose_sql_top_region | PASS | 2 | 738(3809) | 2214ms | 1 | 2213 | [detail](bonsai/compose_sql_top_region.md) |
| compose_sql_top_product | PASS | 2 | 734(3794) | 2270ms | 1 | 2269 | [detail](bonsai/compose_sql_top_product.md) |
| compose_sql_top_month | PASS | 3 | 1036(6121) | 3587ms | 2 | 3585 | [detail](bonsai/compose_sql_top_month.md) |
| compose_sql_region_of_gizmo | PASS | 3 | 1122(6101) | 4060ms | 2 | 4058 | [detail](bonsai/compose_sql_region_of_gizmo.md) |

## Summary

All benchmarks passed.

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 100% (9/9) |
| First-try success | 4/9 |
| Total turns | 25 |
| Processed tokens | 10450 |
| Cached tokens (~free) | 51184 |
| Total tool calls | 16 |
| Tool errors | 5 |
| Avg tool calls/teaser | 1.8 |
| Total time | 42s |
| Avg time/teaser | 4s |
| Error recovery | 3 teaser(s) passed despite a tool error |
