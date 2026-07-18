# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-18T00:52:09Z
**Score:** 10/10

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| llm_hard_count_r_strawberry | PASS | 2 | 2356(2100) | 3257ms | 27 | 3230 | [detail](bonsai/llm_hard_count_r_strawberry.md) |
| llm_hard_count_s_mississippi | PASS | 2 | 807(3688) | 2850ms | 6 | 2844 | [detail](bonsai/llm_hard_count_s_mississippi.md) |
| llm_hard_big_multiply | PASS | 2 | 706(3687) | 1940ms | 1 | 1939 | [detail](bonsai/llm_hard_big_multiply.md) |
| llm_hard_digit_sum_pow | PASS | 2 | 774(3692) | 2443ms | 23 | 2420 | [detail](bonsai/llm_hard_digit_sum_pow.md) |
| llm_hard_last_digit_pow | PASS | 5 | 1854(10373) | 10643ms | 25 | 10618 | [detail](bonsai/llm_hard_last_digit_pow.md) |
| llm_hard_anagram | PASS | 3 | 1023(5917) | 3758ms | 33 | 3725 | [detail](bonsai/llm_hard_anagram.md) |
| llm_hard_sort_words | PASS | 2 | 762(3693) | 2310ms | 11 | 2299 | [detail](bonsai/llm_hard_sort_words.md) |
| llm_hard_substring | PASS | 2 | 923(3706) | 3633ms | 13 | 3620 | [detail](bonsai/llm_hard_substring.md) |
| llm_hard_count_words_with_o | PASS | 2 | 767(3697) | 2246ms | 13 | 2233 | [detail](bonsai/llm_hard_count_words_with_o.md) |
| llm_hard_count_vowels | PASS | 2 | 839(3703) | 2757ms | 14 | 2743 | [detail](bonsai/llm_hard_count_vowels.md) |

## Summary

All benchmarks passed.

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 100% (10/10) |
| First-try success | 8/10 |
| Total turns | 24 |
| Processed tokens | 10811 |
| Cached tokens (~free) | 44256 |
| Total tool calls | 14 |
| Tool errors | 1 |
| Avg tool calls/teaser | 1.4 |
| Total time | 35s |
| Avg time/teaser | 3s |
| Error recovery | 1 teaser(s) passed despite a tool error |
