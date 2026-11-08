package toolkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	. "github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/solver"
)

// AddSolver registers the exact LP/MILP solver as the loadable toolkit
// "solver" (namespace Solver). It has no outside effect, so it needs no flag.
func AddSolver(sh *Shell) *Shell {
	return sh.AddLoadable(Loadable{
		Name:    "solver",
		Summary: "exact LP/MILP optimizer (Solver.lp): product mix, knapsack, assignment, scheduling",
		Guide:   solverGuide,
		Install: func(sh *Shell) (io.Closer, error) {
			sh.Register(&CommandDef{
				Namespace: "Solver", Name: "lp",
				Signature: "model: {maximize|minimize, subjectTo, bounds?, int?, limits?, sensitivity?, explain?} | {sense, vars, coefs, rhs, ...}",
				Description: "solves a linear or mixed-integer program exactly (rationals, no floats). " +
					"Returns {status, objective, values, vars, constraints, nodes}",
				Examples: []string{
					`Solver.lp({maximize: {a: 3, b: 5}, subjectTo: [{a: 1, le: 4}, {b: 2, le: 12}, {a: 3, b: 2, le: 18}]})`,
				},
				Fn: func(args []Value) Value { return solverLP(sh, args) },
			})
			return nil, nil
		},
	})
}

