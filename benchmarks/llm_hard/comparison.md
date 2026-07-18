# LLM-hard problems: with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| llm_hard_anagram | ✅ | ✅ | 2/1 | 773(3978)/1136(43) | 2.3/10.5s |
| llm_hard_big_multiply | ✅ | ✅ | 2/1 | 707(3975)/1566(43) | 2.0/14.3s |
| llm_hard_count_r_strawberry | ✅ | ✅ | 2/1 | 2502(2244)/263(0) | 3.5/2.4s |
| llm_hard_count_s_mississippi | ✅ | ✅ | 2/1 | 752(3976)/745(43) | 2.5/6.8s |
| llm_hard_count_vowels | ✅ | ❌ | 2/1 | 835(3991)/0(0) | 2.8/30.0s |
| llm_hard_count_words_with_o | ✅ | ✅ | 2/1 | 767(3985)/703(43) | 2.3/6.5s |
| llm_hard_digit_sum_pow | ✅ | ✅ | 2/1 | 774(3980)/775(43) | 2.5/7.3s |
| llm_hard_last_digit_pow | ✅ | ✅ | 2/1 | 816(3978)/618(43) | 3.0/5.8s |
| llm_hard_sort_words | ✅ | ✅ | 2/1 | 762(3981)/242(43) | 2.3/2.5s |
| llm_hard_substring | ✅ | ✅ | 2/1 | 920(3994)/267(43) | 3.6/2.6s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **10/10** | 9/10 |
| Total turns | 20 | 10 |
| Processed tokens | 9608 | 6315 |
| Cached tokens (~free) | 38082 | 344 |
| Total time | 27s | 89s |
