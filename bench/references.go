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

// References gives every deterministic teaser (the composition suite) a
// verified canonical solution. reference_test.go asserts this list stays in
// sync with the compose_* teasers in Suite.
var References = []Reference{
	{Name: "llm_hard_count_r_strawberry",
		Code: `"strawberry" |> chars() |> filter(c => c == "r") |> len()`},
	{Name: "llm_hard_count_s_mississippi",
		Code: `"mississippi" |> chars() |> filter(c => c == "s") |> len()`},
	{Name: "llm_hard_big_multiply",
		Code: `3947 * 5821`},
	{Name: "llm_hard_digit_sum_pow",
		Code: `str(2 ** 20) |> chars() |> map(d => num(d)) |> sum()`},
	{Name: "llm_hard_last_digit_pow",
		Code: `let r = 1; for (let i = 0; i < 100; i = i + 1) { r = (r * 7) % 10 } r`},
	{Name: "llm_hard_anagram",
		Code: `("conversation" |> chars() |> sort() |> join("")) == ("conservation" |> chars() |> sort() |> join(""))`},
	{Name: "llm_hard_sort_words",
		Code: `["banana", "apple", "cherry", "date"] |> sort() |> join(",")`},
	{Name: "llm_hard_substring",
		Code: `"benchmark" |> substring(4, 7)`},
	{Name: "llm_hard_count_words_with_o",
		Code: `"the quick brown fox jumps over the lazy dog" |> split(" ") |> filter(w => w |> contains("o")) |> len()`},
	{Name: "llm_hard_count_vowels",
		Code: `"floccinaucinihilipilification" |> chars() |> filter(c => "aeiou" |> contains(c)) |> len()`},

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