const solverGuide = `Solver — exact linear and mixed-integer programming. load("solver") first.
One command: Solver.lp(model). Numbers are exact: 1/3 stays 1/3, no float tolerance.
Status is optimal, infeasible, unbounded or node_limit.

TYPICAL: Product mix (LP)
  Chairs earn 3, tables earn 5. Wood: 1 per chair, 2 per table, 14 total.
  Tables at most 3 per chair (3*chair - table >= 0). At most 6 chairs.
  let r = Solver.lp({
    maximize: {chair: 3, table: 5},
    subjectTo: [{chair: 1, table: 2, le: 14, name: "wood"},
                {chair: 3, table: -1, ge: 0, name: "ratio"},
                {chair: 1, le: 6}]
  })
  r.status      // "optimal"
  r.objective   // 38
  r.values      // {chair: 6, table: 4}
  r.constraints // [{name: "wood", slack: 0, active: true}, ...]

TYPICAL: Knapsack (integer, 0/1)
  let items = [{id: "a", w: 12, v: 4}, {id: "b", w: 2, v: 2}, {id: "c", w: 1, v: 1},
               {id: "d", w: 1, v: 2}, {id: "e", w: 4, v: 10}]
  let r = Solver.lp({
    sense: "max",
    vars: items |> map(i => ({id: i.id, obj: i.v, lo: 0, hi: 1, int: true})),
    coefs: items |> map(i => ({con: "weight", var: i.id, coef: i.w})),
    rhs: [{con: "weight", le: 15}]
  })
  r.objective   // 15
  r.vars |> filter(v => v.value == 1) |> map(v => v.id)

TYPICAL: Transport or assignment (rows from a join)
  Rows come straight from a query: coefs = [{con, var, coef}], rhs = [{con, le|ge|eq}].
  let cost = {x11: 4, x12: 6, x21: 5, x22: 3}
  let r = Solver.lp({
    sense: "min",
    vars: cost |> entries |> map(e => ({id: e[0], obj: e[1]})),
    coefs: [{con:"s1",var:"x11",coef:1},{con:"s1",var:"x12",coef:1},
            {con:"s2",var:"x21",coef:1},{con:"s2",var:"x22",coef:1},
            {con:"d1",var:"x11",coef:1},{con:"d1",var:"x21",coef:1},
            {con:"d2",var:"x12",coef:1},{con:"d2",var:"x22",coef:1}],
    rhs: [{con:"s1",eq:30},{con:"s2",eq:20},{con:"d1",eq:25},{con:"d2",eq:25}]
  })
  r.objective   // 190

INPUT, form 1: objects (small models, variables inferred)
  maximize: {var: coef}  or  minimize: {var: coef}      exactly one of the two
  subjectTo: [{a: 1, b: 2, le: 14, name?: "c1"}]        keys other than le, ge, eq, name
                                                        are variables; exactly one of le/ge/eq.
                                                        {terms: {a: 1}, le: 4} also works,
                                                        for a variable named le, ge, eq or name.
  bounds: {a: [lo, hi]}   null = no limit. Default bounds are [0, no limit].
  int: ["a", "b"]  or  int: true (all variables)
  limits: {nodes?, pivots?, vars?, constraints?}        defaults 10000, 1000000, 1000, 1000

INPUT, form 2: rows (data from queries; a SQL join gives these shapes)
  sense: "max" | "min"
  vars: [{id, obj?, lo?, hi?, int?}]    obj missing or null = 0. lo missing = 0, lo null = no
                                        lower bound. hi missing or null = no upper bound.
  coefs: [{con, var, coef}]             one row per nonzero; repeated (con, var) add up
  rhs: [{con, le|ge|eq}]                one row per constraint; order = constraint order
  Form 2 needs vars. bounds, int and limits work in both forms. Form 1 rows (subjectTo) and
  form 2 rows (coefs, rhs) can be combined. Give the objective once.

OUTPUT
  {status, objective, values: {var: n}, vars: [{id, value}],
   constraints: [{name, slack, active}], nodes, pivots, gap?, bound?, fractions?}
  objective and values are null when there is no solution (infeasible, unbounded).
  slack: room left on a constraint (rhs-lhs for le, lhs-rhs for ge, 0 for eq).
  active: slack == 0.
  node_limit: the search stopped at limits.nodes. values hold the best integer solution
  found (null if none); gap = distance to the best possible objective; bound = that objective.
  unbounded: the LP relaxation has no finite optimum.
  fractions: present when a number is not an integer. {objective: "20/7", values: {x: "9/7"}}
  Numbers are exact; their display rounds to 34 digits when a fraction does not
  terminate (1/3 shows 0.3333...). Compare with ==, or read fractions for p/q text.

WHAT-IF (LP only): sensitivity: true
  Adds to each constraints row: shadowPrice = objective change per unit of rhs, valid
  while the rhs stays in rhsRange: [lo, hi]. Adds to each vars row: reducedCost =
  objective change per unit a variable at a bound is pushed off it (0 when it is
  between its bounds), and objRange: [lo, hi] = the objective coefficients for which
  this solution stays optimal. null in a range = no limit.
  r = Solver.lp({...product mix above..., sensitivity: true})
  r.constraints[0]   // {name: "wood", slack: 0, active: true, shadowPrice: 2.5, rhsRange: [6, 42]}
                     // one more unit of wood is worth 2.5, up to 42 units
  r.vars[1]          // {id: "table", value: 4, reducedCost: 0, objRange: [0, 6]}
  Integer models have no shadow prices: sensitivity with int is an error.
  Ranges hold for the current solution's basis; at a degenerate corner a range end can
  equal the current value although a larger move gives the same answer.

WHY INFEASIBLE: explain: true
  When status is infeasible, adds conflict: {constraints: [name], bounds: [{var, lo|hi,
  default?}], irreducible, integerOnly}: constraints and bounds that cannot all hold.
  Remove or relax any one of them and the rest is feasible (irreducible: true).
  default: true marks the implicit lower bound 0. integerOnly: true means the set is
  feasible without int; it fails only because the variables must be whole.
  Cost: one solve per constraint and bound, so it is not on by default.
  Solver.lp({minimize: {a: 1, b: 1}, subjectTo: [{a: 1, b: 1, ge: 10, name: "demand"},
    {a: 1, le: 4, name: "capA"}, {b: 1, le: 5, name: "capB"}], explain: true}).conflict
  // {constraints: ["demand", "capA", "capB"], bounds: [], irreducible: true, integerOnly: false}

LIMITS
  The eval timeout bounds the solve. For a big model raise it before the run (eval argument limits: {timeout: ms}, or extendLimit({timeout: ms}) in code).
  Max 1000 variables and 1000 constraints (limits.vars, limits.constraints raise them).`

