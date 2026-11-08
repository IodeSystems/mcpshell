package toolkit

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/iodesystems/mcpshell/cp"
	. "github.com/iodesystems/mcpshell/runtime"
)

// AddCP registers the finite-domain constraint solver as the loadable
// toolkit "cp" (namespace Cp). It has no outside effect, so it needs no flag.
func AddCP(sh *Shell) *Shell {
	return sh.AddLoadable(Loadable{
		Name:    "cp",
		Summary: "constraint programming over integers (Cp.solve): rosters, timetables, job-shop schedules",
		Guide:   cpGuide,
		Install: func(sh *Shell) (io.Closer, error) {
			sh.Register(&CommandDef{
				Namespace: "Cp", Name: "solve",
				Signature: "model: {vars?, intervals?, constraints, minimize?|maximize?, all?, horizon?, limits?, timeLimit?}",
				Description: "solves an integer constraint model by propagation and search; optimal only when proven. " +
					"Returns {status, objective, bound, gap, values, schedule?, solutions?, complete, nodes}",
				Examples: []string{
					`Cp.solve({vars: {x: [0, 9], y: [0, 9]}, constraints: [{allDifferent: ["x", "y"]}, {x: 1, y: 1, eq: 9}], maximize: {x: 1}})`,
				},
				Fn: func(args []Value) Value { return cpSolve(sh, args) },
			})
			return nil, nil
		},
	})
}

const cpGuide = `Cp — constraint programming over integers. load("cp") first.
One command: Cp.solve(model). Variables have integer domains; constraints narrow them; search
tries values and backtracks. With an objective it keeps improving the best solution and
proves it best when the search finishes. For linear models with real numbers use Solver.lp.
status: "optimal" = proven best. "feasible" = a solution: without an objective, the one
asked for; with one, the best found before a limit (bound = best possible, gap =
|objective - bound| / |objective|). "infeasible" = proven none. "unknown" = stopped before
finding one or proving none.

TYPICAL: Shift roster (0/1 variables, linear rows, count)
  let nurses = ["ann", "bob", "cy"], days = [0, 1, 2]
  let v = {}
  for (const n of nurses) for (const d of days) v[n + d] = [0, 1]     // n works day d
  let r = Cp.solve({
    vars: v,
    constraints: [
      ...days.map(d => ({terms: nurses |> reduce((t, n) => ({...t, [n + d]: 1}), {}), ge: 2})),
      ...nurses.map(n => ({terms: days |> reduce((t, d) => ({...t, [n + d]: 1}), {}), le: 2})),
      {ann0: 1, eq: 0}],                                             // ann is off day 0
    minimize: {bob0: 1, bob1: 1, bob2: 1}})
  r.values        // {ann0: 0, ann1: 1, ...}

TYPICAL: Job shop (intervals, noOverlap per machine, before within a job)
  let ops = [{id: "a1", job: "a", m: "m1", d: 3}, {id: "a2", job: "a", m: "m2", d: 2},
             {id: "b1", job: "b", m: "m2", d: 2}, {id: "b2", job: "b", m: "m1", d: 4}]
  let r = Cp.solve({
    intervals: ops |> map(o => ({id: o.id, duration: o.d})),
    constraints: [
      ...(ops |> groupBy(o => o.m) |> values |> map(g => ({noOverlap: g |> map(o => o.id)}))),
      {before: "a1", after: "a2"}, {before: "b1", after: "b2"}],
    minimize: "makespan"})
  r.objective     // 7
  r.schedule      // [{id: "a1", start: 0, end: 3, duration: 3}, ...] sorted by start

TYPICAL: Assignment with costs (allDifferent, element)
  let cost = [[4, 2, 8], [4, 3, 7], [3, 1, 6]]                 // cost[worker][task]
  let r = Cp.solve({
    vars: {w0: [0, 2], w1: [0, 2], w2: [0, 2], c0: [0, 9], c1: [0, 9], c2: [0, 9]},
    constraints: [{allDifferent: ["w0", "w1", "w2"]},
      ...[0, 1, 2].map(i => ({element: cost[i], index: "w" + i, value: "c" + i}))],
    minimize: {c0: 1, c1: 1, c2: 1}})
  r.objective     // 12

INPUT
  vars: {x: [lo, hi], c: {values: [1, 3, 5]}}  or rows [{id, lo, hi} | {id, values}]
        [lo, hi] is a range (both ends in). A list of allowed values needs {values: [...]}.
  intervals: {a: {duration: 3, start?: [lo, hi] | n, end?: [lo, hi] | n}}  or rows
        [{id, duration, start?, end?, ...}]; other row keys are ignored, for grouping.
        Each interval adds the variables "a.start" and "a.end" (end = start + duration).
  horizon: n   latest end of any interval. Default: the sum of durations and gaps plus
        the largest start lower bound. infeasible with the default? Set it larger.
  constraints: rows, one of:
    {x: 1, y: -2, le: 4, name?}     linear; exactly one of le, ge, eq, ne. Keys other than
                                    le, ge, eq, ne, name are variables; {terms: {...}, le}
                                    also works. Interval variables: {"a.end": 1, le: 10}.
    {allDifferent: ["x", "y", "z"]}
    {element: [10, 20, 30], index: "i", value: "v"}       v = array[i], i is 0-based
    {table: ["x", "y"], rows: [[1, 2], [2, 3]]}          (x, y) is one of the rows;
                                    rows may be objects {x: 1, y: 2}
    {count: ["x", "y", "z"], value: 3, le|ge|eq: n}      how many equal value
    {max: ["x", "y"], eq: "m"}   {min: [...], eq: "m"}   m = max / min
    {noOverlap: ["a", "b"]}                              intervals do not overlap
    {cumulative: {a: 2, b: 1}, capacity: 2}              demand per interval; or
                                    cumulative: ["a", "b"] for demand 1 each
    {before: "a", after: "b", gap?: n}                   a.end + gap <= b.start
    {circuit: ["n0", "n1", "n2"]}   one tour: the value of variable k is the position (0-based)
                                    in this list of the node after node k
  minimize | maximize: {var: coef} or "var"; "makespan" = latest interval end
  all: true      list every solution (no objective): solutions: [values, ...]
  limits: {nodes?, solutions?}   defaults 1000000, 1000.   timeLimit: ms (default: 90%
                 of the eval time left)

OUTPUT
  {status, objective, bound, gap, values, schedule?, solutions?, complete, stopped?,
   nodes, fails, horizon?}
  values: every variable, interval ones included ("a.start", "a.end"), and makespan.
  objective, values and schedule are null without a solution. bound and gap are null
  without an objective. complete: the search finished (status proven).
  stopped: "time" | "nodes" | "solutions" when it did not.

LIMITS
  Integers only: |value| <= 1e12, |coef| <= 1e6, at most 1048576 values per domain.
  Not supported: real numbers (use Solver.lp), optional intervals, intervals with a variable
  duration, reified constraints (write them linearly with 0/1 variables).`

