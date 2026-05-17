package bench

import "strings"

// Teaser is one benchmark challenge: a prompt for the LLM and a validator that
// inspects the final answer.
type Teaser struct {
	Name       string
	Prompt     string
	FormatHint string
	Validate   func(string) bool
	TimeoutSec int // per-teaser override; 0 uses the suite default
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
}
