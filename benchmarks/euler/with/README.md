# Benchmark Results

**Model:** bonsai
**Date:** 2026-07-17T23:13:11Z
**Score:** 11/15

| Teaser | Status | Turns | Proc(cached) | Total | Tool ms | Model ms | Details |
|--------|--------|------:|-------------:|------:|--------:|---------:|---------|
| euler_01_multiples_3_5 | PASS | 2 | 727(3772) | 2546ms | 37 | 2509 | [detail](bonsai/euler_01_multiples_3_5.md) |
| euler_02_even_fibonacci | PASS | 2 | 1003(3807) | 4346ms | 39 | 4307 | [detail](bonsai/euler_02_even_fibonacci.md) |
| euler_04_largest_palindrome | PASS | 2 | 912(3779) | 4619ms | 217 | 4402 | [detail](bonsai/euler_04_largest_palindrome.md) |
| euler_05_smallest_multiple | PASS | 2 | 994(3773) | 6740ms | 54 | 6686 | [detail](bonsai/euler_05_smallest_multiple.md) |
| euler_06_sum_square_difference | PASS | 5 | 1744(10879) | 11056ms | 51 | 11005 | [detail](bonsai/euler_06_sum_square_difference.md) |
| euler_09_pythagorean_triplet | PASS | 2 | 999(3803) | 5357ms | 936 | 4421 | [detail](bonsai/euler_09_pythagorean_triplet.md) |
| euler_07_10001st_prime | PASS | 2 | 842(3773) | 9239ms | 6412 | 2827 | [detail](bonsai/euler_07_10001st_prime.md) |
| euler_21_amicable_numbers | PASS | 2 | 1028(3811) | 8824ms | 4980 | 3844 | [detail](bonsai/euler_21_amicable_numbers.md) |
| euler_12_triangle_divisors | FAIL | 4 | 1984(6256) | 74627ms | 64495 | 10132 | [detail](bonsai/euler_12_triangle_divisors.md) (TIMEOUT (60s)) |
| euler_03_largest_prime_factor | PASS | 2 | 1009(3775) | 5140ms | 53 | 5087 | [detail](bonsai/euler_03_largest_prime_factor.md) |
| euler_10_sum_of_primes | FAIL | 6 | 2828(11632) | 111949ms | 94881 | 17068 | [detail](bonsai/euler_10_sum_of_primes.md) (TIMEOUT (90s)) |
| euler_14_longest_collatz | FAIL | 14 | 9255(48841) | 90000ms | 37373 | 52627 | [detail](bonsai/euler_14_longest_collatz.md) (TIMEOUT (90s)) |
| euler_v1_5000th_prime | PASS | 2 | 377(4228) | 5217ms | 1993 | 3224 | [detail](bonsai/euler_v1_5000th_prime.md) |
| euler_v2_sum_primes_1m | PASS | 4 | 1864(8678) | 30049ms | 22076 | 7973 | [detail](bonsai/euler_v2_sum_primes_1m.md) |
| euler_v3_collatz_500k | FAIL | 9 | 5644(23152) | 125603ms | 96453 | 29150 | [detail](bonsai/euler_v3_collatz_500k.md) (TIMEOUT (120s)) |

## Summary

Failed: euler_12_triangle_divisors, euler_10_sum_of_primes, euler_14_longest_collatz, euler_v3_collatz_500k

## Aggregate Stats

| Metric | Value |
|--------|-------|
| Pass rate | 73% (11/15) |
| First-try success | 9/15 |
| Total turns | 60 |
| Processed tokens | 31210 |
| Cached tokens (~free) | 143959 |
| Total tool calls | 45 |
| Tool errors | 33 |
| Avg tool calls/teaser | 3.0 |
| Total time | 495s |
| Avg time/teaser | 33s |
| Error recovery | 2 teaser(s) passed despite a tool error |