type cpParse struct {
	m        *cp.Model
	idx      map[string]int
	names    []string // declared order, for values
	declared map[string]bool
	ivs      []cpInterval
	ividx    map[string]int
	fail     func(string, ...any)
}

type cpInterval struct {
	id         string
	start, end int
	dur        int64
	startLo    int64
	startHi    *int64
	endLo      *int64
	endHi      *int64
}

var cpKeys = []string{"vars", "intervals", "constraints", "minimize", "maximize", "all", "horizon", "limits", "timeLimit"}

var cpKinds = []string{"allDifferent", "element", "table", "count", "max", "min", "noOverlap", "cumulative", "before", "circuit"}

func cpSolve(sh *Shell, args []Value) Value {
	const cmd = "Cp.solve"
	if len(args) != 1 {
		panic(WrongArguments(cmd, "model: object", args, cpExample))
	}
	in, ok := args[0].(*ObjectVal)
	if !ok {
		panic(WrongArguments(cmd, "model: object", args, cpExample))
	}
	fail := func(f string, a ...any) { panic(Runtime(cmd + ": " + fmt.Sprintf(f, a...))) }
	for _, k := range in.Keys() {
		if !contains(cpKeys, k) {
			fail("unknown key %q. Keys: %s", k, strings.Join(cpKeys, ", "))
		}
	}
	p := &cpParse{m: &cp.Model{}, idx: map[string]int{}, declared: map[string]bool{}, ividx: map[string]int{}, fail: fail}
	get := func(k string) (Value, bool) {
		v, ok := in.Get(k)
		if ok && v == Null {
			return nil, false
		}
		return v, ok
	}
	if v, ok := get("vars"); ok {
		p.parseVars(v)
	}
	var cons []Value
	if v, ok := get("constraints"); ok {
		a, isArr := v.(*ArrayVal)
		if !isArr {
			fail("constraints must be an array of rows, got %s", describe(v))
		}
		cons = a.Elements
	}
	horizon, horizonSet := int64(0), false
	if v, ok := get("horizon"); ok {
		horizon, horizonSet = cpInt("horizon", v, fail), true
	}
	if v, ok := get("intervals"); ok {
		p.parseIntervals(v)
	}
	if len(p.ivs) > 0 && !horizonSet {
		horizon = p.defaultHorizon(cons)
	}
	p.makeIntervalVars(horizon)
	for i, c := range cons {
		p.parseConstraint(i, c)
	}

	// Objective.
	maxO, hasMax := get("maximize")
	minO, hasMin := get("minimize")
	if hasMax && hasMin {
		fail("give one objective: minimize or maximize, not both")
	}
	if hasMax || hasMin {
		o, sense := minO, cp.Minimize
		if hasMax {
			o, sense = maxO, cp.Maximize
		}
		p.m.Sense = sense
		p.parseObjective(o)
	}
	all := false
	if v, ok := get("all"); ok {
		all = boolOf("all", v, fail)
		if all && p.m.Objective != nil {
			fail("all: true lists solutions and takes no objective; drop minimize/maximize, or drop all")
		}
	}
	lim := cp.Limits{All: all}
	if lo, ok := get("limits"); ok {
		o, isObj := lo.(*ObjectVal)
		if !isObj {
			fail("limits must be an object {nodes?, solutions?}, got %s", describe(lo))
		}
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			n := cpInt("limits."+k, v, fail)
			if n < 1 || n > math.MaxInt32 {
				fail("limits.%s must be a positive integer, got %d", k, n)
			}
			switch k {
			case "nodes":
				lim.MaxNodes = int(n)
			case "solutions":
				lim.MaxSolutions = int(n)
			default:
				fail("limits: unknown key %q. Keys: nodes, solutions", k)
			}
		}
	}
	timeLimit := 0
	if v, ok := get("timeLimit"); ok {
		n := cpInt("timeLimit", v, fail)
		if n < 1 {
			fail("timeLimit must be a positive number of ms, got %d", n)
		}
		timeLimit = int(min(n, math.MaxInt32))
	}
	if len(p.m.Vars) == 0 {
		fail("no variables. Give vars: {x: [lo, hi]} or intervals")
	}

	ctx, done := routeContext(sh, rfail(cmd), timeLimit)
	defer done()
	res, err := cp.Solve(ctx, p.m, lim)
	if err != nil {
		if errors.Is(err, cp.ErrInvalid) {
			fail("%s", strings.TrimPrefix(err.Error(), "cp: invalid model: "))
		}
		fail("%v", err)
	}
	if sh.Limits().Cancelled() {
		fail("Execution cancelled")
	}
	out := p.result(res, horizon, all)
	switch {
	case res.Stop == "time":
		how := "raise the eval time limit (" + sh.Limits().RaiseHint("timeout", sh.Limits().TimeoutMs*2) + ")"
		if timeLimit > 0 {
			how = "raise timeLimit"
		}
		sh.Note(fmt.Sprintf("%s: the time limit stopped the search after %d nodes; status %q is not proven. To search longer, %s", cmd, res.Nodes, res.Status.String(), how))
	case res.Stop == "nodes":
		sh.Note(fmt.Sprintf("%s: limits.nodes (%d) stopped the search; status %q is not proven. Raise limits.nodes to search longer", cmd, res.Nodes, res.Status.String()))
	case res.Stop == "solutions":
		sh.Note(fmt.Sprintf("%s: limits.solutions stopped the listing at %d; there may be more", cmd, len(res.Solutions)))
	}
	if res.Status == cp.Infeasible && len(p.ivs) > 0 && !horizonSet {
		sh.Note(fmt.Sprintf("%s: infeasible within the default horizon %d. If intervals may end later, set horizon", cmd, horizon))
	}
	return out
}

