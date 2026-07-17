# Project Euler: with vs. without mcpshell

**Model:** bonsai &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.

## Head-to-head (self-contained problems)

Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.

| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |
|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|
| euler_01_multiples_3_5 | ✅ | ✅ | 2/1 | 727(3772)/1482(0) | 2.5/12.7s |
| euler_02_even_fibonacci | ✅ | ❌ | 2/1 | 1003(3807)/0(0) | 4.3/30.0s |
| euler_03_largest_prime_factor | ✅ | ✅ | 2/1 | 1009(3775)/3295(0) | 5.1/29.6s |
| euler_04_largest_palindrome | ✅ | ❌ | 2/1 | 912(3779)/0(0) | 4.6/60.0s |
| euler_05_smallest_multiple | ✅ | ✅ | 2/1 | 994(3773)/2348(0) | 6.7/21.0s |
| euler_06_sum_square_difference | ✅ | ❌ | 5/1 | 1744(10879)/0(0) | 11.1/30.0s |
| euler_07_10001st_prime | ✅ | ✅ | 2/1 | 842(3773)/540(0) | 9.2/4.5s |
| euler_09_pythagorean_triplet | ✅ | ❌ | 2/1 | 999(3803)/0(0) | 5.4/60.0s |
| euler_10_sum_of_primes | ❌ | ✅ | 6/1 | 2828(11632)/821(0) | 111.9/7.4s |
| euler_12_triangle_divisors | ❌ | ❌ | 4/1 | 1984(6256)/0(0) | 74.6/60.0s |
| euler_14_longest_collatz | ❌ | ✅ | 14/1 | 9255(48841)/437(0) | 90.0/3.6s |
| euler_21_amicable_numbers | ✅ | ✅ | 2/1 | 1028(3811)/2319(0) | 8.8/20.7s |
| euler_v1_5000th_prime | ✅ | ❌ | 2/1 | 377(4228)/0(0) | 5.2/90.0s |
| euler_v2_sum_primes_1m | ✅ | ✅ | 4/1 | 1864(8678)/1482(0) | 30.0/12.9s |
| euler_v3_collatz_500k | ❌ | ❌ | 9/1 | 5644(23152)/5993(0) | 125.6/55.3s |

## Headline

| Metric (self-contained) | With mcpshell | Without |
|-------------------------|:-------------:|:-------:|
| **Solved** | **11/15** | 8/15 |
| Total turns | 60 | 15 |
| Processed tokens | 31210 | 18717 |
| Cached tokens (~free) | 143959 | 0 |
| Total time | 495s | 498s |
