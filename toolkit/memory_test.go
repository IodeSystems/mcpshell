package toolkit_test

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	rt "github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// memShell is an mcp-like shell. The memory limit counts the whole process
// heap, so tests set it as the current live heap plus room.
func memShell() *rt.Shell {
	sh := rt.NewShell()
	toolkit.InstallCore(sh)
	sh.AddLoadable(toolkit.GraphLoadable())
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	return sh
}

// room returns a memory limit with extra bytes of room above the live heap.
func room(extra int64) int64 {
	runtime.GC()
	return rt.HeapBytes() + extra
}

func evalMem(sh *rt.Shell, limit int64, code string) (rt.Value, error) {
	return sh.EvalExportedWith(code, nil, rt.EvalOptions{Limits: rt.LimitSpec{MemoryBytes: &limit}})
}

func TestMemoryLimitStopsGrowth(t *testing.T) {
	sh := memShell()
	// Strings of 10 KB: the heap passes the limit within the step interval.
	v, err := evalMem(sh, room(24<<20), `let a = []; for (let i = 0; i < 100000; i++) { a.push("x".repeat(10000) + i) }; len(a)`)
	if err == nil {
		t.Fatalf("want a memory error, got %v", v)
	}
	for _, want := range []string{"memory limit exceeded:", "live heap", "this eval:", "extendLimit({memory:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestMemoryLimitStopsDoubling(t *testing.T) {
	sh := memShell()
	// concat doubles in a few steps: the result check catches it.
	_, err := evalMem(sh, room(32<<20), `let a = range(50000); while (true) { a = a.concat(a) }`)
	if err == nil || !strings.Contains(err.Error(), "memory limit exceeded") {
		t.Fatalf("want a memory error, got %v", err)
	}
	_, err = evalMem(sh, room(32<<20), `let s = "x".repeat(1000000); while (true) { s = s + s }`)
	if err == nil || !strings.Contains(err.Error(), "string concatenation needs") {
		t.Fatalf("string doubling: %v", err)
	}
	_, err = evalMem(sh, room(32<<20), `let a = range(100000); while (true) { a = [...a, ...a] }`)
	if err == nil || !strings.Contains(err.Error(), "array spread needs") {
		t.Fatalf("spread doubling: %v", err)
	}
}

func TestNativeAllocatorsRefusedBeforeAllocating(t *testing.T) {
	sh := memShell()
	limit := room(64 << 20)
	for _, tc := range []struct{ code, want string }{
		{`range(1e10)`, "range(0, 10000000000) needs"},
		{`range(5, 1e12)`, "range(5, 1000000000000) needs"},
		{`"ab".repeat(1e10)`, "repeat(10000000000) needs"},
		{`"ab".padStart(1e11, "0")`, "padStart(100000000000) needs"},
		{`toArray({length: 1e10})`, "toArray({length: 10000000000}) needs"},
		{`Array(1e10)`, "Array(10000000000) needs"},
	} {
		before := rt.HeapBytes()
		_, err := evalMem(sh, limit, tc.code)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want %q, got %v", tc.code, tc.want, err)
			continue
		}
		if grown := rt.HeapBytes() - before; grown > 32<<20 {
			t.Errorf("%s: heap grew by %d bytes: the guard ran after allocating", tc.code, grown)
		}
	}
	// Small ones still work.
	if v, err := evalMem(sh, limit, `len(range(1000)) + len("ab".repeat(1000))`); err != nil || v.Display() != "3000" {
		t.Errorf("small allocators: %v %v", v, err)
	}
}

func TestMemoryGCBeforeFailAvoidsFalsePositive(t *testing.T) {
	// With the collector off, garbage piles up past the limit. The check must
	// collect and measure again before it fails the eval.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	sh := memShell()
	limit := room(48 << 20)
	v, err := evalMem(sh, limit, `let n = 0; for (let i = 0; i < 40; i++) { let s = "z".repeat(4000000); n += len(s) }; n`)
	if err != nil || v.Display() != "160000000" {
		t.Fatalf("garbage must not fail the eval: %v %v", v, err)
	}
	// The step check too: small garbage over many steps.
	v, err = evalMem(sh, room(8<<20), `let n = 0; for (let i = 0; i < 30000; i++) { let s = "q".padEnd(4000, "w"); n += len(s) }; n`)
	if err != nil || v.Display() != "120000000" {
		t.Fatalf("garbage in the step check: %v %v", v, err)
	}
}

func TestMemoryReportNamesExportsAndKeptResults(t *testing.T) {
	sh := memShell()
	big := room(40 << 20)
	if _, err := evalMem(sh, big, `export let rows = range(60000) |> map(i => ({id: i, name: "row" + i}))`); err != nil {
		t.Fatal(err)
	}
	kept := &rt.ArrayVal{}
	for i := 0; i < 50000; i++ {
		kept.Elements = append(kept.Elements, rt.Str(strings.Repeat("k", 100)))
	}
	sh.TruncationNote(kept, 10, 99)
	_, err := evalMem(sh, room(8<<20), `"y".repeat(20000000).length`)
	if err == nil {
		t.Fatal("want a memory error")
	}
	msg := err.Error()
	for _, want := range []string{
		`export rows: ~`, `export rows = null`,
		`$r1: ~`, `kept result; Array, 50k elements`, `forget("$r1")`,
		"this eval: the rest",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("report lacks %q:\n%s", want, msg)
		}
	}
}