const cpExample = `Cp.solve({vars: {x: [0, 9], y: [0, 9]}, constraints: [{allDifferent: ["x", "y"]}, {x: 1, y: 1, eq: 9}], maximize: {x: 1}})`

// cpInt reads an exact integer.
func cpInt(what string, v Value, fail func(string, ...any)) int64 {
	n, ok := v.(*NumberVal)
	if !ok {
		fail("%s must be an integer, got %s", what, describe(v))
	}
	r, exact := n.Exact()
	if !exact || !r.IsInt() || !r.Num().IsInt64() {
		fail("%s must be an integer, got %s. Cp works on integers; scale the data (cents, minutes) or use Solver.lp", what, n.Display())
	}
	return r.Num().Int64()
}

func (p *cpParse) addVar(name string, v cp.Var) int {
	if _, dup := p.idx[name]; dup {
		p.fail("variable %q is declared twice", name)
	}
	v.Name = name
	p.idx[name] = len(p.m.Vars)
	p.m.Vars = append(p.m.Vars, v)
	p.names = append(p.names, name)
	return p.idx[name]
}

func (p *cpParse) ref(where, name string) int {
	i, ok := p.idx[name]
	if !ok {
		hint := ""
		if _, isIv := p.ividx[name]; isIv {
			hint = fmt.Sprintf(`. %q is an interval: use "%s.start" or "%s.end"`, name, name, name)
		} else if len(p.names) > 0 {
			shown := p.names
			if len(shown) > 8 {
				shown = append(append([]string(nil), shown[:8]...), "...")
			}
			hint = " (variables: " + strings.Join(shown, ", ") + ")"
		}
		p.fail("%s: unknown variable %q%s. Declare it in vars", where, name, hint)
	}
	return i
}

