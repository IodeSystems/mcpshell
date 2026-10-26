# With vs. without mcpshell

**Model:** DeepSeek v4p1 flash (fireworks-deepseek-v4p1-flash-1017) &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| anagram_check | ✅ | ✅ | 2/1 | 547(4552)/230(0) | 1.7/1.0s |
| balanced_parens | ✅ | ✅ | 2/1 | 1454(3806)/282(0) | 2.8/1.1s |
| binary_search | ✅ | ✅ | 2/1 | 3697(1530)/198(0) | 2.2/0.8s |
| bitwise_flags | ✅ | ✅ | 2/1 | 2849(2368)/300(0) | 0.8/0.8s |
| closure_counter | ✅ | ✅ | 2/1 | 2183(3060)/196(0) | 2.4/0.7s |
| collatz_steps | ✅ | ✅ | 2/1 | 573(4558)/292(0) | 1.6/1.1s |
| compose_core_csv_top_dept | ✅ | ✅ | 3/1 | 3860(4634)/216(0) | 3.7/0.8s |
| compose_core_flatten_even_sum | ✅ | ✅ | 2/1 | 2071(3060)/160(0) | 2.3/0.6s |
| compose_core_pipeline_stats | ✅ | ✅ | 2/1 | 2757(2276)/217(0) | 1.8/1.0s |
| compose_core_top_region | ✅ | ✅ | 2/1 | 793(4552)/202(0) | 2.2/0.5s |
| compose_core_wordfreq_top | ✅ | ✅ | 3/1 | 1764(6300)/214(0) | 2.7/1.1s |
| count_letter_l_lullaby | ✅ | ✅ | 2/1 | 2749(2276)/193(0) | 1.5/0.9s |
| count_letter_r_strawberry | ✅ | ✅ | 2/1 | 1213(3812)/256(0) | 1.5/1.0s |
| count_words_with_letter | ✅ | ✅ | 2/1 | 495(4558)/251(0) | 1.6/0.9s |
| curry | ✅ | ✅ | 2/1 | 2843(2282)/245(0) | 1.8/1.2s |
| deep_clone | ✅ | ❌ | 9/1 | 9156(22574)/546(0) | 16.5/1.9s |
| digit_sum_power | ✅ | ✅ | 2/1 | 483(4558)/188(0) | 1.8/0.7s |
| escape_heavy_strings | ✅ | ✅ | 2/1 | 5579(0)/340(0) | 2.4/1.1s |
| event_emitter | ✅ | ✅ | 2/1 | 3080(2282)/302(0) | 3.3/1.2s |
| factorial | ✅ | ✅ | 2/1 | 3447(1530)/181(0) | 2.0/0.9s |
| fibonacci_memo | ✅ | ✅ | 2/1 | 2087(3060)/189(0) | 2.0/1.1s |
| fizzbuzz | ✅ | ✅ | 2/1 | 3693(1530)/278(0) | 2.0/1.4s |
| linked_list | ✅ | ✅ | 2/1 | 2252(3060)/220(0) | 2.2/1.0s |
| llm_hard_anagram | ✅ | ✅ | 2/1 | 465(4552)/235(0) | 1.6/1.0s |
| llm_hard_big_multiply | ✅ | ✅ | 2/1 | 403(4552)/246(0) | 1.9/0.9s |
| llm_hard_count_r_strawberry | ✅ | ✅ | 2/1 | 2713(2276)/139(0) | 1.3/0.8s |
| llm_hard_count_s_mississippi | ✅ | ✅ | 2/1 | 3453(1530)/141(0) | 3.1/0.9s |
| llm_hard_count_vowels | ✅ | ✅ | 2/1 | 453(4590)/659(0) | 1.6/2.3s |
| llm_hard_count_words_with_o | ✅ | ✅ | 2/1 | 467(4554)/163(0) | 1.4/0.6s |
| llm_hard_digit_sum_pow | ✅ | ✅ | 2/1 | 439(4552)/136(0) | 2.4/0.8s |
| llm_hard_last_digit_pow | ✅ | ✅ | 2/1 | 2724(2276)/131(0) | 1.2/0.6s |
| llm_hard_sort_words | ✅ | ✅ | 2/1 | 429(4580)/145(0) | 1.5/0.7s |
| llm_hard_substring | ✅ | ✅ | 2/1 | 1994(3060)/165(0) | 1.6/0.8s |
| longest_common_subsequence | ✅ | ✅ | 2/1 | 1521(3812)/1014(0) | 3.3/3.5s |
| matrix_multiply | ✅ | ✅ | 2/1 | 2900(2282)/278(0) | 1.9/1.1s |
| merge_sort | ✅ | ✅ | 2/1 | 3070(2322)/247(0) | 5.7/1.2s |
| nth_prime | ✅ | ✅ | 2/1 | 1311(3812)/164(0) | 3.0/0.8s |
| object_transform | ✅ | ✅ | 2/1 | 2871(2282)/278(0) | 1.6/1.1s |
| pipe_chain | ✅ | ✅ | 2/1 | 1394(3806)/257(0) | 7.5/1.0s |
| pipe_csv_parse | ✅ | ✅ | 2/1 | 1490(3812)/335(0) | 1.9/1.3s |
| pipe_wordfreq | ✅ | ✅ | 2/1 | 481(4606)/333(0) | 1.4/1.3s |
| recursive_flatten | ✅ | ❌ | 2/1 | 5291(0)/242(0) | 2.0/1.0s |
| reduce_groupby | ✅ | ✅ | 2/1 | 2251(3060)/339(0) | 2.2/1.2s |
| regex_extract | ✅ | ✅ | 2/1 | 1313(3812)/250(0) | 2.1/1.2s |
| roman_numerals | ✅ | ✅ | 2/1 | 775(4568)/330(0) | 3.5/1.3s |
| scatter_parallel | ✅ | ✅ | 2/1 | 1325(3812)/228(0) | 1.6/0.9s |
| string_manipulation | ✅ | ✅ | 2/1 | 505(4560)/234(0) | 2.3/0.9s |
| tower_of_hanoi | ✅ | ✅ | 2/1 | 447(4564)/173(0) | 1.6/0.9s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **48/48** | 46/48 |
| Total turns | 105 | 48 |
| Processed tokens | 100110 | 12558 |
| Cached tokens (~free) | 179850 | 0 |
| Total time | 122s | 51s |

Plus 4 tool-only problem(s) the tool-equipped agent solves that are impossible without it (below).

## Tool-only (no baseline — needs data/state the model can't have)

| Problem | With | Tool calls | Tool ms | Model ms |
|---------|:----:|:----------:|--------:|---------:|
| compose_sql_region_of_gizmo | ✅ | 1 | 36 | 2163 |
| compose_sql_top_month | ✅ | 1 | 0 | 2138 |
| compose_sql_top_product | ✅ | 1 | 1 | 1364 |
| compose_sql_top_region | ✅ | 1 | 1 | 1593 |
