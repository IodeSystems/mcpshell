# LLM-hard problems: with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| llm_hard_anagram | ✅ | ✅ | 3/1 | 1023(5917)/1136(43) | 3.8/10.5s |
| llm_hard_big_multiply | ✅ | ✅ | 2/1 | 706(3687)/1566(43) | 1.9/14.2s |
| llm_hard_count_r_strawberry | ✅ | ✅ | 2/1 | 2356(2100)/263(0) | 3.3/2.4s |
| llm_hard_count_s_mississippi | ✅ | ✅ | 2/1 | 807(3688)/745(43) | 2.9/6.8s |
| llm_hard_count_vowels | ✅ | ❌ | 2/1 | 839(3703)/0(0) | 2.8/30.0s |
| llm_hard_count_words_with_o | ✅ | ✅ | 2/1 | 767(3697)/703(43) | 2.2/6.6s |
| llm_hard_digit_sum_pow | ✅ | ✅ | 2/1 | 774(3692)/775(43) | 2.4/7.2s |
| llm_hard_last_digit_pow | ✅ | ✅ | 5/1 | 1854(10373)/618(43) | 10.6/5.8s |
| llm_hard_sort_words | ✅ | ✅ | 2/1 | 762(3693)/242(43) | 2.3/2.5s |
| llm_hard_substring | ✅ | ✅ | 2/1 | 923(3706)/267(43) | 3.6/2.5s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **10/10** | 9/10 |
| Total turns | 24 | 10 |
| Processed tokens | 10811 | 6315 |
| Cached tokens (~free) | 44256 | 344 |
| Total time | 36s | 89s |