// solverLP implements Solver.lp.
func solverLP(sh *Shell, args []Value) Value {
	const cmd = "Solver.lp"
	if len(args) != 1 {
		panic(WrongArguments(cmd, "model: object", args, solverExample))
	}
	in, ok := args[0].(*ObjectVal)
	if !ok {
		panic(WrongArguments(cmd, "model: object", args, solverExample))
	}
	m, names, lim := parseSolverModel(in)
	wantSens := solverFlag(in, "sensitivity")
	wantExplain := solverFlag(in, "explain")
	if wantSens {
		for _, v := range m.Vars {
			if v.Integer {
				panic(Runtime(fmt.Sprintf("%s: sensitivity needs an LP, and %q is an integer variable. Shadow prices and ranges do not exist for integer models. To see them for one integer solution, solve, fix the integers with bounds: {%s: [v, v]}, drop int, and solve again with sensitivity: true", cmd, v.Name, v.Name)))
			}
		}
	}

	lm := sh.Limits()
	remaining := lm.Remaining()
	if remaining <= 0 {
		panic(Runtime(cmd + ": no time left in this eval's time limit (" + fmt.Sprint(lm.TimeoutMs) + "ms). Raise it first: " + lm.RaiseHint("timeout", lm.TimeoutMs*2)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), remaining)
	defer cancel()
	// Cancel() on the limits (a cancelled eval) stops the solve too.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(25 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if lm.Cancelled() {
					cancel()
					return
				}
			}
		}
	}()

	var res *solver.Result
	var sens *solver.Sensitivity
	var err error
	if wantSens {
		res, sens, err = solver.SolveSensitivity(ctx, m, lim)
	} else {
		res, err = solver.Solve(ctx, m, lim)
	}
	if err == nil && res.Status == solver.Infeasible {
		if wantExplain {
			conflict, cerr := solver.ExplainInfeasible(ctx, m, lim)
			if cerr == nil {
				out := solverResult(m, names, res, nil).(*ObjectVal)
				out.Set("conflict", conflictValue(m, names, conflict))
				if !conflict.Irreducible {
					sh.Note(cmd + ": the conflict search stopped early (time limit, or limits.nodes on an integer check); the conflict is infeasible but may not be the smallest. Raise the timeout or limits.nodes for a smaller one")
				}
				return out
			}
			err = cerr
		} else {
			sh.Note(cmd + ": infeasible. Add explain: true to the model to get the constraints and bounds that conflict (conflict: {constraints, bounds})")
		}
	}
	if err != nil {
		var le *solver.LimitError
		switch {
		case errors.As(err, &le):
			opt := map[string]string{"variables": "vars", "constraints": "constraints", "pivots": "pivots"}[le.Limit]
			panic(Runtime(fmt.Sprintf("%s: limit hit: more than %d %s. Shrink the model or raise limits.%s", cmd, le.Max, le.Limit, opt)))
		case errors.Is(err, context.DeadlineExceeded):
			panic(Runtime(fmt.Sprintf("%s: stopped at the execution time limit (%dms). Shrink the model, set limits.nodes to get the best answer so far, or raise the limit first (%s)", cmd, lm.TimeoutMs, lm.RaiseHint("timeout", lm.TimeoutMs*2))))
		case errors.Is(err, context.Canceled):
			panic(Runtime(cmd + ": Execution cancelled"))
		}
		panic(Runtime(cmd + ": " + strings.TrimPrefix(err.Error(), "solver: invalid model: ")))
	}
	return solverResult(m, names, res, sens)
}

// solverFlag reads an optional boolean model key.
func solverFlag(in *ObjectVal, key string) bool {
	v, ok := in.Get(key)
	if !ok || v == Null {
		return false
	}
	b, isBool := v.(*BoolVal)
	if !isBool {
		panic(Runtime(fmt.Sprintf("Solver.lp: %s must be true or false, got %s", key, describe(v))))
	}
	return b.V
}

// conflictValue is {constraints: [name], bounds: [{var, lo|hi, default?}],
// irreducible, integerOnly}.
func conflictValue(m *solver.Model, names []string, c *solver.Conflict) Value {
	cons := make([]Value, 0, len(c.Cons))
	for _, i := range c.Cons {
		cons = append(cons, Str(names[i]))
	}
	bounds := []Value{}
	for _, j := range c.Lo {
		b := NewObject()
		b.Set("var", Str(m.Vars[j].Name))
		if m.Vars[j].Lo == nil {
			b.Set("lo", numInt0())
			b.Set("default", Bln(true))
		} else {
			b.Set("lo", NumRat(new(big.Rat).Set(m.Vars[j].Lo)))
		}
		bounds = append(bounds, b)
	}
	for _, j := range c.Hi {
		b := NewObject()
		b.Set("var", Str(m.Vars[j].Name))
		b.Set("hi", NumRat(new(big.Rat).Set(m.Vars[j].Hi)))
		bounds = append(bounds, b)
	}
	out := NewObject()
	out.Set("constraints", &ArrayVal{Elements: cons})
	out.Set("bounds", &ArrayVal{Elements: bounds})
	out.Set("irreducible", Bln(c.Irreducible))
	out.Set("integerOnly", Bln(c.Integer))
	return out
}

