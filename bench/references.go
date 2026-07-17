package bench

// Reference is a canonical single-eval mcpshell solution for a deterministic
// teaser. It documents how the problem is solved in mcpshell and lets the test
// suite verify the expected answer without an LLM (see reference_test.go).
//
// Heavy solutions are correct but take seconds to ~a minute in the tree-walking
// interpreter, so the test runs them only when MCPSHELL_BENCH_HEAVY is set.
// Ceiling solutions document the intended approach but are never executed —
// they exceed the interpreter's practical runtime, which is itself a finding
// the benchmark records.
type Reference struct {
	Name    string
	Code    string
	Heavy   bool
	Ceiling bool
}

// References gives every deterministic teaser (Project Euler + composition) a
// verified canonical solution. reference_test.go asserts this list stays in
// sync with the euler_*/compose_* teasers in Suite.
var References = []Reference{
	{Name: "euler_01_multiples_3_5",
		Code: `range(0, 1000) |> filter(n => n % 3 == 0 || n % 5 == 0) |> sum()`},
	{Name: "euler_02_even_fibonacci",
		Code: `let a = 1; let b = 2; let s = 0; while (b <= 4000000) { if (b % 2 == 0) { s = s + b } let t = a + b; a = b; b = t } s`},
	{Name: "euler_03_largest_prime_factor",
		Code: `let n = 600851475143; let mp = 1; let d = 2; while (d * d <= n) { while (n % d == 0) { mp = d; n = n / d } d = d + 1 } if (n > 1) { mp = n } mp`},
	{Name: "euler_04_largest_palindrome", Heavy: true,
		Code: `extendLimit({steps: 50000000}); let best = 0; for (let a = 100; a < 1000; a = a + 1) { for (let b = a; b < 1000; b = b + 1) { let p = a * b; let s = str(p); if (s == reverse(s) && p > best) { best = p } } } best`},
	{Name: "euler_05_smallest_multiple",
		Code: `function gcd(a, b) { while (b != 0) { let t = b; b = a % b; a = t } return a } let l = 1; for (let i = 2; i <= 20; i = i + 1) { l = l * i / gcd(l, i) } l`},
	{Name: "euler_06_sum_square_difference",
		Code: `let n = range(1, 101); sum(n) ** 2 - sum(map(n, x => x * x))`},
	{Name: "euler_07_10001st_prime", Heavy: true,
		Code: `extendLimit({steps: 50000000}); function isPrime(n) { if (n < 2) { return false } let d = 2; while (d * d <= n) { if (n % d == 0) { return false } d = d + 1 } return true } let c = 0; let n = 1; while (c < 10001) { n = n + 1; if (isPrime(n)) { c = c + 1 } } n`},
	{Name: "euler_09_pythagorean_triplet", Heavy: true,
		Code: `let r = 0; for (let a = 1; a < 1000; a = a + 1) { for (let b = a + 1; b < 1000 - a; b = b + 1) { let c = 1000 - a - b; if (a * a + b * b == c * c) { r = a * b * c } } } r`},
	{Name: "euler_10_sum_of_primes", Heavy: true,
		Code: `extendLimit({steps: 200000000, timeout: 120000}); let limit = 2000000; let sieve = fill(range(0, limit), true); let s = 0; for (let i = 2; i < limit; i = i + 1) { if (sieve[i]) { s = s + i; for (let j = i + i; j < limit; j = j + i) { sieve[j] = false } } } s`},
	{Name: "euler_12_triangle_divisors", Heavy: true,
		Code: `extendLimit({steps: 50000000}); function nDiv(n) { let c = 0; let d = 1; while (d * d <= n) { if (n % d == 0) { c = c + 2; if (d * d == n) { c = c - 1 } } d = d + 1 } return c } let n = 1; while (true) { let a = n % 2 == 0 ? n / 2 : n; let b = n % 2 == 0 ? n + 1 : (n + 1) / 2; if (nDiv(a) * nDiv(b) > 500) { break } n = n + 1 } n * (n + 1) / 2`},
	{Name: "euler_14_longest_collatz", Heavy: true,
		// Memoized chain lengths bring ~1M chains down to ~1.5min in the
		// tree-walking interpreter (the naive version exceeds several minutes).
		Code: `extendLimit({steps: 300000000, timeout: 280000}); let cache = {"1": 0}; function clen(start) { let n = start; let extra = 0; while (true) { let c = cache[str(n)]; if (c != null) { return c + extra } if (n % 2 == 0) { n = n / 2 } else { n = 3 * n + 1 } extra = extra + 1 } } let best = 0; let bestLen = 0; for (let s = 1; s < 1000000; s = s + 1) { let l = clen(s); cache[str(s)] = l; if (l > bestLen) { bestLen = l; best = s } } best`},
	{Name: "euler_21_amicable_numbers", Heavy: true,
		Code: `extendLimit({steps: 50000000}); function dsum(n) { let s = 1; let d = 2; while (d * d <= n) { if (n % d == 0) { s = s + d; if (d * d != n) { s = s + n / d } } d = d + 1 } return s } let total = 0; for (let a = 2; a < 10000; a = a + 1) { let b = dsum(a); if (b != a && dsum(b) == a) { total = total + a } } total`},

	// Non-canonical variants: same computations with perturbed parameters, so
	// the answers are not the famous Project Euler numbers a model can recall.
	{Name: "euler_v1_5000th_prime", Heavy: true,
		Code: `extendLimit({steps: 50000000}); function isPrime(n) { if (n < 2) { return false } let d = 2; while (d * d <= n) { if (n % d == 0) { return false } d = d + 1 } return true } let c = 0; let n = 1; while (c < 5000) { n = n + 1; if (isPrime(n)) { c = c + 1 } } n`},
	{Name: "euler_v2_sum_primes_1m", Heavy: true,
		Code: `extendLimit({steps: 200000000, timeout: 120000}); let limit = 1000000; let sieve = fill(range(0, limit), true); let s = 0; for (let i = 2; i < limit; i = i + 1) { if (sieve[i]) { s = s + i; for (let j = i + i; j < limit; j = j + i) { sieve[j] = false } } } s`},
	{Name: "euler_v3_collatz_500k", Heavy: true,
		Code: `extendLimit({steps: 300000000, timeout: 280000}); let cache = {"1": 0}; function clen(start) { let n = start; let extra = 0; while (true) { let c = cache[str(n)]; if (c != null) { return c + extra } if (n % 2 == 0) { n = n / 2 } else { n = 3 * n + 1 } extra = extra + 1 } } let best = 0; let bestLen = 0; for (let s = 1; s < 500000; s = s + 1) { let l = clen(s); cache[str(s)] = l; if (l > bestLen) { bestLen = l; best = s } } best`},

	{Name: "compose_core_top_region",
		Code: `[{region:"North",amt:10},{region:"South",amt:5},{region:"North",amt:7},{region:"East",amt:12},{region:"South",amt:9},{region:"North",amt:3}] |> groupBy(r => r.region) |> entries |> map(e => [e[0], e[1] |> map(r => r.amt) |> sum()]) |> sort((a,b) => b[1] - a[1]) |> at(0) |> (e => e[0] + "=" + str(e[1]))`},
	{Name: "compose_core_wordfreq_top",
		Code: `"the cat sat on the mat the cat sat" |> split(" ") |> countBy(w => w) |> entries |> sort((a,b) => b[1] - a[1]) |> at(0) |> (e => e[0] + ":" + str(e[1]))`},
	{Name: "compose_core_flatten_even_sum",
		Code: `[[1,2,[3,4]],[5,[6,7]],[8]] |> flat() |> flat() |> filter(n => n % 2 == 0) |> sum()`},
	{Name: "compose_core_csv_top_dept",
		Code: `"name,dept,sales\nalice,A,120\nbob,B,90\ncarol,A,75\ndan,B,200" |> lines() |> skip(1) |> map(l => split(l, ",")) |> groupBy(r => r[1]) |> entries |> map(e => [e[0], e[1] |> map(r => num(r[2])) |> sum()]) |> sort((a,b) => b[1] - a[1]) |> at(0) |> (e => e[0] + "=" + str(e[1]))`},
	{Name: "compose_core_pipeline_stats",
		Code: `range(1, 21) |> map(n => n * n) |> filter(n => n % 2 == 1) |> sum()`},
	{Name: "compose_sql_top_region",
		Code: `shop.query("select region, sum(qty*unit_price) rev from orders group by region order by rev desc limit 1") |> at(0) |> (r => r.region + ": " + str(r.rev))`},
	{Name: "compose_sql_top_product",
		Code: `shop.query("select product, sum(qty*unit_price) rev from orders group by product order by rev desc limit 1") |> at(0) |> (r => r.product)`},
	{Name: "compose_sql_top_month",
		Code: `shop.query("select substr(created,1,7) m, sum(qty*unit_price) rev from orders group by m order by rev desc limit 1") |> at(0) |> (r => r.m)`},
	{Name: "compose_sql_region_of_gizmo",
		Code: `shop.query("select region, product, sum(qty*unit_price) rev from orders group by region, product") |> groupBy(r => r.region) |> entries |> map(e => [e[0], e[1] |> sort((a,b) => b.rev - a.rev) |> at(0)]) |> filter(e => e[1].product == "gizmo") |> map(e => e[0]) |> join(",")`},
}
