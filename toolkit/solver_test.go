package toolkit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func solverShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := toolkit.AddSolver(toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell())))
	okEval(t, sh, `load("solver")`)
	return sh
}

func TestSolverLoadHint(t *testing.T) {
	sh := toolkit.AddSolver(toolkit.InstallCore(runtime.NewShell()))
	mustContain(t, "call before load", failEval(t, sh, `Solver.lp({})`), `load("solver")`)
	mustContain(t, "help before load", okEval(t, solverShell(t), `help("solver")`), "Solver.lp", "TYPICAL: Product mix", "TYPICAL: Knapsack", "TYPICAL: Transport")
}

func TestSolverProductMix(t *testing.T) {
	// Dense form. Optimum 36 at x=2, y=6 (solver.TestDantzig).
	got := okEval(t, solverShell(t), `let r = Solver.lp({maximize: {x: 3, y: 5},
	  subjectTo: [{x: 1, le: 4}, {y: 2, le: 12}, {x: 3, y: 2, le: 18, name: "mix"}]});
	[r.status, r.objective, r.values.x, r.values.y, r.constraints[2].name, r.constraints[2].active, r.constraints[0].slack, r.constraints[0].active] |> join(",")`)
	if got != "optimal,36,2,6,mix,true,2,false" {
		t.Errorf("got %s", got)
	}
	// Same model, row form.
	got = okEval(t, solverShell(t), `let r = Solver.lp({sense: "max",
	  vars: [{id: "x", obj: 3}, {id: "y", obj: 5}],
	  coefs: [{con: "a", var: "x", coef: 1}, {con: "b", var: "y", coef: 2}, {con: "c", var: "x", coef: 3}, {con: "c", var: "y", coef: 2}],
	  rhs: [{con: "a", le: 4}, {con: "b", le: 12}, {con: "c", le: 18}]});
	[r.status, r.objective, r.vars[0].id, r.vars[0].value, r.vars[1].value] |> join(",")`)
	if got != "optimal,36,x,2,6" {
		t.Errorf("got %s", got)
	}
}

func TestSolverExactFractions(t *testing.T) {
	// solver.TestFractionalOptimum: x=9/7, y=11/7, obj 20/7.
	src := `let r = Solver.lp({maximize: {x: 1, y: 1}, subjectTo: [{x: 3, y: 2, le: 7}, {x: 1, y: 3, le: 6}]});`
	if got := okEval(t, solverShell(t), src+`[r.objective == 20/7, r.values.x == 9/7, r.values.y == 11/7, r.objective == 2.857142857142857] |> join(",")`); got != "true,true,true,false" {
		t.Errorf("exactness: %s", got)
	}
	if got := okEval(t, solverShell(t), src+`[r.fractions.objective, r.fractions.values.x, r.fractions.values.y] |> join(",")`); got != "20/7,9/7,11/7" {
		t.Errorf("fractions: %s", got)
	}
	// An integer answer has no fractions key.
	if got := okEval(t, solverShell(t), `Solver.lp({maximize: {x: 1}, subjectTo: [{x: 2, le: 4}]}) |> (r => r.fractions)`); got != "null" && got != "undefined" {
		t.Errorf("fractions on integer answer = %s", got)
	}
	// Decimal coefficients stay exact: 0.1x <= 0.3 gives x == 3, not 2.9999.
	if got := okEval(t, solverShell(t), `Solver.lp({maximize: {x: 1}, subjectTo: [{x: 0.1, le: 0.3}]}).values.x == 3`); got != "true" {
		t.Errorf("decimal exactness: %s", got)
	}
}