func numInt0() Value { return NumRat(new(big.Rat)) }

// ratOrNull is an exact number, or null for an unlimited range end.
func ratOrNull(r *big.Rat) Value {
	if r == nil {
		return Null
	}
	return NumRat(new(big.Rat).Set(r))
}

const solverExample = `Solver.lp({maximize: {x: 3, y: 5}, subjectTo: [{x: 1, le: 4}, {y: 2, le: 12}, {x: 3, y: 2, le: 18}]})`

var solverKeys = []string{"maximize", "minimize", "sense", "subjectTo", "bounds", "int", "limits", "vars", "coefs", "rhs", "sensitivity", "explain"}

// parseSolverModel turns the language model into a solver.Model.
func parseSolverModel(in *ObjectVal) (*solver.Model, []string, solver.Limits) {
	const cmd = "Solver.lp"
	fail := func(f string, a ...any) { panic(Runtime(cmd + ": " + fmt.Sprintf(f, a...))) }
	for _, k := range in.Keys() {
		if !contains(solverKeys, k) {
			fail("unknown key %q. Keys: %s", k, strings.Join(solverKeys, ", "))
		}
	}
	get := func(k string) (Value, bool) {
		v, ok := in.Get(k)
		if ok && v == Null {
			return nil, false
		}
		return v, ok
	}
	array := func(k string) []Value {
		v, ok := get(k)
		if !ok {
			return nil
		}
		a, isArr := v.(*ArrayVal)
		if !isArr {
			fail("%s must be an array of rows, got %s", k, describe(v))
		}
		return a.Elements
	}
	object := func(k string) *ObjectVal {
		v, ok := get(k)
		if !ok {
			return nil
		}
		o, isObj := v.(*ObjectVal)
		if !isObj {
			fail("%s must be an object, got %s", k, describe(v))
		}
		return o
	}

	m := &solver.Model{}
	idx := map[string]int{}
	addVar := func(name string) int {
		if i, ok := idx[name]; ok {
			return i
		}
		idx[name] = len(m.Vars)
		m.Vars = append(m.Vars, solver.Var{Name: name})
		return idx[name]
	}
	varRows := array("vars")
	rowsForm := len(varRows) > 0

	// Variables from rows.
	var rowObj []solver.Term
	for i, r := range varRows {
		where := fmt.Sprintf("vars[%d]", i)
		o, isObj := r.(*ObjectVal)
		if !isObj {
			fail("%s must be an object {id, obj?, lo?, hi?, int?}, got %s", where, describe(r))
		}
		idv, ok := o.Get("id")
		if !ok {
			fail("%s has no id. Row: %s", where, o.Display())
		}
		name := keyOf(where+" id", idv, fail)
		if _, dup := idx[name]; dup {
			fail("%s: duplicate variable id %q", where, name)
		}
		vi := addVar(name)
		for _, k := range o.Keys() {
			if !contains([]string{"id", "obj", "lo", "hi", "int"}, k) {
				fail("%s (%q): unknown key %q. Keys: id, obj, lo, hi, int", where, name, k)
			}
		}
		if v, ok := o.Get("obj"); ok && v != Null {
			rowObj = append(rowObj, solver.Term{Var: name, Coef: ratOf(where+" obj", v, fail)})
		}
		if v, ok := o.Get("lo"); ok {
			if v == Null {
				m.Vars[vi].NoLo = true
			} else {
				m.Vars[vi].Lo = ratOf(where+" lo", v, fail)
			}
		}
		if v, ok := o.Get("hi"); ok && v != Null {
			m.Vars[vi].Hi = ratOf(where+" hi", v, fail)
		}
		if v, ok := o.Get("int"); ok && v != Null {
			m.Vars[vi].Integer = boolOf(where+" int", v, fail)
		}
	}

	// Objective.
	maxO, minO := object("maximize"), object("minimize")
	senseV, hasSense := get("sense")
	nObj := 0
	for _, b := range []bool{maxO != nil, minO != nil, hasSense} {
		if b {
			nObj++
		}
	}
	if nObj != 1 {
		fail("give the objective once: maximize: {var: coef}, minimize: {var: coef}, or sense: \"max\"|\"min\" with vars[].obj")
	}
	switch {
	case hasSense:
		s, isStr := senseV.(*StringVal)
		if !isStr {
			fail("sense must be \"max\" or \"min\", got %s", describe(senseV))
		}
		switch strings.ToLower(s.V) {
		case "max", "maximize":
			m.Sense = solver.Maximize
		case "min", "minimize":
			m.Sense = solver.Minimize
		default:
			fail("sense must be \"max\" or \"min\", got %q", s.V)
		}
		if !rowsForm {
			fail("sense needs vars rows with obj; use maximize or minimize for an object objective")
		}
		m.Objective = rowObj
	default:
		o := maxO
		m.Sense = solver.Maximize
		if minO != nil {
			o, m.Sense = minO, solver.Minimize
		}
		if len(rowObj) > 0 {
			fail("objective given twice: vars[].obj and %s", map[bool]string{true: "maximize", false: "minimize"}[maxO != nil])
		}
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			t := solver.Term{Var: k, Coef: ratOf("objective "+k, v, fail)}
			if rowsForm {
				if _, ok := idx[k]; !ok {
					fail("objective: unknown variable %q%s", k, knownVars(m))
				}
			} else {
				addVar(k)
			}
			m.Objective = append(m.Objective, t)
		}
	}

	// resolve checks a variable name against the declared variables, or adds it.
	resolve := func(where, name string) {
		if _, ok := idx[name]; ok {
			return
		}
		if rowsForm {
			fail("%s: unknown variable %q%s", where, name, knownVars(m))
		}
		addVar(name)
	}

	// Form 1 constraints.
	used := map[string]bool{}
	nameFor := func(where, name string) string {
		if used[name] {
			fail("%s: duplicate constraint name %q", where, name)
		}
		used[name] = true
		return name
	}
	var names []string
	for i, r := range array("subjectTo") {
		where := fmt.Sprintf("subjectTo[%d]", i)
		o, isObj := r.(*ObjectVal)
		if !isObj {
			fail("%s must be an object like {x: 1, y: 2, le: 14}, got %s", where, describe(r))
		}
		c := solver.Constraint{}
		name := fmt.Sprintf("c%d", i+1)
		var termsObj *ObjectVal
		if tv, ok := o.Get("terms"); ok {
			termsObj, isObj = tv.(*ObjectVal)
			if !isObj {
				fail("%s: terms must be an object {var: coef}, got %s", where, describe(tv))
			}
		}
		nOp := 0
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			switch k {
			case "le", "ge", "eq":
				nOp++
				c.Op = map[string]solver.Op{"le": solver.LE, "ge": solver.GE, "eq": solver.EQ}[k]
				c.RHS = ratOf(where+" "+k, v, fail)
			case "name":
				s, isStr := v.(*StringVal)
				if !isStr {
					fail("%s: name must be a string, got %s", where, describe(v))
				}
				name = s.V
			case "terms":
			default:
				if termsObj != nil {
					fail("%s: with terms, the other keys are le, ge, eq, name; got %q", where, k)
				}
				resolve(where, k)
				c.Terms = append(c.Terms, solver.Term{Var: k, Coef: ratOf(fmt.Sprintf("%s %s", where, k), v, fail)})
			}
		}
		if termsObj != nil {
			for _, k := range termsObj.Keys() {
				v, _ := termsObj.Get(k)
				resolve(where, k)
				c.Terms = append(c.Terms, solver.Term{Var: k, Coef: ratOf(fmt.Sprintf("%s terms.%s", where, k), v, fail)})
			}
		}
		if nOp != 1 {
			fail("%s needs exactly one of le, ge, eq (got %d). Keys other than le, ge, eq, name, terms are variables. Row: %s", where, nOp, o.Display())
		}
		names = append(names, nameFor(where, name))
		m.Cons = append(m.Cons, c)
	}

	// Form 2 constraints: rhs rows define constraints, coefs rows fill them.
	cidx := map[string]int{}
	for i, r := range array("rhs") {
		where := fmt.Sprintf("rhs[%d]", i)
		o, isObj := r.(*ObjectVal)
		if !isObj {
			fail("%s must be an object {con, le|ge|eq}, got %s", where, describe(r))
		}
		cv, ok := o.Get("con")
		if !ok {
			fail("%s has no con. Row: %s", where, o.Display())
		}
		con := keyOf(where+" con", cv, fail)
		c := solver.Constraint{}
		nOp := 0
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			switch k {
			case "con":
			case "le", "ge", "eq":
				nOp++
				c.Op = map[string]solver.Op{"le": solver.LE, "ge": solver.GE, "eq": solver.EQ}[k]
				c.RHS = ratOf(fmt.Sprintf("%s (%q) %s", where, con, k), v, fail)
			default:
				fail("%s (%q): unknown key %q. Keys: con, le, ge, eq", where, con, k)
			}
		}
		if nOp != 1 {
			fail("%s (%q) needs exactly one of le, ge, eq (got %d). Row: %s", where, con, nOp, o.Display())
		}
		if _, dup := cidx[con]; dup {
			fail("%s: duplicate constraint %q", where, con)
		}
		cidx[con] = len(m.Cons)
		names = append(names, nameFor(where, con))
		m.Cons = append(m.Cons, c)
	}
	for i, r := range array("coefs") {
		where := fmt.Sprintf("coefs[%d]", i)
		o, isObj := r.(*ObjectVal)
		if !isObj {
			fail("%s must be an object {con, var, coef}, got %s", where, describe(r))
		}
		for _, k := range o.Keys() {
			if k != "con" && k != "var" && k != "coef" {
				fail("%s: unknown key %q. Keys: con, var, coef", where, k)
			}
		}
		var parts [3]Value
		for j, k := range []string{"con", "var", "coef"} {
			v, ok := o.Get(k)
			if !ok || v == Null {
				fail("%s has no %s. Row: %s", where, k, o.Display())
			}
			parts[j] = v
		}
		con := keyOf(where+" con", parts[0], fail)
		name := keyOf(where+" var", parts[1], fail)
		ci, ok := cidx[con]
		if !ok {
			fail("%s: constraint %q has no rhs row {con: %q, le|ge|eq: n}", where, con, con)
		}
		resolve(where, name)
		m.Cons[ci].Terms = append(m.Cons[ci].Terms,
			solver.Term{Var: name, Coef: ratOf(fmt.Sprintf("%s (%q, %q) coef", where, con, name), parts[2], fail)})
	}

	// bounds and int.
	if b := object("bounds"); b != nil {
		for _, k := range b.Keys() {
			i, ok := idx[k]
			if !ok {
				fail("bounds: unknown variable %q%s", k, knownVars(m))
			}
			v, _ := b.Get(k)
			pair, isArr := v.(*ArrayVal)
			if !isArr || len(pair.Elements) != 2 {
				fail("bounds.%s must be [lo, hi] (null = no limit), got %s", k, describe(v))
			}
			if pair.Elements[0] == Null {
				m.Vars[i].NoLo, m.Vars[i].Lo = true, nil
			} else {
				m.Vars[i].NoLo, m.Vars[i].Lo = false, ratOf("bounds."+k+" lo", pair.Elements[0], fail)
			}
			m.Vars[i].Hi = nil
			if pair.Elements[1] != Null {
				m.Vars[i].Hi = ratOf("bounds."+k+" hi", pair.Elements[1], fail)
			}
		}
	}
	if iv, ok := get("int"); ok {
		switch x := iv.(type) {
		case *BoolVal:
			for i := range m.Vars {
				m.Vars[i].Integer = x.V
			}
		case *ArrayVal:
			for _, e := range x.Elements {
				name := keyOf("int", e, fail)
				i, ok := idx[name]
				if !ok {
					fail("int: unknown variable %q%s", name, knownVars(m))
				}
				m.Vars[i].Integer = true
			}
		default:
			fail("int must be true or an array of variable names, got %s", describe(iv))
		}
	}
	if len(m.Vars) == 0 {
		fail("no variables. Give maximize/minimize or subjectTo with variables, or vars rows")
	}

	// limits.
	var lim solver.Limits
	if lo := object("limits"); lo != nil {
		for _, k := range lo.Keys() {
			v, _ := lo.Get(k)
			n, isNum := v.(*NumberVal)
			if !isNum || n.V != float64(int(n.V)) || n.V < 1 {
				fail("limits.%s must be a positive integer, got %s", k, describe(v))
			}
			switch k {
			case "nodes":
				lim.MaxNodes = int(n.V)
			case "pivots":
				lim.MaxPivots = int(n.V)
			case "vars":
				lim.MaxVars = int(n.V)
			case "constraints":
				lim.MaxCons = int(n.V)
			default:
				fail("limits: unknown key %q. Keys: nodes, pivots, vars, constraints", k)
			}
		}
	}
	return m, names, lim
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func describe(v Value) string { return v.TypeName() + ": " + v.Inspect() }

