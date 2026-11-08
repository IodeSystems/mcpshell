package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func cpShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := toolkit.AddCP(toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell())))
	okEval(t, sh, `load("cp")`)
	return sh
}

func TestCPLoadAndHelp(t *testing.T) {
	sh := toolkit.AddCP(toolkit.InstallCore(runtime.NewShell()))
	mustContain(t, "before load", failEval(t, sh, `Cp.solve({})`), `load("cp")`)
	mustContain(t, "help", okEval(t, cpShell(t), `help("cp")`), "Cp.solve", "TYPICAL: Shift roster", "TYPICAL: Job shop", "TYPICAL: Assignment")
}

// The guide's three examples, as written there.
func TestCPGuideExamples(t *testing.T) {
	sh := cpShell(t)
	got := okEval(t, sh, `let nurses = ["ann", "bob", "cy"], days = [0, 1, 2]
	let v = {}
	for (const n of nurses) for (const d of days) v[n + d] = [0, 1]
	let r = Cp.solve({vars: v, constraints: [
	    ...days.map(d => ({terms: nurses |> reduce((t, n) => ({...t, [n + d]: 1}), {}), ge: 2})),
	    ...nurses.map(n => ({terms: days |> reduce((t, d) => ({...t, [n + d]: 1}), {}), le: 2})),
	    {ann0: 1, eq: 0}],
	  minimize: {bob0: 1, bob1: 1, bob2: 1}});
	[r.status, r.objective, r.values.ann0, r.values.ann1, r.values.ann2] |> join(",")`)
	if got != "optimal,2,0,1,1" {
		t.Errorf("roster: %s", got)
	}
	got = okEval(t, cpShell(t), `let ops = [{id: "a1", job: "a", m: "m1", d: 3}, {id: "a2", job: "a", m: "m2", d: 2},
	           {id: "b1", job: "b", m: "m2", d: 2}, {id: "b2", job: "b", m: "m1", d: 4}]
	let r = Cp.solve({intervals: ops |> map(o => ({id: o.id, duration: o.d})),
	  constraints: [...(ops |> groupBy(o => o.m) |> values |> map(g => ({noOverlap: g |> map(o => o.id)}))),
	    {before: "a1", after: "a2"}, {before: "b1", after: "b2"}],
	  minimize: "makespan"});
	[r.status, r.objective, r.values.makespan, r.schedule[0].id, r.schedule |> map(s => s.end) |> join("/")] |> join(",")`)
	if got != "optimal,7,7,a1,3/2/5/7" {
		t.Errorf("job shop: %s", got)
	}
	got = okEval(t, cpShell(t), `let cost = [[4, 2, 8], [4, 3, 7], [3, 1, 6]]
	let r = Cp.solve({vars: {w0: [0, 2], w1: [0, 2], w2: [0, 2], c0: [0, 9], c1: [0, 9], c2: [0, 9]},
	  constraints: [{allDifferent: ["w0", "w1", "w2"]},
	    ...[0, 1, 2].map(i => ({element: cost[i], index: "w" + i, value: "c" + i}))],
	  minimize: {c0: 1, c1: 1, c2: 1}});
	[r.status, r.objective] |> join(",")`)
	if got != "optimal,12" {
		t.Errorf("assignment: %s", got)
	}
}

func TestCPKinds(t *testing.T) {
	sh := cpShell(t)
	for src, want := range map[string]string{
		// table with object rows, count, max/min, ne, values domain
		`Cp.solve({vars: {x: [0, 5], y: [0, 5]}, constraints: [{table: ["x", "y"], rows: [{x: 1, y: 2}, {x: 3, y: 4}, {x: 5, y: 0}]}],
		  maximize: {y: 1}}).values`: `{x: 3, y: 4}`,
		`Cp.solve({vars: {a: [0, 3], b: [0, 3], c: [0, 3]}, constraints: [{count: ["a", "b", "c"], value: 2, eq: 2}, {a: 1, ne: 2}],
		  all: true}).solutions |> len`: "3", // b = c = 2, a in {0, 1, 3},
		`Cp.solve({vars: {a: [1, 4], b: [2, 6], m: [0, 9], n: [0, 9]}, constraints: [{max: ["a", "b"], eq: "m"}, {min: ["a", "b"], eq: "n"}, {m: 1, n: -1, ge: 4}],
		  minimize: {m: 1}}).values`: `{a: 1, b: 5, m: 5, n: 1}`,
		`Cp.solve({vars: {c: {values: [2, 7, 9]}}, constraints: [{c: 1, ge: 3}], minimize: "c"}).objective`: "7",
		// cumulative: three unit-demand jobs of 2 on capacity 2 take 4
		`Cp.solve({intervals: {a: {duration: 2}, b: {duration: 2}, c: {duration: 2}}, constraints: [{cumulative: ["a", "b", "c"], capacity: 2}],
		  minimize: "makespan"}).objective`: "4",
		`Cp.solve({intervals: {a: {duration: 2}, b: {duration: 2}}, constraints: [{cumulative: {a: 2, b: 1}, capacity: 2}],
		  minimize: "makespan"}).objective`: "4",
		// before with gap, start window, interval vars in a linear row
		`Cp.solve({intervals: {a: {duration: 2, start: [3, 10]}, b: {duration: 1}}, constraints: [{before: "a", after: "b", gap: 2}, {"b.end": 1, le: 20}],
		  minimize: "makespan"}).schedule |> map(s => s.id + "@" + s.start) |> join(" ")`: "a@3 b@7",
	} {
		if got := okEval(t, sh, src); got != want {
			t.Errorf("%s\n got %s\nwant %s", src, got, want)
		}
	}
}