func (p *cpParse) names2(where string, v Value) []int {
	a, ok := v.(*ArrayVal)
	if !ok {
		p.fail("%s must be an array of variable names, got %s", where, describe(v))
	}
	var out []int
	for _, e := range a.Elements {
		out = append(out, p.ref(where, keyOf(where, e, p.fail)))
	}
	return out
}

func (p *cpParse) domain(where string, v Value) cp.Var {
	switch x := v.(type) {
	case *ArrayVal:
		if len(x.Elements) != 2 {
			p.fail("%s must be [lo, hi] (a range) or {values: [...]}, got %s", where, describe(v))
		}
		return cp.Var{Lo: cpInt(where+" lo", x.Elements[0], p.fail), Hi: cpInt(where+" hi", x.Elements[1], p.fail)}
	case *ObjectVal:
		var out cp.Var
		has := map[string]bool{}
		for _, k := range x.Keys() {
			val, _ := x.Get(k)
			has[k] = true
			switch k {
			case "values":
				a, ok := val.(*ArrayVal)
				if !ok || len(a.Elements) == 0 {
					p.fail("%s.values must be a non-empty array of integers, got %s", where, describe(val))
				}
				for i, e := range a.Elements {
					out.Values = append(out.Values, cpInt(fmt.Sprintf("%s.values[%d]", where, i), e, p.fail))
				}
			case "lo":
				out.Lo = cpInt(where+".lo", val, p.fail)
			case "hi":
				out.Hi = cpInt(where+".hi", val, p.fail)
			case "id":
			default:
				p.fail("%s: unknown key %q. Keys: lo, hi or values", where, k)
			}
		}
		if has["values"] == (has["lo"] || has["hi"]) || has["lo"] != has["hi"] && !has["values"] {
			p.fail("%s needs lo and hi, or values. Got %s", where, x.Display())
		}
		return out
	}
	p.fail("%s must be [lo, hi] or {values: [...]}, got %s", where, describe(v))
	return cp.Var{}
}

func (p *cpParse) parseVars(v Value) {
	switch x := v.(type) {
	case *ObjectVal:
		for _, k := range x.Keys() {
			d, _ := x.Get(k)
			p.declared[k] = true
			p.addVar(k, p.domain("vars."+k, d))
		}
	case *ArrayVal:
		for i, r := range x.Elements {
			where := fmt.Sprintf("vars[%d]", i)
			o, ok := r.(*ObjectVal)
			if !ok {
				p.fail("%s must be an object {id, lo, hi} or {id, values}, got %s", where, describe(r))
			}
			idv, ok := o.Get("id")
			if !ok {
				p.fail("%s has no id. Row: %s", where, o.Display())
			}
			name := keyOf(where+" id", idv, p.fail)
			p.declared[name] = true
			p.addVar(name, p.domain(where, o))
		}
	default:
		p.fail("vars must be an object {x: [lo, hi]} or rows [{id, lo, hi}], got %s", describe(v))
	}
}