func TestMemoryReportNamesToolkits(t *testing.T) {
	sh := memShell()
	if _, err := sh.Eval(`load("graph", "db")`); err != nil {
		t.Fatal(err)
	}
	if _, err := sh.Eval(`for (let i = 0; i < 3000; i++) { addNode(root(), "person", {name: "n" + i, bio: "x".repeat(200)}) }`); err != nil {
		t.Fatal(err)
	}
	rows := `range(20000) |> map(i => ({id: i, text: "t".repeat(200) + i}))`
	if _, err := sh.Eval(`Db.load("events", ` + rows + `)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sh.Eval(`Db.load("small", [{a: 1}])`); err != nil {
		t.Fatal(err)
	}
	_, err := evalMem(sh, room(8<<20), `"y".repeat(20000000).length`)
	if err == nil {
		t.Fatal("want a memory error")
	}
	msg := err.Error()
	for _, want := range []string{
		`graph: `, `3.0k nodes`, `unload("graph")`,
		`db: `, `off-heap`, `2 tables; largest "events"`, `Db.drop("events") or unload("db")`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("report lacks %q:\n%s", want, msg)
		}
	}
	// Unloaded, they are gone from the report.
	if _, err := sh.Eval(`unload("db", "graph")`); err != nil {
		t.Fatal(err)
	}
	_, err = evalMem(sh, room(8<<20), `"y".repeat(20000000).length`)
	if err == nil || strings.Contains(err.Error(), "db: ") || strings.Contains(err.Error(), "graph: ") {
		t.Errorf("an unloaded toolkit must leave the report: %v", err)
	}
}

func TestMemoryNearLimitNote(t *testing.T) {
	sh := memShell()
	limit := room(40 << 20)
	if _, err := evalMem(sh, limit, `export let s = "x".repeat(36000000)`); err != nil {
		t.Fatal(err)
	}
	var note string
	for _, n := range sh.Diagnostics().Notes {
		if strings.HasPrefix(n, "memory ") {
			note = n
		}
	}
	if note == "" {
		t.Fatalf("want a memory note at >= 80%%, notes: %q", sh.Diagnostics().Notes)
	}
	for _, want := range []string{"live heap (", "export s: ~", "export s = null"} {
		if !strings.Contains(note, want) {
			t.Errorf("note lacks %q:\n%s", want, note)
		}
	}
	if !strings.Contains(sh.Diagnostics().Format(), "\nnote: memory ") {
		t.Errorf("the block lacks the note:\n%s", sh.Diagnostics().Format())
	}
	// A small eval under a roomy limit reports nothing.
	if _, err := sh.Eval(`1`); err != nil {
		t.Fatal(err)
	}
	for _, n := range sh.Diagnostics().Notes {
		if strings.HasPrefix(n, "memory ") {
			t.Errorf("the default limit is far away, but got a note: %s", n)
		}
	}
	// Dropping the export frees it: the note is gone under the same limit.
	if _, err := evalMem(sh, limit, `export let s = null`); err != nil {
		t.Fatal(err)
	}
	for _, n := range sh.Diagnostics().Notes {
		if strings.HasPrefix(n, "memory ") {
			t.Errorf("after the export was cleared: %s", n)
		}
	}
}

func TestMemoryLimitResetsPerEval(t *testing.T) {
	sh := memShell()
	lim := int64(64 << 20)
	v, err := sh.EvalWith(`limits().maxMemory`, rt.EvalOptions{Limits: rt.LimitSpec{MemoryBytes: &lim}})
	if err != nil || v.Display() != fmt.Sprint(lim) {
		t.Fatalf("limit not applied: %v %v", v, err)
	}
	v, err = sh.Eval(`limits().maxMemory`)
	if err != nil || v.Display() != fmt.Sprint(rt.DefaultMaxMemory) {
		t.Errorf("the next eval must have the default limit: %v %v", v, err)
	}
}

func TestForget(t *testing.T) {
	sh := memShell()
	arr := &rt.ArrayVal{}
	for i := 0; i < 1000; i++ {
		arr.Elements = append(arr.Elements, rt.Num(float64(i)))
	}
	sh.TruncationNote(arr, 5, 50)
	sh.TruncationNote(arr, 5, 50)
	v, err := sh.Eval(`forget("$r1")`)
	if err != nil || !strings.HasPrefix(v.Display(), "Forgot $r1 (~") {
		t.Fatalf("forget: %v %v", v, err)
	}
	if names := sh.KeptResultNames(); len(names) != 1 || names[0] != "$r2" {
		t.Errorf("kept = %v", names)
	}
	if _, err := sh.Eval(`$r1`); err == nil {
		t.Error("$r1 must be gone")
	}
	if v, err := sh.Eval(`len($r2)`); err != nil || v.Display() != "1000" {
		t.Errorf("$r2 must stay: %v %v", v, err)
	}
	_, err = sh.Eval(`forget("$r1")`)
	if err == nil || !strings.Contains(err.Error(), "not a kept result") || !strings.Contains(err.Error(), "$r2") {
		t.Errorf("forget of a gone result: %v", err)
	}
	if _, err := sh.Eval(`forget("rows")`); err == nil {
		t.Error("forget must only drop kept results")
	}
}

func TestExtendLimitMemory(t *testing.T) {
	sh := memShell()
	v, err := sh.Eval(`extendLimit({memory: "1500MB"}); limits().maxMemory`)
	if err != nil || v.Display() != fmt.Sprint(1500<<20) {
		t.Errorf("extendLimit memory: %v %v", v, err)
	}
	if _, err := sh.Eval(`extendLimit({memory: "3GB"})`); err == nil || !strings.Contains(err.Error(), "operator maximum") {
		t.Errorf("over the cap: %v", err)
	}
}