func knownVars(m *solver.Model) string {
	var names []string
	for i, v := range m.Vars {
		if i == 8 {
			names = append(names, "...")
			break
		}
		names = append(names, v.Name)
	}
	if len(names) == 0 {
		return ""
	}
	return " (variables: " + strings.Join(names, ", ") + ")"
}

// keyOf reads an id: a string, or a number shown as its digits (SQL ids).
func keyOf(what string, v Value, fail func(string, ...any)) string {
	switch x := v.(type) {
	case *StringVal:
		if x.V == "" {
			fail("%s is an empty string", what)
		}
		return x.V
	case *NumberVal:
		return x.Display()
	}
	fail("%s must be a string or number, got %s", what, describe(v))
	return ""
}

func boolOf(what string, v Value, fail func(string, ...any)) bool {
	b, ok := v.(*BoolVal)
	if !ok {
		fail("%s must be true or false, got %s", what, describe(v))
	}
	return b.V
}

// ratOf reads an exact number. A float-only value (sqrt, sin, ...) has no
// exact form, so it is refused instead of rounded.
func ratOf(what string, v Value, fail func(string, ...any)) *big.Rat {
	n, ok := v.(*NumberVal)
	if !ok {
		fail("%s must be a number, got %s", what, describe(v))
	}
	r, exact := n.Exact()
	if !exact {
		fail("%s is not an exact number (%s comes from a float operation such as sqrt or sin). Use an integer, decimal or fraction such as 1/3", what, n.Display())
	}
	return new(big.Rat).Set(r)
}