func TestSolverKnapsackForms(t *testing.T) {
	// solver.TestKnapsackSmall: optimum 15.
	dense := `Solver.lp({maximize: {i0: 4, i1: 2, i2: 1, i3: 2, i4: 10},
	  subjectTo: [{i0: 12, i1: 2, i2: 1, i3: 1, i4: 4, le: 15}],
	  bounds: {i0: [0, 1], i1: [0, 1], i2: [0, 1], i3: [0, 1], i4: [0, 1]}, int: true}).objective`
	if got := okEval(t, solverShell(t), dense); got != "15" {
		t.Errorf("dense = %s", got)
	}
	rows := `let items = [{id: "a", w: 12, v: 4}, {id: "b", w: 2, v: 2}, {id: "c", w: 1, v: 1}, {id: "d", w: 1, v: 2}, {id: "e", w: 4, v: 10}];
	let r = Solver.lp({sense: "max",
	  vars: items |> map(i => ({id: i.id, obj: i.v, lo: 0, hi: 1, int: true})),
	  coefs: items |> map(i => ({con: "w", var: i.id, coef: i.w})),
	  rhs: [{con: "w", le: 15}]});
	[r.objective, r.vars |> filter(v => v.value == 1) |> map(v => v.id) |> join("")] |> join(",")`
	if got := okEval(t, solverShell(t), rows); got != "15,bcde" {
		t.Errorf("rows = %s", got)
	}
}

func TestSolverTransport(t *testing.T) {
	// solver.TestTransportation: optimum 190 at 25,5,0,20.
	dense := `Solver.lp({minimize: {x11: 4, x12: 6, x21: 5, x22: 3}, subjectTo: [
	  {x11: 1, x12: 1, eq: 30}, {x21: 1, x22: 1, eq: 20}, {x11: 1, x21: 1, eq: 25}, {x12: 1, x22: 1, eq: 25}]})
	  |> (r => [r.objective, r.values.x11, r.values.x12, r.values.x21, r.values.x22] |> join(","))`
	if got := okEval(t, solverShell(t), dense); got != "190,25,5,0,20" {
		t.Errorf("dense = %s", got)
	}
	sparse := `let cost = {x11: 4, x12: 6, x21: 5, x22: 3};
	Solver.lp({sense: "min",
	  vars: cost |> entries |> map(e => ({id: e[0], obj: e[1]})),
	  coefs: [{con:"s1",var:"x11",coef:1},{con:"s1",var:"x12",coef:1},{con:"s2",var:"x21",coef:1},{con:"s2",var:"x22",coef:1},
	          {con:"d1",var:"x11",coef:1},{con:"d1",var:"x21",coef:1},{con:"d2",var:"x12",coef:1},{con:"d2",var:"x22",coef:1}],
	  rhs: [{con:"s1",eq:30},{con:"s2",eq:20},{con:"d1",eq:25},{con:"d2",eq:25}]})
	  |> (r => [r.objective, r.values.x11, r.values.x12, r.values.x21, r.values.x22] |> join(","))`
	if got := okEval(t, solverShell(t), sparse); got != "190,25,5,0,20" {
		t.Errorf("sparse = %s", got)
	}
}

func TestSolverStatuses(t *testing.T) {
	got := okEval(t, solverShell(t), `let r = Solver.lp({maximize: {x: 1}, subjectTo: [{x: 1, le: 1}, {x: 1, ge: 2}]});
	[r.status, r.objective, r.values, r.vars |> len()] |> map(v => str(v)) |> join(",")`)
	if got != "infeasible,null,null,0" {
		t.Errorf("infeasible = %s", got)
	}
	got = okEval(t, solverShell(t), `Solver.lp({maximize: {x: 1, y: 1}, subjectTo: [{x: 1, y: -1, le: 1}]}).status`)
	if got != "unbounded" {
		t.Errorf("unbounded = %s", got)
	}
	// Free variable via null lower bound: min x, x >= -5 -> -5.
	got = okEval(t, solverShell(t), `Solver.lp({minimize: {x: 1}, subjectTo: [{x: 1, ge: -5}], bounds: {x: [null, null]}}).objective`)
	if got != "-5" {
		t.Errorf("free var = %s", got)
	}
	// 2*sum(x) = 25 has no integer point but the LP relaxation does. Cut short.
	got = okEval(t, solverShell(t), `let r = Solver.lp({sense: "max",
	  vars: range(0, 25) |> map(i => ({id: "x" + str(i), obj: 1, hi: 1, int: true})),
	  coefs: range(0, 25) |> map(i => ({con: "p", var: "x" + str(i), coef: 2})),
	  rhs: [{con: "p", eq: 25}], limits: {nodes: 30}});
	[r.status, r.nodes, r.values] |> map(v => str(v)) |> join(",")`)
	if got != "node_limit,30,null" {
		t.Errorf("node_limit = %s", got)
	}
	// Knapsack cut short with an incumbent: values, gap and bound present.
	got = okEval(t, solverShell(t), `let w = [23,31,29,44,53,38,63,85,89,82,17,26,41,35,57,19,48,72,33,66];
	let name = i => "i" + str(i);
	let r = Solver.lp({sense: "max",
	  vars: range(0, 20) |> map(i => ({id: name(i), obj: w[i] + i % 7, hi: 1, int: true})),
	  coefs: range(0, 20) |> map(i => ({con: "cap", var: name(i), coef: w[i]})),
	  rhs: [{con: "cap", le: 400}], limits: {nodes: 40}});
	[r.status, r.values != null, r.gap != null, r.bound != null] |> join(",")`)
	if got != "node_limit,true,true,true" {
		t.Errorf("node_limit incumbent = %s", got)
	}
}

