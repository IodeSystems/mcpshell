package bench

import "strings"

// Teaser is one benchmark challenge: a prompt for the LLM and a validator that
// inspects the final answer.
type Teaser struct {
	Name       string
	Prompt     string
	FormatHint string
	Validate   func(string) bool
	TimeoutSec int  // per-teaser override; 0 uses the suite default
	ToolOnly   bool // needs data/state only the tool provides; excluded from the no-tool baseline
}

// all builds a validator that requires every substring to be present.
func all(subs ...string) func(string) bool {
	return func(s string) bool {
		for _, sub := range subs {
			if !strings.Contains(s, sub) {
				return false
			}
		}
		return true
	}
}

func has(s, sub string) bool { return strings.Contains(s, sub) }

// Suite is the benchmark challenge set.
var Suite = []Teaser{
	{
		Name:       "factorial",
		Prompt:     "Compute 7 factorial (7!) using mcpshell.",
		FormatHint: "Use the mcpshell tool. Return just the number.",
		Validate:   all("5040"),
	},
	{
		Name:       "fizzbuzz",
		Prompt:     `Using mcpshell, generate FizzBuzz for 1-15: multiples of 3→"Fizz", 5→"Buzz", both→"FizzBuzz", else the number as string.`,
		FormatHint: "Use the mcpshell tool. Return as an array.",
		Validate:   all("FizzBuzz", "Buzz", "Fizz"),
	},
	{
		Name:       "closure_counter",
		Prompt:     "In mcpshell, create a counter using closures: a function that returns an object with increment() and get() methods. Call increment 5 times, return get().",
		FormatHint: "Use the mcpshell tool. Return just the number.",
		Validate:   all("5"),
	},
	{
		Name:       "pipe_chain",
		Prompt:     "In mcpshell, take the array [5,3,8,1,9,2,7,4,6] and use pipes (|>) to: sort it, take the largest 3, double each, then sum them.",
		FormatHint: "Use the mcpshell tool. Return just the sum as a number.",
		Validate:   all("48"),
	},
	{
		Name:       "recursive_flatten",
		Prompt:     "In mcpshell, write a recursive function that flattens a nested array like [[1,[2]],[[3,4],[5]]] into [1,2,3,4,5].",
		FormatHint: "Use the mcpshell tool. Return the flattened array.",
		Validate:   all("[1, 2, 3, 4, 5]"),
	},
	{
		Name:       "object_transform",
		Prompt:     `In mcpshell, given the array [{name:"alice",score:85},{name:"bob",score:92},{name:"carol",score:78}], use pipes to filter scores > 80, extract names, sort them, and join with commas.`,
		FormatHint: "Use the mcpshell tool. Return the joined string.",
		Validate:   func(s string) bool { return has(s, "alice") && has(s, "bob") && !has(s, "carol") },
	},
	{
		Name:       "string_manipulation",
		Prompt:     `In mcpshell, take the string "hello world foo bar" and: split by spaces, reverse the array of words, uppercase each word, join with "-".`,
		FormatHint: "Use the mcpshell tool. Return the resulting string.",
		Validate: func(s string) bool {
			return has(s, "BAR-FOO-WORLD-HELLO") || has(s, "BAR - FOO - WORLD - HELLO")
		},
	},
	{
		Name:       "reduce_groupby",
		Prompt:     `In mcpshell, given [{type:"fruit",name:"apple"},{type:"veg",name:"carrot"},{type:"fruit",name:"banana"},{type:"veg",name:"pea"}], group by type into an object like {fruit:[...],veg:[...]}. Use reduce.`,
		FormatHint: "Use the mcpshell tool. Return the grouped object.",
		Validate:   all("apple", "banana", "carrot", "pea"),
	},
	{
		Name:       "bitwise_flags",
		Prompt:     "In mcpshell, define permission flags: READ=4, WRITE=2, EXEC=1. Combine READ+WRITE into a variable (addition for non-overlapping flags). Check if it has WRITE (using & and !== 0), check if it has EXEC.",
		FormatHint: "Use the mcpshell tool. Return an object with boolean values: {hasWrite: true/false, hasExec: true/false}.",
		Validate:   all("hasWrite", "true", "hasExec", "false"),
	},
	{
		Name:       "scatter_parallel",
		Prompt:     "In mcpshell, use the scatter pipe |* to square each element in [1,2,3,4,5] in parallel, then reduce to sum them.",
		FormatHint: "Use the mcpshell tool. Return just the sum.",
		Validate:   all("55"),
	},
	{
		Name:       "fibonacci_memo",
		Prompt:     "In mcpshell, implement fibonacci (fib(0)=0, fib(1)=1) with memoization using an object as cache. Compute fib(20).",
		FormatHint: "Use the mcpshell tool. Return just the number.",
		Validate:   all("6765"),
		TimeoutSec: 60,
	},
	{
		Name:       "regex_extract",
		Prompt:     `In mcpshell, extract all email-like patterns from the string "contact alice@example.com or bob@test.org for info". Use match() with a regex.`,
		FormatHint: "Use the mcpshell tool. Return the array of matches.",
		Validate:   all("alice@example.com", "bob@test.org"),
	},
	{
		Name:       "matrix_multiply",
		Prompt:     "In mcpshell, multiply two 2x2 matrices: A=[[1,2],[3,4]] and B=[[5,6],[7,8]].",
		FormatHint: "Use the mcpshell tool. Return the result as a 2D array.",
		Validate:   all("19", "22", "43", "50"),
	},
	{
		Name:       "deep_clone",
		Prompt:     "In mcpshell, write a function that deep-clones a nested object. Clone {a:1,b:{c:2,d:[3,4]}} and modify the clone's b.c to 99. Return both original and clone to prove they are independent.",
		FormatHint: "Use the mcpshell tool. Return an array or object showing both values.",
		Validate:   all("99", "c: 2"),
		TimeoutSec: 60,
	},
	{
		Name:       "binary_search",
		Prompt:     "In mcpshell, implement binary search on a sorted array. Search for 7 in [1,3,5,7,9,11,13,15].",
		FormatHint: "Use the mcpshell tool. Return the index as a number.",
		Validate:   all("3"),
		TimeoutSec: 60,
	},
	{
		Name:       "curry",
		Prompt:     "In mcpshell, write a function that curries a two-argument function. Create a curried add, then use it: let add5 = curriedAdd(5); return add5(3).",
		FormatHint: "Use the mcpshell tool. Return just the number.",
		Validate:   all("8"),
	},
	{
		Name:       "linked_list",
		Prompt:     "In mcpshell, implement a singly linked list using nested objects {value, next}. Build a list of [10, 20, 30], then write a function to convert it to an array.",
		FormatHint: "Use the mcpshell tool. Return the array.",
		Validate:   all("10", "20", "30"),
		TimeoutSec: 60,
	},
	{
		Name:       "pipe_wordfreq",
		Prompt:     `In mcpshell, take the string "the cat sat on the mat the cat" and use pipes to: split by spaces, count word frequencies into an object.`,
		FormatHint: "Use the mcpshell tool. Return the frequency object.",
		Validate:   all("the", "3", "cat", "2"),
		TimeoutSec: 60,
	},
	{
		Name:       "roman_numerals",
		Prompt:     "In mcpshell, write a function that converts an integer to a Roman numeral string. Convert 3749 and 2867, return them joined with a comma.",
		FormatHint: "Use the mcpshell tool. Return the two Roman numeral strings joined with a comma.",
		Validate:   all("MMMDCCXLIX", "MMDCCCLXVII"),
	},
	{
		Name:       "merge_sort",
		Prompt:     "In mcpshell, implement merge sort. Sort the array [38, 27, 43, 3, 9, 82, 10].",
		FormatHint: "Use the mcpshell tool. Return the sorted array.",
		Validate: func(s string) bool {
			return has(s, "[3, 9, 10, 27, 38, 43, 82]") ||
				all("3", "9", "10", "27", "38", "43", "82")(s)
		},
	},
	{
		Name:       "event_emitter",
		Prompt:     "In mcpshell, implement a simple event emitter with on(event, handler) and emit(event, data) methods. Register two handlers for 'data' event: one that returns data as-is, one that returns data * 2. Emit with value 42, collect all handler results into an array.",
		FormatHint: "Use the mcpshell tool. Return the array of results.",
		Validate:   all("42", "84"),
		TimeoutSec: 60,
	},
	{
		Name:       "pipe_csv_parse",
		Prompt:     `In mcpshell, parse this CSV string into an array of objects: "name,age,city\nalice,30,nyc\nbob,25,sf\ncarol,35,la". First row is headers.`,
		FormatHint: "Use the mcpshell tool. Return the array of objects.",
		Validate:   all("alice", "30", "nyc", "bob", "carol"),
		TimeoutSec: 60,
	},
	{
		Name:       "count_letter_r_strawberry",
		Prompt:     "Using mcpshell, count the number of times the letter 'r' appears in the word 'strawberry'.",
		FormatHint: "Use the mcpshell tool. Return just the count as a number.",
		Validate:   all("3"),
	},
	{
		Name:       "count_letter_l_lullaby",
		Prompt:     "Using mcpshell, count the number of times the letter 'l' appears in the word 'lullaby'.",
		FormatHint: "Use the mcpshell tool. Return just the count as a number.",
		Validate:   all("3"),
	},
	{
		Name:       "count_words_with_letter",
		Prompt:     "Using mcpshell, take the sentence 'the quick brown fox jumps over the lazy dog' and count the number of words that contain the letter 'o'.",
		FormatHint: "Use the mcpshell tool. Return just the count as a number.",
		Validate:   all("4"),
	},
	{
		Name:       "anagram_check",
		Prompt:     "Using mcpshell, write a function that checks if two words are anagrams. Test it with 'listen' and 'silent'.",
		FormatHint: "Use the mcpshell tool. Return true or false.",
		Validate:   all("true"),
	},
	{
		Name:       "nth_prime",
		Prompt:     "Using mcpshell, find the 50th prime number.",
		FormatHint: "Use the mcpshell tool. Return just the number.",
		Validate:   all("229"),
	},
	{
		Name:       "collatz_steps",
		Prompt:     "Using mcpshell, compute the number of steps in the Collatz sequence starting from 27 until it reaches 1. (If n is even, n/2; if odd, 3n+1).",
		FormatHint: "Use the mcpshell tool. Return just the step count.",
		Validate:   all("111"),
	},
	{
		Name:       "digit_sum_power",
		Prompt:     "Using mcpshell, compute 2 to the power of 15 (use ** operator) and then sum all the digits of the result.",
		FormatHint: "Use the mcpshell tool. Return just the digit sum.",
		Validate:   all("26"),
	},
	{
		Name:       "longest_common_subsequence",
		Prompt:     "Using mcpshell, find the length of the longest common subsequence of 'ABCBDAB' and 'BDCAB'.",
		FormatHint: "Use the mcpshell tool. Return just the length.",
		Validate:   all("4"),
		TimeoutSec: 60,
	},
	{
		Name:       "balanced_parens",
		Prompt:     "Using mcpshell, write a function that checks if a string of parentheses is balanced. Test with '((())())' and '((()'. Return an object {test1: true/false, test2: true/false}.",
		FormatHint: "Use the mcpshell tool. Return an object with boolean values.",
		Validate:   all("true", "false"),
	},
	{
		Name:       "tower_of_hanoi",
		Prompt:     "Using mcpshell, compute the minimum number of moves to solve Tower of Hanoi with 10 disks (formula: 2**n - 1).",
		FormatHint: "Use the mcpshell tool. Return just the number.",
		Validate:   all("1023"),
	},
	{
		Name: "escape_heavy_strings",
		Prompt: `Using mcpshell, given these three Windows file paths:
- C:\Users\admin\Documents\report_2024.csv
- D:\Projects\src\main\dist\build_log.txt
- E:\backup\db\prod_dump_2024-01-15.sql
Extract just the filename (after the last backslash) from each path using the regex pattern \\([^\\]+)$ to capture the filename part (including the leading backslash), then strip the leading backslash. Join the filenames with " | ".`,
		FormatHint: "Use the mcpshell tool. Return the joined string.",
		Validate:   all("report_2024.csv", "build_log.txt", "prod_dump_2024-01-15.sql", "|"),
		TimeoutSec: 60,
	},

	// --- Project Euler (non-canonical) --------------------------------------
	// Euler-style problems with perturbed constants so the answers are NOT the
	// famous, memorized Project Euler numbers — the reasoning-only baseline must
	// actually compute, not recall. Neutrally phrased (no mention of mcpshell) so
	// the same prompt is fair with and without the tool. Answers are exact
	// integers within float64's safe range; the heavier ones can exceed the
	// default 1M-step budget, so the model must call extendLimit({steps}).
	{
		Name:       "euler_01_multiples_3_5",
		Prompt:     "Find the sum of all the multiples of 3 or 5 below 1379.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("443670"),
	},
	{
		Name:       "euler_02_even_fibonacci",
		Prompt:     "In the Fibonacci sequence 1, 2, 3, 5, 8, 13, 21, ... each term is the sum of the previous two. Find the sum of the even-valued terms whose value does not exceed 20,000,000.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("19544084"),
	},
	{
		Name:       "euler_04_largest_palindrome",
		Prompt:     "A palindromic number reads the same both ways. Find the largest palindrome that is the product of two integers each between 100 and 900 inclusive.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("749947"),
		TimeoutSec: 60,
	},
	{
		Name:       "euler_05_smallest_multiple",
		Prompt:     "What is the smallest positive number that is evenly divisible by every integer from 1 to 23?",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("5354228880"),
	},
	{
		Name:       "euler_06_sum_square_difference",
		Prompt:     "Find the difference between the square of the sum of the first 137 natural numbers (1 to 137) and the sum of their squares.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("88492684"),
	},
	{
		Name:       "euler_09_pythagorean_triplet",
		Prompt:     "Consider all Pythagorean triplets of positive integers a < b < c (with a^2 + b^2 = c^2) for which a + b + c = 1716. Among them, find the largest product a*b*c.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("175452420"),
		TimeoutSec: 60,
	},
	{
		Name:       "euler_07_10001st_prime",
		Prompt:     "What is the 9001st prime number? (2 is the 1st.)",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("93187"),
		TimeoutSec: 90,
	},
	{
		Name:       "euler_21_amicable_numbers",
		Prompt:     "Let d(n) be the sum of the proper divisors of n (divisors less than n). Two distinct numbers a and b are amicable if d(a) = b and d(b) = a. Find the sum of all amicable numbers below 15000.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("80106"),
		TimeoutSec: 60,
	},
	{
		Name:       "euler_12_triangle_divisors",
		Prompt:     "The triangle numbers are 1, 3, 6, 10, 15, ...; the nth triangle number is the sum of the integers 1 to n. What is the value of the first triangle number to have more than 420 divisors?",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("17907120"),
		TimeoutSec: 60,
	},
	{
		Name:       "euler_03_largest_prime_factor",
		Prompt:     "What is the largest prime factor of the number 822999948151?",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("999999937"),
		TimeoutSec: 60,
	},
	{
		Name:       "euler_10_sum_of_primes",
		Prompt:     "Find the sum of all the prime numbers below 1,500,000.",
		FormatHint: "Return ONLY the final number, with no commas, text, or explanation.",
		Validate:   all("82074443256"),
		TimeoutSec: 90,
	},
	{
		Name:       "euler_14_longest_collatz",
		Prompt:     "The Collatz iteration sends n to n/2 when n is even and to 3n+1 when n is odd, stopping at 1. Which starting number below 700,000 produces the longest chain?",
		FormatHint: "Return ONLY the starting number, with no commas, text, or explanation.",
		Validate:   all("626331"),
		TimeoutSec: 120,
	},

	// --- Composition -------------------------------------------------------
	// These reward composing a whole pipeline into ONE eval rather than
	// round-tripping through several tool calls. The core+math ones carry
	// their data inline (fair with and without the tool); the SQL ones query
	// the seeded `shop` fixture (meaningful only with the tool — the model
	// cannot know the rows otherwise). Watch the tool-call count: a good
	// composer solves each in a single call.
	{
		Name:       "compose_core_top_region",
		Prompt:     `Given [{region:"North",amt:10},{region:"South",amt:5},{region:"North",amt:7},{region:"East",amt:12},{region:"South",amt:9},{region:"North",amt:3}], total amt by region and return the region with the highest total and its total, formatted as region=total.`,
		FormatHint: "Return ONLY the final value (e.g. Foo=42), no explanation.",
		Validate:   all("North", "20"),
	},
	{
		Name:       "compose_core_wordfreq_top",
		Prompt:     `In the string "the cat sat on the mat the cat sat", find the most frequent word and its count, formatted as word:count.`,
		FormatHint: "Return ONLY the final value (e.g. foo:5), no explanation.",
		Validate:   all("the", "3"),
	},
	{
		Name:       "compose_core_flatten_even_sum",
		Prompt:     "Flatten the nested array [[1,2,[3,4]],[5,[6,7]],[8]] completely, keep only the even numbers, and return their sum.",
		FormatHint: "Return ONLY the final number, no explanation.",
		Validate:   all("20"),
	},
	{
		Name:       "compose_core_csv_top_dept",
		Prompt:     "Parse this CSV (first row is headers): \"name,dept,sales\\nalice,A,120\\nbob,B,90\\ncarol,A,75\\ndan,B,200\". Sum sales per dept and return the dept with the highest total and that total, formatted as dept=total.",
		FormatHint: "Return ONLY the final value (e.g. X=999), no explanation.",
		Validate:   all("B", "290"),
	},
	{
		Name:       "compose_core_pipeline_stats",
		Prompt:     "Take the integers 1 to 20, square each, keep only the odd squares, and return their sum.",
		FormatHint: "Return ONLY the final number, no explanation.",
		Validate:   all("1330"),
	},
	{
		Name:       "compose_sql_top_region",
		Prompt:     "A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. Find the region with the highest total revenue and return it as region: revenue.",
		FormatHint: "Return ONLY the final value (e.g. Foo: 123), no explanation.",
		Validate:   all("West", "278"),
		TimeoutSec: 60,
		ToolOnly:   true,
	},
	{
		Name:       "compose_sql_top_product",
		Prompt:     "A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. Which product has the highest total revenue across all orders?",
		FormatHint: "Return ONLY the product name, no explanation.",
		Validate:   all("gadget"),
		TimeoutSec: 60,
		ToolOnly:   true,
	},
	{
		Name:       "compose_sql_top_month",
		Prompt:     "A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created); created is an ISO date like 2024-03-09. A row's revenue is qty*unit_price. Which calendar month (YYYY-MM) has the highest total revenue?",
		FormatHint: "Return ONLY the month as YYYY-MM, no explanation.",
		Validate:   all("2024-03"),
		TimeoutSec: 60,
		ToolOnly:   true,
	},
	{
		Name:       "compose_sql_region_of_gizmo",
		Prompt:     "A SQLite database is attached as the `shop` namespace with table orders(id, region, product, qty, unit_price, created). A row's revenue is qty*unit_price. For each region, find its single highest-revenue product. Which region's top product is 'gizmo'?",
		FormatHint: "Return ONLY the region name, no explanation.",
		Validate:   all("West"),
		TimeoutSec: 60,
		ToolOnly:   true,
	},
}
