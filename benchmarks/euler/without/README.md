# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-17T22:52:54Z
**Score:** 8/15

| Teaser | Status | Turns | Tokens | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------:|------:|--------:|---------:|---------|
| euler_01_multiples_3_5 | PASS | 1 | 1482 | 12680ms | 0 | 12680 | [detail](bonsai/euler_01_multiples_3_5.md) |
| euler_02_even_fibonacci | FAIL | 1 | 0 | 30010ms | 0 | 30010 | [detail](bonsai/euler_02_even_fibonacci.md) (TIMEOUT (30s)) |
| euler_04_largest_palindrome | FAIL | 1 | 0 | 60018ms | 0 | 60018 | [detail](bonsai/euler_04_largest_palindrome.md) (TIMEOUT (60s)) |
| euler_05_smallest_multiple | PASS | 1 | 2348 | 20971ms | 0 | 20971 | [detail](bonsai/euler_05_smallest_multiple.md) |
| euler_06_sum_square_difference | FAIL | 1 | 0 | 30027ms | 0 | 30027 | [detail](bonsai/euler_06_sum_square_difference.md) (TIMEOUT (30s)) |
| euler_09_pythagorean_triplet | FAIL | 1 | 0 | 60020ms | 0 | 60020 | [detail](bonsai/euler_09_pythagorean_triplet.md) (TIMEOUT (60s)) |
| euler_07_10001st_prime | PASS | 1 | 540 | 4537ms | 0 | 4537 | [detail](bonsai/euler_07_10001st_prime.md) |
| euler_21_amicable_numbers | PASS | 1 | 2319 | 20679ms | 0 | 20679 | [detail](bonsai/euler_21_amicable_numbers.md) |
| euler_12_triangle_divisors | FAIL | 1 | 0 | 60000ms | 0 | 60000 | [detail](bonsai/euler_12_triangle_divisors.md) (TIMEOUT (60s)) |
| euler_03_largest_prime_factor | PASS | 1 | 3295 | 29648ms | 0 | 29648 | [detail](bonsai/euler_03_largest_prime_factor.md) |
| euler_10_sum_of_primes | PASS | 1 | 821 | 7368ms | 0 | 7368 | [detail](bonsai/euler_10_sum_of_primes.md) |
| euler_14_longest_collatz | PASS | 1 | 437 | 3566ms | 0 | 3566 | [detail](bonsai/euler_14_longest_collatz.md) |
| euler_v1_5000th_prime | FAIL | 1 | 0 | 90000ms | 0 | 90000 | [detail](bonsai/euler_v1_5000th_prime.md) (TIMEOUT (90s)) |
| euler_v2_sum_primes_1m | PASS | 1 | 1482 | 12906ms | 0 | 12906 | [detail](bonsai/euler_v2_sum_primes_1m.md) |
| euler_v3_collatz_500k | FAIL | 1 | 5993 | 55278ms | 0 | 55278 | [detail](bonsai/euler_v3_collatz_500k.md) |

## Summary

Failed: euler_02_even_fibonacci, euler_04_largest_palindrome, euler_06_sum_square_difference, euler_09_pythagorean_triplet, euler_12_triangle_divisors, euler_v1_5000th_prime, euler_v3_collatz_500k

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 53% (8/15) |
| First-try success | 0/15 |
| Total turns | 15 |
| Total tokens | 18717 |
| Total tool calls | 0 |
| Tool errors | 0 |
| Avg tool calls/teaser | 0.0 |
| Total time | 497s |
| Avg time/teaser | 33s |
| Error recovery | N/A (no errors) |
