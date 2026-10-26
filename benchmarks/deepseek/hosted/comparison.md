# Hosted upstream: mcpshell vs. discrete tools

**Model:** DeepSeek v4p1 flash (fireworks-deepseek-v4p1-flash-1017) &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = Acme's 102 tools given to the model as discrete tools, no mcpshell.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| hosted_customer_overdue_total | ✅ | ✅ | 6/3 | 21850(9124)/21230(10276) | 6.4/3.8s |
| hosted_gold_open_tickets | ✅ | ✅ | 7/3 | 13908(23136)/22861(10146) | 8.0/5.3s |
| hosted_lookup_email | ✅ | ✅ | 4/2 | 9342(8930)/10582(10170) | 3.5/2.5s |
| hosted_platform_open_tasks | ✅ | ✅ | 7/5 | 11123(24884)/43608(11394) | 7.6/7.7s |
| hosted_region_paid_invoices | ✅ | ✅ | 6/3 | 21116(9680)/29731(10178) | 7.0/12.6s |
| hosted_top_product_delivered | ✅ | ✅ | 7/7 | 17604(21174)/59356(80234) | 12.4/68.2s |
| hosted_vendor_approved_po | ✅ | ✅ | 7/3 | 16914(21404)/12185(21122) | 11.3/6.1s |
| hosted_west_avg_order | ✅ | ✅ | 7/3 | 19810(16770)/11840(20570) | 7.8/6.9s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **8/8** | 8/8 |
| Total turns | 51 | 29 |
| Processed tokens | 131667 | 211393 |
| Cached tokens (~free) | 135102 | 174090 |
| Total time | 64s | 113s |
