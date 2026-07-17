# Project Euler (non-canonical): with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| euler_01_multiples_3_5 | ✅ | ❌ | 2/1 | 731(3772)/0(0) | 2.5/30.0s |
| euler_02_even_fibonacci | ✅ | ❌ | 2/1 | 1167(3816)/0(0) | 5.5/30.0s |
| euler_03_largest_prime_factor | ✅ | ❌ | 4/1 | 1319(8425)/0(0) | 7.2/60.0s |
| euler_04_largest_palindrome | ✅ | ❌ | 2/1 | 930(3788)/0(0) | 10.2/60.0s |
| euler_05_smallest_multiple | ✅ | ❌ | 2/1 | 1664(3773)/0(0) | 10.0/30.0s |
| euler_06_sum_square_difference | ✅ | ❌ | 11/1 | 4877(33056)/0(0) | 27.8/30.0s |
| euler_07_10001st_prime | ✅ | ❌ | 2/1 | 833(3772)/6541(43) | 8.4/60.4s |
| euler_09_pythagorean_triplet | ✅ | ❌ | 2/1 | 1290(3806)/0(0) | 9.0/60.0s |
| euler_10_sum_of_primes | ❌ | ❌ | 7/1 | 3435(14940)/4562(0) | 104.7/41.5s |
| euler_12_triangle_divisors | ❌ | ❌ | 6/1 | 3983(12301)/0(0) | 61.7/60.0s |
| euler_14_longest_collatz | ❌ | ❌ | 14/1 | 9351(49737)/0(0) | 120.0/120.0s |
| euler_21_amicable_numbers | ✅ | ❌ | 3/1 | 1677(6267)/0(0) | 22.7/60.0s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **9/12** | 0/12 |
| Total turns | 57 | 12 |
| Processed tokens | 31257 | 11103 |
| Cached tokens (~free) | 147453 | 43 |
| Total time | 390s | 642s |
