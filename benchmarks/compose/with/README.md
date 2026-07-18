# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-18T04:37:29Z
**Score:** 9/9

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| compose_core_top_region | PASS | 6 | 2571(15290) | 12275ms | 182 | 12093 | [detail](bonsai/compose_core_top_region.md) |
| compose_core_wordfreq_top | PASS | 3 | 1354(6465) | 5437ms | 120 | 5317 | [detail](bonsai/compose_core_wordfreq_top.md) |
| compose_core_flatten_even_sum | PASS | 3 | 1136(6388) | 4626ms | 30 | 4596 | [detail](bonsai/compose_core_flatten_even_sum.md) |
| compose_core_csv_top_dept | PASS | 3 | 1687(6603) | 7054ms | 121 | 6933 | [detail](bonsai/compose_core_csv_top_dept.md) |
| compose_core_pipeline_stats | PASS | 2 | 930(3988) | 3812ms | 13 | 3799 | [detail](bonsai/compose_core_pipeline_stats.md) |
| compose_sql_top_region | PASS | 2 | 808(4027) | 2271ms | 1 | 2270 | [detail](bonsai/compose_sql_top_region.md) |
| compose_sql_top_product | PASS | 2 | 836(4012) | 2613ms | 1 | 2612 | [detail](bonsai/compose_sql_top_product.md) |
| compose_sql_top_month | PASS | 3 | 1091(6483) | 3557ms | 2 | 3555 | [detail](bonsai/compose_sql_top_month.md) |
| compose_sql_region_of_gizmo | PASS | 3 | 1192(6463) | 4095ms | 3 | 4092 | [detail](bonsai/compose_sql_region_of_gizmo.md) |

## Summary

All benchmarks passed.

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 100% (9/9) |
| First-try success | 3/9 |
| Total turns | 27 |
| Processed tokens | 11605 |
| Cached tokens (~free) | 59719 |
| Total tool calls | 18 |
| Tool errors | 5 |
| Avg tool calls/teaser | 2.0 |
| Total time | 45s |
| Avg time/teaser | 5s |
| Error recovery | 4 teaser(s) passed despite a tool error |
