# Benchmark Results

**Model:** fireworks-deepseek-v4p1-flash-1017
**Date:** 2026-10-26T03:19:34Z
**Score:** 8/8

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| hosted_lookup_email | PASS | 4 | 9342(8930) | 3503ms | 19 | 3484 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_lookup_email.md) |
| hosted_region_paid_invoices | PASS | 6 | 21116(9680) | 7025ms | 101 | 6924 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_region_paid_invoices.md) |
| hosted_gold_open_tickets | PASS | 7 | 13908(23136) | 7966ms | 101 | 7865 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_gold_open_tickets.md) |
| hosted_top_product_delivered | PASS | 7 | 17604(21174) | 12362ms | 234 | 12128 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_top_product_delivered.md) |
| hosted_platform_open_tasks | PASS | 7 | 11123(24884) | 7551ms | 53 | 7498 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_platform_open_tasks.md) |
| hosted_customer_overdue_total | PASS | 6 | 21850(9124) | 6406ms | 42 | 6364 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_customer_overdue_total.md) |
| hosted_west_avg_order | PASS | 7 | 19810(16770) | 7849ms | 62 | 7787 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_west_avg_order.md) |
| hosted_vendor_approved_po | PASS | 7 | 16914(21404) | 11254ms | 205 | 11049 | [detail](fireworks-deepseek-v4p1-flash-1017/hosted_vendor_approved_po.md) |

## Summary

All benchmarks passed.

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 100% (8/8) |
| First-try success | 0/8 |
| Total turns | 51 |
| Processed tokens | 131667 |
| Cached tokens (~free) | 135102 |
| Total tool calls | 43 |
| Tool errors | 8 |
| Avg tool calls/teaser | 5.4 |
| Total time | 63s |
| Avg time/teaser | 7s |
| Error recovery | 7 teaser(s) passed despite a tool error |