// A 5-city tour with circuit: next_k is the position of the city after k,
// and element gives the cost of each leg. Brute force over 4! tours: 19.
func TestCPCircuitTSP(t *testing.T) {
	got := okEval(t, cpShell(t), `let d = [[0, 3, 4, 2, 7], [3, 0, 4, 6, 3], [4, 4, 0, 5, 8], [2, 6, 5, 0, 6], [7, 3, 8, 6, 0]]
	let vars = {}, cons = [{circuit: [0, 1, 2, 3, 4].map(k => "n" + k)}]
	for (const k of [0, 1, 2, 3, 4]) { vars["n" + k] = [0, 4]; vars["c" + k] = [0, 9]
	  cons.push({element: d[k], index: "n" + k, value: "c" + k}) }
	let r = Cp.solve({vars, constraints: cons, minimize: {c0: 1, c1: 1, c2: 1, c3: 1, c4: 1}});
	[r.status, r.objective] |> join(",")`)
	if got != "optimal,19" {
		t.Errorf("got %s", got)
	}
}

func TestCPStatusesAndNotes(t *testing.T) {
	sh := cpShell(t)
	got := okEval(t, sh, `let r = Cp.solve({vars: {x: [0, 3]}, constraints: [{x: 1, ge: 5}]}); [r.status, r.values, r.complete] |> join(",")`)
	if got != "infeasible,null,true" {
		t.Errorf("infeasible: %s", got)
	}
	// Satisfaction: status feasible, objective and bound null.
	mustContain(t, "satisfy", okEval(t, sh, `Cp.solve({vars: {x: [0, 3]}, constraints: [{x: 1, ge: 2}]})`),
		`status: "feasible"`, `objective: null`, `bound: null`, `values: {x: 2}`)
	// Node limit: not proven, with a note.
	sh = cpShell(t)
	got = okEval(t, sh, `let v = {}; for (let i = 0; i < 12; i++) v["x" + i] = [0, 10]
	let r = Cp.solve({vars: v, constraints: [{terms: Object.keys(v) |> reduce((t, k) => ({...t, [k]: 2}), {}), eq: 61}], limits: {nodes: 50}});
	[r.status, r.complete, r.stopped] |> join(",")`)
	if got != "unknown,false,nodes" {
		t.Errorf("node limit: %s", got)
	}
	mustContain(t, "node note", strings.Join(sh.Limits().Notes(), "\n"), "limits.nodes", "not proven")
	// Default horizon too small for a release date plus a deadline: the note.
	sh = cpShell(t)
	okEval(t, sh, `Cp.solve({intervals: {a: {duration: 2}, b: {duration: 2}}, constraints: [{before: "a", after: "b"}, {"a.start": 1, ge: 3}]}).status`)
	mustContain(t, "horizon note", strings.Join(sh.Limits().Notes(), "\n"), "default horizon", "set horizon")
}

func TestCPErrors(t *testing.T) {
	sh := cpShell(t)
	for src, want := range map[string][]string{
		`Cp.solve({vars: {x: [0, 1]}, constraints: [{x: 1, le: 1.5}]})`:              {"must be an integer", "Solver.lp", "Usage:"},
		`Cp.solve({vars: {x: [0, 1]}, constraints: [{y: 1, le: 1}]})`:                {`unknown variable "y"`, "(variables: x)"},
		`Cp.solve({intervals: {a: {duration: 1}}, constraints: [{a: 1, le: 1}]})`:    {`"a" is an interval`, `"a.start"`},
		`Cp.solve({vars: {x: [0, 1]}, constraints: [{x: 1}]})`:                       {"exactly one of le, ge, eq, ne", "allDifferent"},
		`Cp.solve({vars: {x: [0, 1]}, constraints: [{allDifferent: ["x"], le: 2}]})`: {`unknown key "le" for allDifferent`},
		`Cp.solve({vars: {x: [0, 1]}, objective: {x: 1}})`:                           {`unknown key "objective"`, "minimize"},
		`Cp.solve({vars: {x: [0, 1]}, minimize: "x", all: true})`:                    {"takes no objective"},
		`Cp.solve({vars: {x: [0, 1, 2]}})`:                                           {"[lo, hi] (a range) or {values: [...]}"},
		`Cp.solve({intervals: {a: {duration: 5, end: [0, 3]}}})`:                     {`interval "a" cannot fit`},
		`Cp.solve({intervals: {a: {dur: 5}}})`:                                       {`unknown key "dur"`, "duration"},
		`Cp.solve({vars: {x: [0, 1]}, minimize: "makespan"})`:                        {"needs intervals"},
	} {
		mustContain(t, src, failEval(t, sh, src), want...)
	}
}