func (p *cpParse) parseIntervals(v Value) {
	one := func(where, id string, o *ObjectVal, rows bool) {
		iv := cpInterval{id: id}
		hasDur := false
		bound := func(what string, val Value) (lo, hi *int64) {
			switch x := val.(type) {
			case *NumberVal:
				n := cpInt(where+"."+what, x, p.fail)
				return &n, &n
			case *ArrayVal:
				if len(x.Elements) != 2 {
					p.fail("%s.%s must be [lo, hi] or a number, got %s", where, what, describe(val))
				}
				a := cpInt(where+"."+what+" lo", x.Elements[0], p.fail)
				b := cpInt(where+"."+what+" hi", x.Elements[1], p.fail)
				return &a, &b
			}
			p.fail("%s.%s must be [lo, hi] or a number, got %s", where, what, describe(val))
			return nil, nil
		}
		for _, k := range o.Keys() {
			val, _ := o.Get(k)
			switch k {
			case "duration":
				iv.dur = cpInt(where+".duration", val, p.fail)
				if iv.dur < 0 {
					p.fail("%s.duration must be >= 0, got %d", where, iv.dur)
				}
				hasDur = true
			case "start":
				lo, hi := bound("start", val)
				iv.startLo, iv.startHi = *lo, hi
			case "end":
				iv.endLo, iv.endHi = bound("end", val)
			case "id":
			default:
				if !rows {
					p.fail("%s: unknown key %q. Keys: duration, start, end", where, k)
				}
			}
		}
		if !hasDur {
			p.fail("%s needs duration. Got %s", where, o.Display())
		}
		if _, dup := p.ividx[id]; dup {
			p.fail("%s: interval %q is declared twice", where, id)
		}
		p.ividx[id] = len(p.ivs)
		p.ivs = append(p.ivs, iv)
	}
	switch x := v.(type) {
	case *ObjectVal:
		for _, k := range x.Keys() {
			r, _ := x.Get(k)
			o, ok := r.(*ObjectVal)
			if !ok {
				p.fail("intervals.%s must be {duration, start?, end?}, got %s", k, describe(r))
			}
			one("intervals."+k, k, o, false)
		}
	case *ArrayVal:
		for i, r := range x.Elements {
			where := fmt.Sprintf("intervals[%d]", i)
			o, ok := r.(*ObjectVal)
			if !ok {
				p.fail("%s must be an object {id, duration, start?, end?}, got %s", where, describe(r))
			}
			idv, ok := o.Get("id")
			if !ok {
				p.fail("%s has no id. Row: %s", where, o.Display())
			}
			one(where, keyOf(where+" id", idv, p.fail), o, true)
		}
	default:
		p.fail("intervals must be an object {a: {duration}} or rows [{id, duration}], got %s", describe(v))
	}
}

// defaultHorizon: every interval can run one after another, after the
// latest given start bound, with every before-gap added.
func (p *cpParse) defaultHorizon(cons []Value) int64 {
	var h, maxStart int64
	for _, iv := range p.ivs {
		h += iv.dur
		maxStart = max(maxStart, iv.startLo)
		if iv.endLo != nil {
			maxStart = max(maxStart, *iv.endLo)
		}
	}
	for _, c := range cons {
		if o, ok := c.(*ObjectVal); ok {
			if _, isBefore := o.Get("before"); isBefore {
				if g, ok := o.Get("gap"); ok {
					if n, ok := g.(*NumberVal); ok && n.V > 0 && n.V < 1e12 {
						h += int64(math.Ceil(n.V))
					}
				}
			}
		}
	}
	return min(h+maxStart, cp.MaxAbsValue)
}

func (p *cpParse) makeIntervalVars(horizon int64) {
	for k := range p.ivs {
		iv := &p.ivs[k]
		hi := horizon - iv.dur
		if iv.startHi != nil {
			hi = min(hi, *iv.startHi)
		}
		lo := iv.startLo
		if iv.endLo != nil {
			lo = max(lo, *iv.endLo-iv.dur)
		}
		if iv.endHi != nil {
			hi = min(hi, *iv.endHi-iv.dur)
		}
		if hi < lo {
			p.fail("interval %q cannot fit: start in [%d, %d] (horizon %d, duration %d). Widen its start/end or set horizon", iv.id, lo, hi, horizon, iv.dur)
		}
		iv.start = p.addVar(iv.id+".start", cp.Var{Lo: lo, Hi: hi})
		iv.end = p.addVar(iv.id+".end", cp.Var{Lo: lo + iv.dur, Hi: hi + iv.dur})
		p.m.Cons = append(p.m.Cons, cp.Constraint{Kind: cp.Linear, Vars: []int{iv.end, iv.start}, Coefs: []int64{1, -1}, Op: cp.EQ, RHS: iv.dur})
		p.m.SearchFirst = append(p.m.SearchFirst, iv.start)
	}
}

