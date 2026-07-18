package runtime_test

import "testing"

// TestExactNumbers pins the auto-promoting arbitrary-precision behavior: integer
// arithmetic never rounds past 2^53, decimal/rational math is exact, and
// transcendental ops fall back to float64 (the documented precision boundary).
func TestExactNumbers(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// integers auto-promote past float64's 2^53 ceiling
		{"literal past 2^53", `9007199254740993`, "9007199254740993"},
		{"2**64", `2 ** 64`, "18446744073709551616"},
		{"factorial 25 exact", `let f=1; for(let i=1;i<=25;i=i+1){f=f*i}; f`, "15511210043330985984000000"},
		{"big minus", `2 ** 100 - 1`, "1267650600228229401496703205375"},
		{"big mod", `(2 ** 100) % 7`, "2"},
		// exact decimals (the classic float footgun)
		{"0.1+0.2", `0.1 + 0.2`, "0.3"},
		{"0.1+0.2==0.3", `0.1 + 0.2 == 0.3`, "true"},
		{"terminating div", `10 / 4`, "2.5"},
		// exact rationals compose without drift
		{"thirds sum to one", `1/3 + 1/3 + 1/3`, "1"},
		{"rational display", `1 / 3`, "0.3333333333333333333333333333333333"},
		// comparisons use the exact value
		{"exact compare", `2 ** 64 > 2 ** 64 - 1`, "true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := evalDisplay(t, c.src); got != c.want {
				t.Errorf("%s\n  got  %s\n  want %s", c.src, got, c.want)
			}
		})
	}
}
