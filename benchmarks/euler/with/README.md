# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-17T23:43:57Z
**Score:** 9/12

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| euler_01_multiples_3_5 | PASS | 2 | 731(3772) | 2550ms | 43 | 2507 | [detail](bonsai/euler_01_multiples_3_5.md) |
| euler_02_even_fibonacci | PASS | 2 | 1167(3816) | 5533ms | 60 | 5473 | [detail](bonsai/euler_02_even_fibonacci.md) |
| euler_04_largest_palindrome | PASS | 2 | 930(3788) | 10195ms | 6764 | 3431 | [detail](bonsai/euler_04_largest_palindrome.md) |
| euler_05_smallest_multiple | PASS | 2 | 1664(3773) | 9981ms | 76 | 9905 | [detail](bonsai/euler_05_smallest_multiple.md) |
| euler_06_sum_square_difference | PASS | 11 | 4877(33056) | 27836ms | 136 | 27700 | [detail](bonsai/euler_06_sum_square_difference.md) |
| euler_09_pythagorean_triplet | PASS | 2 | 1290(3806) | 8957ms | 3412 | 5545 | [detail](bonsai/euler_09_pythagorean_triplet.md) |
| euler_07_10001st_prime | PASS | 2 | 833(3772) | 8423ms | 5631 | 2792 | [detail](bonsai/euler_07_10001st_prime.md) |
| euler_21_amicable_numbers | PASS | 3 | 1677(6267) | 22666ms | 15438 | 7228 | [detail](bonsai/euler_21_amicable_numbers.md) |
| euler_12_triangle_divisors | FAIL | 6 | 3983(12301) | 61653ms | 41283 | 20370 | [detail](bonsai/euler_12_triangle_divisors.md) (TIMEOUT (60s)) |
| euler_03_largest_prime_factor | PASS | 4 | 1319(8425) | 7180ms | 439 | 6741 | [detail](bonsai/euler_03_largest_prime_factor.md) |
| euler_10_sum_of_primes | FAIL | 7 | 3435(14940) | 104674ms | 88874 | 15800 | [detail](bonsai/euler_10_sum_of_primes.md) (TIMEOUT (90s)) |
| euler_14_longest_collatz | FAIL | 14 | 9351(49737) | 120001ms | 69372 | 50629 | [detail](bonsai/euler_14_longest_collatz.md) (TIMEOUT (120s)) |

## Summary

Failed: euler_12_triangle_divisors, euler_10_sum_of_primes, euler_14_longest_collatz

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 75% (9/12) |
| First-try success | 6/12 |
| Total turns | 57 |
| Processed tokens | 31257 |
| Cached tokens (~free) | 147453 |
| Total tool calls | 45 |
| Tool errors | 29 |
| Avg tool calls/teaser | 3.8 |
| Total time | 389s |
| Avg time/teaser | 32s |
| Error recovery | 2 teaser(s) passed despite a tool error |