func (p *cpParse) interval(where string, v Value) cpInterval {
	id := keyOf(where, v, p.fail)
	k, ok := p.ividx[id]
	if !ok {
		p.fail("%s: unknown interval %q. Declare it in intervals", where, id)
	}
	return p.ivs[k]
}

var cpOps = map[string]cp.Op{"le": cp.LE, "ge": cp.GE, "eq": cp.EQ, "ne": cp.NE}

func (p *cpParse) parseConstraint(i int, r Value) {
	where := fmt.Sprintf("constraints[%d]", i)
	o, ok := r.(*ObjectVal)
	if !ok {
		p.fail("%s must be an object, got %s", where, describe(r))
	}
	if nv, ok := o.Get("name"); ok {
		if s, isStr := nv.(*StringVal); isStr {
			where += fmt.Sprintf(" (%q)", s.V)
		}
	}
	kind := ""
	for _, k := range o.Keys() {
		if contains(cpKinds, k) {
			if kind != "" {
				p.fail("%s has two kinds, %s and %s; give one per row", where, kind, k)
			}
			kind = k
		}
	}
	allowed := map[string][]string{
		"allDifferent": {"allDifferent"}, "element": {"element", "index", "value"},
		"table": {"table", "rows"}, "count": {"count", "value", "le", "ge", "eq"},
		"max": {"max", "eq"}, "min": {"min", "eq"}, "noOverlap": {"noOverlap"},
		"cumulative": {"cumulative", "capacity"}, "before": {"before", "after", "gap"},
		"circuit": {"circuit"},
	}
	if kind != "" {
		keys := append(allowed[kind], "name")
		for _, k := range o.Keys() {
			if !contains(keys, k) {
				p.fail("%s: unknown key %q for %s. Keys: %s", where, k, kind, strings.Join(keys, ", "))
			}
		}
	}
	need := func(k string) Value {
		v, ok := o.Get(k)
		if !ok || v == Null {
			p.fail("%s: %s needs %s. Row: %s", where, kind, k, o.Display())
		}
		return v
	}
	opOf := func(ops ...string) (cp.Op, int64) {
		found := ""
		for _, k := range ops {
			if _, ok := o.Get(k); ok {
				if found != "" {
					p.fail("%s needs exactly one of %s. Row: %s", where, strings.Join(ops, ", "), o.Display())
				}
				found = k
			}
		}
		if found == "" {
			p.fail("%s needs exactly one of %s. Row: %s", where, strings.Join(ops, ", "), o.Display())
		}
		v, _ := o.Get(found)
		return cpOps[found], cpInt(where+" "+found, v, p.fail)
	}
	add := func(c cp.Constraint) { p.m.Cons = append(p.m.Cons, c) }
	switch kind {
	case "":
		p.linear(where, o, add)
	case "allDifferent":
		add(cp.Constraint{Kind: cp.AllDifferent, Vars: p.names2(where+" allDifferent", need("allDifferent"))})
	case "element":
		arr, ok := need("element").(*ArrayVal)
		if !ok || len(arr.Elements) == 0 {
			p.fail("%s: element must be a non-empty array of integers", where)
		}
		var vals []int64
		for k, e := range arr.Elements {
			vals = append(vals, cpInt(fmt.Sprintf("%s element[%d]", where, k), e, p.fail))
		}
		ix := p.ref(where+" index", keyOf(where+" index", need("index"), p.fail))
		val := p.ref(where+" value", keyOf(where+" value", need("value"), p.fail))
		add(cp.Constraint{Kind: cp.Element, Vars: []int{ix, val}, Array: vals})
	case "table":
		vars := p.names2(where+" table", need("table"))
		names := need("table").(*ArrayVal).Elements
		rows, ok := need("rows").(*ArrayVal)
		if !ok {
			p.fail("%s: rows must be an array of rows", where)
		}
		var tuples [][]int64
		for k, row := range rows.Elements {
			rw := fmt.Sprintf("%s rows[%d]", where, k)
			var t []int64
			switch x := row.(type) {
			case *ArrayVal:
				if len(x.Elements) != len(vars) {
					p.fail("%s has %d values for %d variables", rw, len(x.Elements), len(vars))
				}
				for _, e := range x.Elements {
					t = append(t, cpInt(rw, e, p.fail))
				}
			case *ObjectVal:
				for _, nm := range names {
					e, ok := x.Get(keyOf(rw, nm, p.fail))
					if !ok {
						p.fail("%s has no key %s. Row: %s", rw, nm.Display(), x.Display())
					}
					t = append(t, cpInt(rw, e, p.fail))
				}
			default:
				p.fail("%s must be an array or object, got %s", rw, describe(row))
			}
			tuples = append(tuples, t)
		}
		add(cp.Constraint{Kind: cp.Table, Vars: vars, Tuples: tuples})
	case "count":
		vars := p.names2(where+" count", need("count"))
		op, rhs := opOf("le", "ge", "eq")
		add(cp.Constraint{Kind: cp.Count, Vars: vars, Value: cpInt(where+" value", need("value"), p.fail), Op: op, RHS: rhs})
	case "max", "min":
		vars := p.names2(where+" "+kind, need(kind))
		if len(vars) == 0 {
			p.fail("%s: %s needs at least one variable", where, kind)
		}
		t := p.ref(where+" eq", keyOf(where+" eq", need("eq"), p.fail))
		k := cp.Max
		if kind == "min" {
			k = cp.Min
		}
		add(cp.Constraint{Kind: k, Vars: append([]int{t}, vars...)})
	case "noOverlap":
		a, ok := need("noOverlap").(*ArrayVal)
		if !ok {
			p.fail("%s: noOverlap must be an array of interval ids", where)
		}
		var tasks []cp.Task
		for _, e := range a.Elements {
			iv := p.interval(where+" noOverlap", e)
			tasks = append(tasks, cp.Task{Start: iv.start, Dur: iv.dur, Demand: 1})
		}
		add(cp.Constraint{Kind: cp.NoOverlap, Tasks: tasks})
	case "cumulative":
		var tasks []cp.Task
		switch x := need("cumulative").(type) {
		case *ArrayVal:
			for _, e := range x.Elements {
				iv := p.interval(where+" cumulative", e)
				tasks = append(tasks, cp.Task{Start: iv.start, Dur: iv.dur, Demand: 1})
			}
		case *ObjectVal:
			for _, k := range x.Keys() {
				d, _ := x.Get(k)
				iv := p.interval(where+" cumulative", Str(k))
				dem := cpInt(where+" cumulative."+k, d, p.fail)
				if dem < 0 {
					p.fail("%s: demand of %q must be >= 0", where, k)
				}
				tasks = append(tasks, cp.Task{Start: iv.start, Dur: iv.dur, Demand: dem})
			}
		default:
			p.fail("%s: cumulative must be {interval: demand} or [interval ids]", where)
		}
		capacity := cpInt(where+" capacity", need("capacity"), p.fail)
		add(cp.Constraint{Kind: cp.Cumulative, Tasks: tasks, Capacity: capacity})
	case "before":
		a := p.interval(where+" before", need("before"))
		b := p.interval(where+" after", need("after"))
		var gap int64
		if g, ok := o.Get("gap"); ok && g != Null {
			gap = cpInt(where+" gap", g, p.fail)
		}
		// b.start - a.end >= gap
		add(cp.Constraint{Kind: cp.Linear, Vars: []int{b.start, a.end}, Coefs: []int64{1, -1}, Op: cp.GE, RHS: gap})
	case "circuit":
		add(cp.Constraint{Kind: cp.Circuit, Vars: p.names2(where+" circuit", need("circuit"))})
	}
}

