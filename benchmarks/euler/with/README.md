# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-17T20:55:38Z
**Score:** 10/12

| Teaser | Status | Tool Calls | Errors | Total | Tool ms | Model ms | Details |
|--------|--------|-----------|--------|-------|---------|----------|---------|
| euler_01_multiples_3_5 | PASS | 1 | 0 | 2509ms | 37 | 2472 | [detail](bonsai/euler_01_multiples_3_5.md) |
| euler_02_even_fibonacci | PASS | 1 | 0 | 4318ms | 39 | 4279 | [detail](bonsai/euler_02_even_fibonacci.md) |
| euler_04_largest_palindrome | PASS | 1 | 0 | 3588ms | 250 | 3338 | [detail](bonsai/euler_04_largest_palindrome.md) |
| euler_05_smallest_multiple | PASS | 1 | 0 | 4251ms | 76 | 4175 | [detail](bonsai/euler_05_smallest_multiple.md) |
| euler_06_sum_square_difference | PASS | 4 | 3 | 8736ms | 53 | 8683 | [detail](bonsai/euler_06_sum_square_difference.md) |
| euler_09_pythagorean_triplet | PASS | 1 | 0 | 4771ms | 969 | 3802 | [detail](bonsai/euler_09_pythagorean_triplet.md) |
| euler_07_10001st_prime | PASS | 1 | 0 | 9790ms | 6975 | 2815 | [detail](bonsai/euler_07_10001st_prime.md) |
| euler_21_amicable_numbers | PASS | 1 | 0 | 8907ms | 5132 | 3775 | [detail](bonsai/euler_21_amicable_numbers.md) |
| euler_12_triangle_divisors | FAIL | 3 | 3 | 73831ms | 64705 | 9126 | [detail](bonsai/euler_12_triangle_divisors.md) (TIMEOUT (60s)) |
| euler_03_largest_prime_factor | PASS | 1 | 0 | 3906ms | 54 | 3852 | [detail](bonsai/euler_03_largest_prime_factor.md) |
| euler_10_sum_of_primes | PASS | 5 | 4 | 85739ms | 69688 | 16051 | [detail](bonsai/euler_10_sum_of_primes.md) |
| euler_14_longest_collatz | FAIL | 12 | 12 | 139063ms | 98256 | 40807 | [detail](bonsai/euler_14_longest_collatz.md) (TIMEOUT (90s)) |

## Summary

Failed: euler_12_triangle_divisors, euler_14_longest_collatz

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 83% (10/12) |
| First-try success | 8/12 |
| Total tool calls | 32 |
| Tool errors | 22 |
| Avg tool calls/teaser | 2.7 |
| Total time | 349s |
| Avg time/teaser | 29s |
| Error recovery | 2 teaser(s) passed despite a tool error |