func TestSolverErrors(t *testing.T) {
	sh := solverShell(t)
	cases := []struct {
		src  string
		want []string
	}{
		{`Solver.lp()`, []string{"Wrong arguments", "Solver.lp"}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [{x: 1, le: 1}, {x: 1, y: 2}]})`, []string{"subjectTo[1]", "exactly one of le, ge, eq"}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [{x: "a", le: 1}]})`, []string{"subjectTo[0] x", "must be a number", `string: "a"`}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [{x: 1, le: "7"}]})`, []string{"subjectTo[0] le", "must be a number"}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [{x: Math.sqrt(2), le: 7}]})`, []string{"subjectTo[0] x", "not an exact number"}},
		{`Solver.lp({sense: "max", vars: [{id: "x", obj: 1}], coefs: [{con: "c", var: "z", coef: 1}], rhs: [{con: "c", le: 1}]})`, []string{"coefs[0]", `unknown variable "z"`, "variables: x"}},
		{`Solver.lp({sense: "max", vars: [{id: "x", obj: 1}], subjectTo: [{y: 1, le: 1}]})`, []string{"subjectTo[0]", `unknown variable "y"`}},
		{`Solver.lp({sense: "max", vars: [{id: "x", obj: 1}], coefs: [{con: "c", var: "x", coef: 1}], rhs: []})`, []string{"coefs[0]", `constraint "c" has no rhs row`}},
		{`Solver.lp({sense: "max", vars: [{id: "x", obj: 1}], coefs: [{con: "c", var: "x", coef: 1}], rhs: [{con: "c"}]})`, []string{`rhs[0] ("c")`, "exactly one of le, ge, eq"}},
		{`Solver.lp({sense: "max", vars: [{id: "x", obj: 1}], coefs: [{con: "c", var: "x"}], rhs: [{con: "c", le: 1}]})`, []string{"coefs[0] has no coef"}},
		{`Solver.lp({sense: "max", vars: [{id: "x", obj: "big"}]})`, []string{"vars[0] obj", "must be a number"}},
		{`Solver.lp({sense: "max", vars: [{id: "x"}, {id: "x"}]})`, []string{"vars[1]", `duplicate variable id "x"`}},
		{`Solver.lp({subjectTo: [{x: 1, le: 1}]})`, []string{"give the objective once"}},
		{`Solver.lp({maximize: {x: 1}, minimize: {x: 1}})`, []string{"give the objective once"}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [], bounds: {q: [0, 1]}})`, []string{"bounds", `unknown variable "q"`}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [], int: ["q"]})`, []string{"int", `unknown variable "q"`}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [{x: 1, le: 1, name: "a"}, {x: 1, le: 2, name: "a"}]})`, []string{"subjectTo[1]", `duplicate constraint name "a"`}},
		{`Solver.lp({maximise: {x: 1}})`, []string{`unknown key "maximise"`, "Keys:"}},
		{`Solver.lp({maximize: {x: 1}, subjectTo: [{x: 1, le: 1}], limits: {nodes: 0}})`, []string{"limits.nodes", "positive integer"}},
		{`Solver.lp({maximize: {x: 1}, bounds: {x: [3, 1]}})`, []string{"lower bound", "upper bound"}},
		{`Solver.lp({maximize: {x: 1, y: 1}, subjectTo: [{x: 1, y: 1, le: 1}], limits: {vars: 1}})`, []string{"limit hit", "limits.vars"}},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) { mustContain(t, "error", failEval(t, sh, c.src), c.want...) })
	}
}

func TestSolverTimeLimit(t *testing.T) {
	sh := solverShell(t)
	sh.Limits().SetDefaults(1_000_000, 256, 300)
	start := time.Now()
	msg := failEval(t, sh, `Solver.lp({sense: "max",
	  vars: range(0, 41) |> map(i => ({id: "x" + str(i), obj: 1, hi: 1, int: true})),
	  coefs: range(0, 41) |> map(i => ({con: "p", var: "x" + str(i), coef: 2})),
	  rhs: [{con: "p", eq: 41}], limits: {nodes: 100000000}})`)
	mustContain(t, "timeout error", msg, "Solver.lp", "time limit (300ms)", "extendLimit")
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
	if strings.Contains(msg, "context") {
		t.Errorf("raw context error leaked: %s", msg)
	}
}

func TestSolverSensitivity(t *testing.T) {
	sh := solverShell(t)
	got := okEval(t, sh, `let r = Solver.lp({maximize: {chair: 3, table: 5},
	  subjectTo: [{chair: 1, table: 2, le: 14, name: "wood"}, {chair: 3, table: -1, ge: 0, name: "ratio"}, {chair: 1, le: 6}],
	  sensitivity: true});
	[r.constraints |> map(c => [c.name, c.shadowPrice, c.rhsRange]), r.vars |> map(v => [v.id, v.reducedCost, v.objRange])]`)
	if want := `[[["wood", 2.5, [6, 42]], ["ratio", 0, [null, 14]], ["c3", 0.5, [2, 14]]], [["chair", 0, [2.5, null]], ["table", 0, [0, 6]]]]`; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	// Without the flag, no sensitivity keys.
	mustContain(t, "plain keys", okEval(t, sh, `Solver.lp({maximize: {x: 1}, subjectTo: [{x: 1, le: 2}]}).constraints[0] |> keys`), `["name", "slack", "active"]`)
	mustContain(t, "integer refused", failEval(t, sh, `Solver.lp({maximize: {x: 1}, subjectTo: [{x: 2, le: 3}], int: true, sensitivity: true})`),
		"sensitivity needs an LP", `{x: [v, v]}`, "Usage:")
	mustContain(t, "flag type", failEval(t, sh, `Solver.lp({maximize: {x: 1}, sensitivity: 1})`), "sensitivity must be true or false")
}

func TestSolverExplain(t *testing.T) {
	sh := solverShell(t)
	got := okEval(t, sh, `Solver.lp({minimize: {a: 1, b: 1}, subjectTo: [{c: 1, le: 100, name: "budget"}, {a: 1, b: 1, ge: 10, name: "demand"},
	  {a: 1, le: 4, name: "capA"}, {b: 1, le: 5, name: "capB"}], explain: true}).conflict`)
	if want := `{constraints: ["demand", "capA", "capB"], bounds: [], irreducible: true, integerOnly: false}`; got != want {
		t.Errorf("got %s", got)
	}
	got = okEval(t, sh, `Solver.lp({minimize: {a: 1}, subjectTo: [{a: 1, le: -3}], explain: true}).conflict.bounds`)
	if want := `[{var: "a", lo: 0, default: true}]`; got != want {
		t.Errorf("default bound: got %s", got)
	}
	got = okEval(t, sh, `Solver.lp({minimize: {a: 1}, subjectTo: [{a: 2, eq: 3}], int: true, explain: true}).conflict.integerOnly`)
	if got != "true" {
		t.Errorf("integerOnly: got %s", got)
	}
	// A feasible model with explain: no conflict key.
	mustContain(t, "feasible", okEval(t, sh, `Solver.lp({minimize: {a: 1}, explain: true}) |> keys`), `"status"`)
	if strings.Contains(okEval(t, sh, `Solver.lp({minimize: {a: 1}, explain: true}) |> keys`), "conflict") {
		t.Error("conflict on a feasible model")
	}
}

func TestSolverInfeasibleNote(t *testing.T) {
	sh := solverShell(t)
	okEval(t, sh, `Solver.lp({minimize: {a: 1}, subjectTo: [{a: 1, ge: 10}, {a: 1, le: 4}]}).status`)
	mustContain(t, "note", strings.Join(sh.Limits().Notes(), "\n"), "explain: true")
}