func (p *cpParse) linear(where string, o *ObjectVal, add func(cp.Constraint)) {
	c := cp.Constraint{Kind: cp.Linear}
	coef := map[int]int64{}
	var order []int
	term := func(name string, v Value, label string) {
		j := p.ref(where, name)
		if _, seen := coef[j]; !seen {
			order = append(order, j)
		}
		coef[j] += cpInt(label, v, p.fail)
	}
	var terms *ObjectVal
	if tv, ok := o.Get("terms"); ok {
		t, isObj := tv.(*ObjectVal)
		if !isObj {
			p.fail("%s: terms must be an object {var: coef}, got %s", where, describe(tv))
		}
		terms = t
	}
	nOp := 0
	for _, k := range o.Keys() {
		v, _ := o.Get(k)
		switch k {
		case "le", "ge", "eq", "ne":
			nOp++
			c.Op, c.RHS = cpOps[k], cpInt(where+" "+k, v, p.fail)
		case "name", "terms":
		default:
			if terms != nil {
				p.fail("%s: with terms, the other keys are le, ge, eq, ne, name; got %q", where, k)
			}
			term(k, v, where+" "+k)
		}
	}
	if terms != nil {
		for _, k := range terms.Keys() {
			v, _ := terms.Get(k)
			term(k, v, where+" terms."+k)
		}
	}
	if nOp != 1 {
		p.fail("%s needs exactly one of le, ge, eq, ne (got %d), or a kind: %s. Keys other than le, ge, eq, ne, name, terms are variables. Row: %s",
			where, nOp, strings.Join(cpKinds, ", "), o.Display())
	}
	for _, j := range order {
		c.Vars = append(c.Vars, j)
		c.Coefs = append(c.Coefs, coef[j])
	}
	add(c)
}

