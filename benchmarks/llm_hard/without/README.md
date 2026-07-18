# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-18T04:36:43Z
**Score:** 9/10

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| llm_hard_count_r_strawberry | PASS | 1 | 263(0) | 2361ms | 0 | 2361 | [detail](bonsai/llm_hard_count_r_strawberry.md) |
| llm_hard_count_s_mississippi | PASS | 1 | 745(43) | 6843ms | 0 | 6843 | [detail](bonsai/llm_hard_count_s_mississippi.md) |
| llm_hard_big_multiply | PASS | 1 | 1566(43) | 14304ms | 0 | 14304 | [detail](bonsai/llm_hard_big_multiply.md) |
| llm_hard_digit_sum_pow | PASS | 1 | 775(43) | 7262ms | 0 | 7262 | [detail](bonsai/llm_hard_digit_sum_pow.md) |
| llm_hard_last_digit_pow | PASS | 1 | 618(43) | 5781ms | 0 | 5781 | [detail](bonsai/llm_hard_last_digit_pow.md) |
| llm_hard_anagram | PASS | 1 | 1136(43) | 10479ms | 0 | 10479 | [detail](bonsai/llm_hard_anagram.md) |
| llm_hard_sort_words | PASS | 1 | 242(43) | 2523ms | 0 | 2523 | [detail](bonsai/llm_hard_sort_words.md) |
| llm_hard_substring | PASS | 1 | 267(43) | 2615ms | 0 | 2615 | [detail](bonsai/llm_hard_substring.md) |
| llm_hard_count_words_with_o | PASS | 1 | 703(43) | 6492ms | 0 | 6492 | [detail](bonsai/llm_hard_count_words_with_o.md) |
| llm_hard_count_vowels | FAIL | 1 | 0(0) | 30008ms | 0 | 30008 | [detail](bonsai/llm_hard_count_vowels.md) (TIMEOUT (30s)) |

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
