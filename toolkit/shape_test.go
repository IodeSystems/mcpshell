package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func TestInferType(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"primitives", `[1, "a", true, null]`, "Array<string | number | boolean | null>"},
		{"enum field", `[{s: "paid", n: 1}, {s: "open", n: 2}, {s: "paid", n: 3}, {s: "open", n: 4}]`,
			`Array<{s: "open" | "paid", n: number}>`},
		{"distinct strings stay string", `[{id: "a"}, {id: "b"}, {id: "c"}]`, "Array<{id: string}>"},
		{"one string is a string", `"hello"`, "string"},
		{"optional key", `[{a: 1, b: 2}, {a: 3}]`, "Array<{a: number, b?: number}>"},
		{"groupBy is a Record", `[{k: "x", v: 1}, {k: "y", v: 2}, {k: "z", v: 3}] |> groupBy(r => r.k)`,
			`Record<"x" | "y" | "z", Array<{k: string, v: number}>>`},
		{"countBy is a Record", `range(1, 12) |> countBy(n => "k" + str(n))`, "Record<string, number>"},
		{"few same-typed scalars stay an object", `{x: 1, y: 2, z: 3}`, "{x: number, y: number, z: number}"},
		{"non-identifier key is quoted", `{"a-b": 1}`, `{"a-b": number}`},
		{"empty", `[[], {}]`, "Array<Array<unknown> | {}>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := coreShell().Eval(c.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := runtime.InferType(v); got != c.want {
				t.Errorf("InferType(%s) = %s, want %s", c.src, got, c.want)
			}
		})
	}
	// A wide object is capped and says how many keys it left out.
	v, _ := coreShell().Eval(`range(0, 80) |> map(i => ["key_number_" + str(i), i % 2 == 0 ? 1 : "s"]) |> fromEntries`)
	if got := runtime.InferType(v); len(got) > 601 || !strings.Contains(got, "more}") {
		t.Errorf("wide object type is %d bytes: %s", len(got), got)
	}
}

// TestTruncationKeepsResults: a truncated value is kept as $rN for the next
// evals, the last KeptResults of them, and a dropped ref says which are kept.
func TestTruncationKeepsResults(t *testing.T) {
	sh := coreShell()
	for i := 1; i <= runtime.KeptResults+1; i++ {
		v, err := sh.EvalExported(`range(0, 100)`, nil)
		if err != nil {
			t.Fatal(err)
		}
		note := sh.TruncationNote(v, 10, len(v.Display()))
		if want := "$r" + string(rune('0'+i)) + ": Array<number>, length 100"; !strings.Contains(note, want) {
			t.Fatalf("note %d = %s\nwant %q", i, note, want)
		}
	}
	if v, err := sh.EvalExported(`$r6 |> sum()`, nil); err != nil || v.Display() != "4950" {
		t.Errorf("$r6 |> sum() = %v, %v", v, err)
	}
	_, err := sh.EvalExported(`$r1`, nil)
	if err == nil || !strings.Contains(err.Error(), "'$r1' is not kept") || !strings.Contains(err.Error(), "Kept now: $r2, $r3, $r4, $r5, $r6") {
		t.Errorf("$r1 after eviction: %v", err)
	}
}

func coreShell() *runtime.Shell { return toolkit.InstallCore(runtime.NewShell()) }
