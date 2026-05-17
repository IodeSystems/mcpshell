# Benchmark Results

**Model:** Qwen3-6-27B-MPT
**Date:** 2026-05-17T04:18:41Z
**Score:** 33/33

| Teaser | Status | Tool Calls | Errors | Duration | Details |
|--------|--------|-----------|--------|----------|---------|
| factorial | PASS | 1 | 0 | 13254ms | [detail](Qwen3-6-27B-MPT/factorial.md) |
| fizzbuzz | PASS | 1 | 0 | 1615ms | [detail](Qwen3-6-27B-MPT/fizzbuzz.md) |
| closure_counter | PASS | 1 | 0 | 1525ms | [detail](Qwen3-6-27B-MPT/closure_counter.md) |
| pipe_chain | PASS | 1 | 0 | 1252ms | [detail](Qwen3-6-27B-MPT/pipe_chain.md) |
| recursive_flatten | PASS | 1 | 0 | 1484ms | [detail](Qwen3-6-27B-MPT/recursive_flatten.md) |
| object_transform | PASS | 1 | 0 | 1275ms | [detail](Qwen3-6-27B-MPT/object_transform.md) |
| string_manipulation | PASS | 1 | 0 | 1007ms | [detail](Qwen3-6-27B-MPT/string_manipulation.md) |
| reduce_groupby | PASS | 1 | 0 | 1899ms | [detail](Qwen3-6-27B-MPT/reduce_groupby.md) |
| bitwise_flags | PASS | 1 | 0 | 1295ms | [detail](Qwen3-6-27B-MPT/bitwise_flags.md) |
| scatter_parallel | PASS | 1 | 0 | 1037ms | [detail](Qwen3-6-27B-MPT/scatter_parallel.md) |
| fibonacci_memo | PASS | 1 | 0 | 1496ms | [detail](Qwen3-6-27B-MPT/fibonacci_memo.md) |
| regex_extract | PASS | 1 | 0 | 1215ms | [detail](Qwen3-6-27B-MPT/regex_extract.md) |
| matrix_multiply | PASS | 1 | 0 | 1893ms | [detail](Qwen3-6-27B-MPT/matrix_multiply.md) |
| deep_clone | PASS | 1 | 0 | 2393ms | [detail](Qwen3-6-27B-MPT/deep_clone.md) |
| binary_search | PASS | 1 | 0 | 1924ms | [detail](Qwen3-6-27B-MPT/binary_search.md) |
| curry | PASS | 1 | 0 | 1205ms | [detail](Qwen3-6-27B-MPT/curry.md) |
| linked_list | PASS | 1 | 0 | 2050ms | [detail](Qwen3-6-27B-MPT/linked_list.md) |
| pipe_wordfreq | PASS | 2 | 1 | 1647ms | [detail](Qwen3-6-27B-MPT/pipe_wordfreq.md) |
| roman_numerals | PASS | 1 | 0 | 2407ms | [detail](Qwen3-6-27B-MPT/roman_numerals.md) |
| merge_sort | PASS | 1 | 0 | 2626ms | [detail](Qwen3-6-27B-MPT/merge_sort.md) |
| event_emitter | PASS | 2 | 1 | 3808ms | [detail](Qwen3-6-27B-MPT/event_emitter.md) |
| pipe_csv_parse | PASS | 1 | 0 | 2102ms | [detail](Qwen3-6-27B-MPT/pipe_csv_parse.md) |
| count_letter_r_strawberry | PASS | 1 | 0 | 852ms | [detail](Qwen3-6-27B-MPT/count_letter_r_strawberry.md) |
| count_letter_l_lullaby | PASS | 1 | 0 | 827ms | [detail](Qwen3-6-27B-MPT/count_letter_l_lullaby.md) |
| count_words_with_letter | PASS | 1 | 0 | 976ms | [detail](Qwen3-6-27B-MPT/count_words_with_letter.md) |
| anagram_check | PASS | 1 | 0 | 1257ms | [detail](Qwen3-6-27B-MPT/anagram_check.md) |
| nth_prime | PASS | 1 | 0 | 1870ms | [detail](Qwen3-6-27B-MPT/nth_prime.md) |
| collatz_steps | PASS | 1 | 0 | 1315ms | [detail](Qwen3-6-27B-MPT/collatz_steps.md) |
| digit_sum_power | PASS | 1 | 0 | 1166ms | [detail](Qwen3-6-27B-MPT/digit_sum_power.md) |
| longest_common_subsequence | PASS | 1 | 0 | 2791ms | [detail](Qwen3-6-27B-MPT/longest_common_subsequence.md) |
| balanced_parens | PASS | 1 | 0 | 1910ms | [detail](Qwen3-6-27B-MPT/balanced_parens.md) |
| tower_of_hanoi | PASS | 1 | 0 | 775ms | [detail](Qwen3-6-27B-MPT/tower_of_hanoi.md) |
| escape_heavy_strings | PASS | 1 | 0 | 1715ms | [detail](Qwen3-6-27B-MPT/escape_heavy_strings.md) |

## Summary

All benchmarks passed.

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 100% (33/33) |
| First-try success | 31/33 |
| Total tool calls | 35 |
| Tool errors | 2 |
| Avg tool calls/teaser | 1.1 |
| Total time | 65s |
| Avg time/teaser | 1s |
| Error recovery | 2 teaser(s) passed despite a tool error |