func solverResult(m *solver.Model, names []string, res *solver.Result, sens *solver.Sensitivity) Value {
	status := map[solver.Status]string{
		solver.Optimal: "optimal", solver.Infeasible: "infeasible",
		solver.Unbounded: "unbounded", solver.NodeLimit: "node_limit",
	}[res.Status]

	out := NewObject()
	out.Set("status", Str(status))
	frac := NewObject()
	fracVals := NewObject()
	num := func(r *big.Rat, key string, into *ObjectVal) Value {
		if !r.IsInt() {
			into.Set(key, Str(r.RatString()))
		}
		return NumRat(new(big.Rat).Set(r))
	}
	if res.Objective != nil {
		out.Set("objective", num(res.Objective, "objective", frac))
	} else {
		out.Set("objective", Null)
	}
	vars := make([]Value, 0, len(m.Vars))
	if res.Values != nil {
		values := NewObject()
		for i, v := range m.Vars {
			x := num(res.Values[i], v.Name, fracVals)
			values.Set(v.Name, x)
			row := NewObject()
			row.Set("id", Str(v.Name))
			row.Set("value", x)
			if sens != nil {
				row.Set("reducedCost", NumRat(new(big.Rat).Set(sens.ReducedCost[i])))
				row.Set("objRange", &ArrayVal{Elements: []Value{ratOrNull(sens.ObjLo[i]), ratOrNull(sens.ObjHi[i])}})
			}
			vars = append(vars, row)
		}
		out.Set("values", values)
	} else {
		out.Set("values", Null)
	}
	out.Set("vars", &ArrayVal{Elements: vars})
	cons := make([]Value, 0, len(m.Cons))
	if res.Slack != nil {
		for i, s := range res.Slack {
			row := NewObject()
			row.Set("name", Str(names[i]))
			row.Set("slack", NumRat(new(big.Rat).Set(s)))
			row.Set("active", Bln(s.Sign() == 0))
			if sens != nil {
				row.Set("shadowPrice", NumRat(new(big.Rat).Set(sens.ShadowPrice[i])))
				row.Set("rhsRange", &ArrayVal{Elements: []Value{ratOrNull(sens.RHSLo[i]), ratOrNull(sens.RHSHi[i])}})
			}
			cons = append(cons, row)
		}
	}
	out.Set("constraints", &ArrayVal{Elements: cons})
	out.Set("nodes", Num(float64(res.Nodes)))
	out.Set("pivots", Num(float64(res.Pivots)))
	if res.Gap != nil {
		out.Set("gap", num(res.Gap, "gap", frac))
	}
	if res.BestBound != nil {
		out.Set("bound", num(res.BestBound, "bound", frac))
	}
	if fracVals.Len() > 0 {
		frac.Set("values", fracVals)
	}
	if frac.Len() > 0 {
		out.Set("fractions", frac)
	}
	return out
}