func (p *cpParse) parseObjective(o Value) {
	switch x := o.(type) {
	case *StringVal:
		if _, ok := p.idx[x.V]; !ok && x.V == "makespan" {
			if len(p.ivs) == 0 {
				p.fail(`minimize: "makespan" needs intervals`)
			}
			var lo, hi int64
			for _, iv := range p.ivs {
				lo = max(lo, p.m.Vars[iv.end].Lo)
				hi = max(hi, p.m.Vars[iv.end].Hi)
			}
			mk := p.addVar("makespan", cp.Var{Lo: lo, Hi: hi})
			vars := []int{mk}
			for _, iv := range p.ivs {
				vars = append(vars, iv.end)
			}
			p.m.Cons = append(p.m.Cons, cp.Constraint{Kind: cp.Max, Vars: vars})
		}
		p.m.Objective = []cp.Term{{Var: p.ref("objective", x.V), Coef: 1}}
	case *ObjectVal:
		p.m.Objective = []cp.Term{}
		for _, k := range x.Keys() {
			v, _ := x.Get(k)
			p.m.Objective = append(p.m.Objective, cp.Term{Var: p.ref("objective", k), Coef: cpInt("objective "+k, v, p.fail)})
		}
	default:
		p.fail(`the objective must be {var: coef} or a variable name such as "makespan", got %s`, describe(o))
	}
}

func (p *cpParse) values(x []int64) Value {
	o := NewObject()
	for _, n := range p.names {
		o.Set(n, numInt64(x[p.idx[n]]))
	}
	return o
}

func numInt64(n int64) Value { return NumRat(new(big.Rat).SetInt64(n)) }

func (p *cpParse) result(res *cp.Result, horizon int64, all bool) Value {
	out := NewObject()
	out.Set("status", Str(res.Status.String()))
	have := res.Values != nil && res.Status != cp.Infeasible && res.Status != cp.Unknown
	hasObj := len(p.m.Objective) > 0
	if have && hasObj {
		out.Set("objective", numInt64(res.Objective))
	} else {
		out.Set("objective", Null)
	}
	if hasObj && (have || res.Status == cp.Unknown) {
		out.Set("bound", numInt64(res.Bound))
	} else {
		out.Set("bound", Null)
	}
	if hasObj && have {
		gap := 0.0
		if res.Objective != 0 {
			gap = math.Abs(float64(res.Objective-res.Bound)) / math.Abs(float64(res.Objective))
		} else if res.Bound != res.Objective {
			gap = 1
		}
		out.Set("gap", Num(math.Round(gap*1e4)/1e4))
	} else {
		out.Set("gap", Null)
	}
	if have {
		out.Set("values", p.values(res.Values))
	} else {
		out.Set("values", Null)
	}
	if len(p.ivs) > 0 {
		if have {
			ivs := append([]cpInterval(nil), p.ivs...)
			sort.SliceStable(ivs, func(a, b int) bool { return res.Values[ivs[a].start] < res.Values[ivs[b].start] })
			rows := make([]Value, 0, len(ivs))
			for _, iv := range ivs {
				r := NewObject()
				r.Set("id", Str(iv.id))
				r.Set("start", numInt64(res.Values[iv.start]))
				r.Set("end", numInt64(res.Values[iv.end]))
				r.Set("duration", numInt64(iv.dur))
				rows = append(rows, r)
			}
			out.Set("schedule", &ArrayVal{Elements: rows})
		} else {
			out.Set("schedule", Null)
		}
		out.Set("horizon", numInt64(horizon))
	}
	if all {
		sols := make([]Value, 0, len(res.Solutions))
		for _, s := range res.Solutions {
			sols = append(sols, p.values(s))
		}
		out.Set("solutions", &ArrayVal{Elements: sols})
	}
	out.Set("complete", Bln(res.Complete))
	if res.Stop != "" {
		out.Set("stopped", Str(res.Stop))
	}
	out.Set("nodes", Num(float64(res.Nodes)))
	out.Set("fails", Num(float64(res.Fails)))
	return out
}
