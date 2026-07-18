# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-18T00:53:37Z
**Score:** 9/10

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| llm_hard_count_r_strawberry | PASS | 1 | 263(0) | 2398ms | 0 | 2398 | [detail](bonsai/llm_hard_count_r_strawberry.md) |
| llm_hard_count_s_mississippi | PASS | 1 | 745(43) | 6777ms | 0 | 6777 | [detail](bonsai/llm_hard_count_s_mississippi.md) |
| llm_hard_big_multiply | PASS | 1 | 1566(43) | 14231ms | 0 | 14231 | [detail](bonsai/llm_hard_big_multiply.md) |
| llm_hard_digit_sum_pow | PASS | 1 | 775(43) | 7224ms | 0 | 7224 | [detail](bonsai/llm_hard_digit_sum_pow.md) |
| llm_hard_last_digit_pow | PASS | 1 | 618(43) | 5841ms | 0 | 5841 | [detail](bonsai/llm_hard_last_digit_pow.md) |
| llm_hard_anagram | PASS | 1 | 1136(43) | 10470ms | 0 | 10470 | [detail](bonsai/llm_hard_anagram.md) |
| llm_hard_sort_words | PASS | 1 | 242(43) | 2490ms | 0 | 2490 | [detail](bonsai/llm_hard_sort_words.md) |
| llm_hard_substring | PASS | 1 | 267(43) | 2516ms | 0 | 2516 | [detail](bonsai/llm_hard_substring.md) |
| llm_hard_count_words_with_o | PASS | 1 | 703(43) | 6554ms | 0 | 6554 | [detail](bonsai/llm_hard_count_words_with_o.md) |
| llm_hard_count_vowels | FAIL | 1 | 0(0) | 30006ms | 0 | 30006 | [detail](bonsai/llm_hard_count_vowels.md) (TIMEOUT (30s)) |

## Summary

Failed: llm_hard_count_vowels

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 90% (9/10) |
| First-try success | 0/10 |
| Total turns | 10 |
| Processed tokens | 6315 |
| Cached tokens (~free) | 344 |
| Total tool calls | 0 |
| Tool errors | 0 |
| Avg tool calls/teaser | 0.0 |
| Total time | 88s |
| Avg time/teaser | 8s |
| Error recovery | N/A (no errors) |
