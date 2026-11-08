package solver

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

// --- helpers ---

func ts(pairs ...any) []Term {
	var out []Term
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Term{pairs[i].(string), toRat(pairs[i+1])})
	}
	return out
}

func toRat(v any) *big.Rat {
	switch x := v.(type) {
	case int:
		return Int(int64(x))
	case *big.Rat:
		return x
	case string:
		r, ok := new(big.Rat).SetString(x)
		if !ok {
			panic(x)
		}
		return r
	}
	panic("bad rat")
}

func con(op Op, rhs any, terms ...any) Constraint {
	return Constraint{Terms: ts(terms...), Op: op, RHS: toRat(rhs)}
}

func vars(names ...string) []Var {
	var vs []Var
	for _, n := range names {
		vs = append(vs, Var{Name: n})
	}
	return vs
}

func solve(t *testing.T, m *Model) *Result {
	t.Helper()
	r, err := Solve(context.Background(), m, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	checkFeasible(t, m, r)
	return r
}

func wantObj(t *testing.T, r *Result, obj string) {
	t.Helper()
	if r.Status != Optimal {
		t.Fatalf("status %v, want optimal", r.Status)
	}
	if r.Objective.Cmp(toRat(obj)) != 0 {
		t.Fatalf("objective %s, want %s", r.Objective.RatString(), obj)
	}
}

func wantVals(t *testing.T, m *Model, r *Result, kv ...any) {
	t.Helper()
	for i := 0; i < len(kv); i += 2 {
		got := r.Value(m, kv[i].(string))
		if got == nil || got.Cmp(toRat(kv[i+1])) != 0 {
			t.Fatalf("%s = %v, want %v", kv[i], got, kv[i+1])
		}
	}
}

// checkFeasible verifies bounds, integrality, constraints, slack and the
// reported objective of a solution.
func checkFeasible(t *testing.T, m *Model, r *Result) {
	t.Helper()
	if r.Values == nil {
		return
	}
	val := map[string]*big.Rat{}
	for i, v := range m.Vars {
		x := r.Values[i]
		val[v.Name] = x
		lo := v.Lo
		if lo == nil && !v.NoLo {
			lo = new(big.Rat)
		}
		if lo != nil && x.Cmp(lo) < 0 || v.Hi != nil && x.Cmp(v.Hi) > 0 {
			t.Fatalf("%s=%s violates bounds", v.Name, x.RatString())
		}
		if v.Integer && !x.IsInt() {
			t.Fatalf("%s=%s not integer", v.Name, x.RatString())
		}
	}
	eval := func(ts []Term) *big.Rat {
		s := new(big.Rat)
		for _, tm := range ts {
			s.Add(s, new(big.Rat).Mul(tm.Coef, val[tm.Var]))
		}
		return s
	}
	for i, c := range m.Cons {
		l := eval(c.Terms)
		cmp := l.Cmp(c.RHS)
		if c.Op == LE && cmp > 0 || c.Op == GE && cmp < 0 || c.Op == EQ && cmp != 0 {
			t.Fatalf("constraint %d violated: %s %v %s", i, l.RatString(), c.Op, c.RHS.RatString())
		}
		var want *big.Rat
		switch c.Op {
		case LE:
			want = new(big.Rat).Sub(c.RHS, l)
		case GE:
			want = new(big.Rat).Sub(l, c.RHS)
		default:
			want = new(big.Rat)
		}
		if r.Slack[i].Cmp(want) != 0 {
			t.Fatalf("slack %d = %s, want %s", i, r.Slack[i].RatString(), want.RatString())
		}
	}
	if eval(m.Objective).Cmp(r.Objective) != 0 {
		t.Fatalf("objective mismatch")
	}
}

// --- textbook LPs ---

func TestDantzig(t *testing.T) {
	m := &Model{Sense: Maximize, Vars: vars("x", "y"),
		Objective: ts("x", 3, "y", 5),
		Cons: []Constraint{
			con(LE, 4, "x", 1),
			con(LE, 12, "y", 2),
			con(LE, 18, "x", 3, "y", 2),
		}}
	r := solve(t, m)
	wantObj(t, r, "36")
	wantVals(t, m, r, "x", 2, "y", 6)
	if r.Slack[2].Sign() != 0 || r.Slack[0].Cmp(Int(2)) != 0 {
		t.Fatal("slack")
	}
}

func TestFractionalOptimum(t *testing.T) {
	// max x+y s.t. 3x+2y<=7, x+3y<=6 -> x=9/7, y=11/7, obj 20/7
	m := &Model{Sense: Maximize, Vars: vars("x", "y"),
		Objective: ts("x", 1, "y", 1),
		Cons:      []Constraint{con(LE, 7, "x", 3, "y", 2), con(LE, 6, "x", 1, "y", 3)}}
	r := solve(t, m)
	wantObj(t, r, "20/7")
	wantVals(t, m, r, "x", "9/7", "y", "11/7")
}

func TestDiet(t *testing.T) {
	m := &Model{Sense: Minimize, Vars: vars("x", "y"),
		Objective: ts("x", 2, "y", 3),
		Cons:      []Constraint{con(GE, 12, "x", 3, "y", 2), con(GE, 8, "x", 1, "y", 2)}}
	r := solve(t, m)
	wantObj(t, r, "13")
	wantVals(t, m, r, "x", 2, "y", 3)
}

func TestTransportation(t *testing.T) {
	// supply 30,20; demand 25,25; cost [[4,6],[5,3]]; optimum 190.
	m := &Model{Sense: Minimize, Vars: vars("x11", "x12", "x21", "x22"),
		Objective: ts("x11", 4, "x12", 6, "x21", 5, "x22", 3),
		Cons: []Constraint{
			con(EQ, 30, "x11", 1, "x12", 1),
			con(EQ, 20, "x21", 1, "x22", 1),
			con(EQ, 25, "x11", 1, "x21", 1),
			con(EQ, 25, "x12", 1, "x22", 1),
		}}
	r := solve(t, m)
	wantObj(t, r, "190")
	wantVals(t, m, r, "x11", 25, "x12", 5, "x21", 0, "x22", 20)
}

func TestBealeCycling(t *testing.T) {
	// Beale's example: cycles under Dantzig's rule with naive tie-breaking.
	m := &Model{Sense: Minimize, Vars: vars("x4", "x5", "x6", "x7"),
		Objective: ts("x4", "-3/4", "x5", 20, "x6", "-1/2", "x7", 6),
		Cons: []Constraint{
			con(LE, 0, "x4", "1/4", "x5", -8, "x6", -1, "x7", 9),
			con(LE, 0, "x4", "1/2", "x5", -12, "x6", "-1/2", "x7", 3),
			con(LE, 1, "x6", 1),
		}}
	r := solve(t, m)
	wantObj(t, r, "-5/4")
	wantVals(t, m, r, "x4", 1, "x6", 1)
}

func TestKuhnCycling(t *testing.T) {
	// Another classic cycling example (Kuhn): optimum is -2.
	m := &Model{Sense: Minimize, Vars: vars("a", "b", "c", "d"),
		Objective: ts("a", -2, "b", -3, "c", 1, "d", 12),
		Cons: []Constraint{
			con(LE, 0, "a", -2, "b", -9, "c", 1, "d", 9),
			con(LE, 0, "a", "1/3", "b", 1, "c", "-1/3", "d", -2),
			con(LE, 2, "a", 1),
			con(LE, 2, "b", 1),
		}}
	r := solve(t, m)
	if r.Status != Optimal {
		t.Fatal(r.Status)
	}
	bruteLPCheck(t, m, r)
}

func TestInfeasible(t *testing.T) {
	m := &Model{Sense: Maximize, Vars: vars("x"), Objective: ts("x", 1),
		Cons: []Constraint{con(LE, 1, "x", 1), con(GE, 2, "x", 1)}}
	if r := solve(t, m); r.Status != Infeasible || r.Values != nil {
		t.Fatal(r.Status)
	}
}

func TestUnbounded(t *testing.T) {
	m := &Model{Sense: Maximize, Vars: vars("x", "y"), Objective: ts("x", 1, "y", 1),
		Cons: []Constraint{con(LE, 3, "x", 1, "y", -1)}}
	if r := solve(t, m); r.Status != Unbounded {
		t.Fatal(r.Status)
	}
	// Minimizing over a free variable.
	m = &Model{Sense: Minimize, Vars: []Var{{Name: "x", NoLo: true}}, Objective: ts("x", 1)}
	if r := solve(t, m); r.Status != Unbounded {
		t.Fatal(r.Status)
	}
}

func TestEqualityAndRedundant(t *testing.T) {
	// Duplicate equality rows leave a redundant artificial row.
	m := &Model{Sense: Maximize, Vars: vars("x", "y"), Objective: ts("x", 1, "y", 2),
		Cons: []Constraint{
			con(EQ, 4, "x", 1, "y", 1),
			con(EQ, 8, "x", 2, "y", 2),
			con(LE, 3, "y", 1),
		}}
	r := solve(t, m)
	wantObj(t, r, "7")
	wantVals(t, m, r, "x", 1, "y", 3)
}

func TestNegativeAndFreeVars(t *testing.T) {
	// x in [-5,-1], y free, z <= 4 with no lower bound.
	m := &Model{Sense: Minimize,
		Vars: []Var{
			{Name: "x", Lo: Int(-5), Hi: Int(-1)},
			{Name: "y", NoLo: true},
			{Name: "z", NoLo: true, Hi: Int(4)},
		},
		Objective: ts("x", 1, "y", 1, "z", -1),
		Cons: []Constraint{
			con(GE, -10, "y", 1),
			con(GE, -2, "x", 1, "y", 1, "z", 1),
		}}
	r := solve(t, m)
	// z=4 (max), y=-10, x=-5: x+y+z=-11 < -2 infeasible; so constraint binds.
	bruteLPCheck(t, m, r)
	// Free variable with an equality.
	m = &Model{Sense: Minimize, Vars: []Var{{Name: "u", NoLo: true}, {Name: "v", NoLo: true}},
		Objective: ts("u", 1, "v", 2),
		Cons:      []Constraint{con(EQ, 3, "u", 1, "v", -1), con(GE, -4, "u", 1), con(GE, -1, "v", 1)}}
	r = solve(t, m)
	// u = v+3, objective 3v+3, v >= -1 -> v=-1, u=2, objective 0.
	wantObj(t, r, "0")
	wantVals(t, m, r, "u", 2, "v", -1)
}

func TestEmptyConstraintsBoundsOnly(t *testing.T) {
	m := &Model{Sense: Maximize, Vars: []Var{{Name: "x", Hi: Rat(7, 2)}}, Objective: ts("x", 2)}
	r := solve(t, m)
	wantObj(t, r, "7")
}

// --- MILP ---

func TestKnapsackSmall(t *testing.T) {
	w := []int{12, 2, 1, 1, 4}
	v := []int{4, 2, 1, 2, 10}
	m := knapsack(w, v, 15)
	r := solve(t, m)
	wantObj(t, r, "15")
}

func knapsack(w, v []int, cap int) *Model {
	m := &Model{Sense: Maximize}
	var cw []Term
	for i := range w {
		n := fmt.Sprintf("i%d", i)
		m.Vars = append(m.Vars, Var{Name: n, Hi: Int(1), Integer: true})
		m.Objective = append(m.Objective, Term{n, Int(int64(v[i]))})
		cw = append(cw, Term{n, Int(int64(w[i]))})
	}
	m.Cons = []Constraint{{cw, LE, Int(int64(cap))}}
	return m
}

func TestKnapsackRounding(t *testing.T) {
	// LP relaxation is fractional: max 5a+4b s.t. 6a+4b<=9 -> int optimum 5? a=1,b=0: 5; a=0,b=2 no (b<=1) -> a=1,b=0=5; b=1,a=0=4.
	m := &Model{Sense: Maximize, Vars: []Var{{Name: "a", Hi: Int(1), Integer: true}, {Name: "b", Hi: Int(1), Integer: true}},
		Objective: ts("a", 5, "b", 4), Cons: []Constraint{con(LE, 9, "a", 6, "b", 4)}}
	r := solve(t, m)
	wantObj(t, r, "5")
	if r.Nodes < 2 {
		t.Fatalf("expected branching, nodes=%d", r.Nodes)
	}
}

func TestAssignment(t *testing.T) {
	cost := [][]int{{9, 2, 7, 8}, {6, 4, 3, 7}, {5, 8, 1, 8}, {7, 6, 9, 4}}
	n := len(cost)
	m := &Model{Sense: Minimize}
	name := func(i, j int) string { return fmt.Sprintf("x%d%d", i, j) }
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			m.Vars = append(m.Vars, Var{Name: name(i, j), Hi: Int(1), Integer: true})
			m.Objective = append(m.Objective, Term{name(i, j), Int(int64(cost[i][j]))})
		}
	}
	for i := 0; i < n; i++ {
		var row, col []Term
		for j := 0; j < n; j++ {
			row = append(row, Term{name(i, j), Int(1)})
			col = append(col, Term{name(j, i), Int(1)})
		}
		m.Cons = append(m.Cons, Constraint{row, EQ, Int(1)}, Constraint{col, EQ, Int(1)})
	}
	r := solve(t, m)
	// brute force over permutations
	best := -1
	var perm func(k int, used int, sum int)
	perm = func(k, used, sum int) {
		if k == n {
			if best < 0 || sum < best {
				best = sum
			}
			return
		}
		for j := 0; j < n; j++ {
			if used&(1<<j) == 0 {
				perm(k+1, used|1<<j, sum+cost[k][j])
			}
		}
	}
	perm(0, 0, 0)
	wantObj(t, r, fmt.Sprint(best))
}

func TestMILPInfeasibleAndUnbounded(t *testing.T) {
	// 2x = 1 has no integer solution.
	m := &Model{Sense: Maximize, Vars: []Var{{Name: "x", Integer: true, Hi: Int(5)}}, Objective: ts("x", 1),
		Cons: []Constraint{con(EQ, 1, "x", 2)}}
	if r := solve(t, m); r.Status != Infeasible {
		t.Fatal(r.Status)
	}
	m = &Model{Sense: Maximize, Vars: []Var{{Name: "x", Integer: true}}, Objective: ts("x", 1)}
	if r := solve(t, m); r.Status != Unbounded {
		t.Fatal(r.Status)
	}
}

func TestMixedInteger(t *testing.T) {
	// max x + 2y; x integer, y continuous; 2x+y<=7/2... x<=1 integer, y<=3/2.
	m := &Model{Sense: Maximize,
		Vars:      []Var{{Name: "x", Integer: true}, {Name: "y"}},
		Objective: ts("x", 1, "y", 2),
		Cons:      []Constraint{con(LE, "7/2", "x", 2, "y", 1), con(LE, "5/2", "y", 1)}}
	r := solve(t, m)
	// x=0,y=5/2 -> 5 ; x=1,y=3/2 -> 4. optimum 5
	wantObj(t, r, "5")
}

// --- validation ---

func TestValidation(t *testing.T) {
	cases := map[string]*Model{
		"empty":    {},
		"unknown":  {Vars: vars("x"), Cons: []Constraint{con(LE, 1, "y", 1)}},
		"unkobj":   {Vars: vars("x"), Objective: ts("q", 1)},
		"lo>hi":    {Vars: []Var{{Name: "x", Lo: Int(3), Hi: Int(2)}}},
		"dup":      {Vars: vars("x", "x")},
		"noname":   {Vars: []Var{{}}},
		"nilrhs":   {Vars: vars("x"), Cons: []Constraint{{Terms: ts("x", 1), Op: LE}}},
		"nilcoef":  {Vars: vars("x"), Cons: []Constraint{{Terms: []Term{{"x", nil}}, Op: LE, RHS: Int(1)}}},
		"intempty": {Vars: []Var{{Name: "x", Integer: true, Lo: Rat(1, 3), Hi: Rat(2, 3)}}},
	}
	for name, m := range cases {
		_, err := Solve(context.Background(), m, Limits{})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}

func TestSizeLimits(t *testing.T) {
	m := &Model{Vars: vars("a", "b", "c")}
	_, err := Solve(context.Background(), m, Limits{MaxVars: 2})
	var le *LimitError
	if !errors.As(err, &le) || le.Limit != "variables" {
		t.Fatal(err)
	}
	m.Cons = []Constraint{con(LE, 1, "a", 1), con(LE, 1, "b", 1)}
	_, err = Solve(context.Background(), m, Limits{MaxCons: 1})
	if !errors.As(err, &le) || le.Limit != "constraints" {
		t.Fatal(err)
	}
	m = &Model{Sense: Maximize, Vars: vars("x", "y"), Objective: ts("x", 3, "y", 5),
		Cons: []Constraint{con(LE, 4, "x", 1), con(LE, 12, "y", 2), con(LE, 18, "x", 3, "y", 2)}}
	_, err = Solve(context.Background(), m, Limits{MaxPivots: 1})
	if !errors.As(err, &le) || le.Limit != "pivots" {
		t.Fatal(err)
	}
}

// parity problem: sum 2x_i = n (n odd) over binaries: LP feasible, MILP
// infeasible, and branch and bound needs an exponential tree.
func parity(n int) *Model {
	m := &Model{Sense: Maximize}
	var cs []Term
	for i := 0; i < n; i++ {
		nm := fmt.Sprintf("x%d", i)
		m.Vars = append(m.Vars, Var{Name: nm, Hi: Int(1), Integer: true})
		m.Objective = append(m.Objective, Term{nm, Int(1)})
		cs = append(cs, Term{nm, Int(2)})
	}
	m.Cons = []Constraint{{cs, EQ, Int(int64(n))}}
	return m
}

func TestNodeLimit(t *testing.T) {
	m := parity(25)
	start := time.Now()
	r, err := Solve(context.Background(), m, Limits{MaxNodes: 40})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != NodeLimit || r.Nodes != 40 {
		t.Fatalf("status %v nodes %d", r.Status, r.Nodes)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("slow")
	}
	// With an incumbent: a 30-item knapsack (275 nodes to solve) cut short.
	rng := rand.New(rand.NewSource(3))
	w := make([]int, 30)
	v := make([]int, 30)
	sum := 0
	for i := range w {
		w[i] = 10 + rng.Intn(40)
		v[i] = w[i] + rng.Intn(10)
		sum += w[i]
	}
	k := knapsack(w, v, sum/2)
	r, err = Solve(context.Background(), k, Limits{MaxNodes: 60})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%v nodes=%d obj=%v bound=%v gap=%v", r.Status, r.Nodes, r.Objective, r.BestBound, r.Gap)
	if r.Status != NodeLimit || r.Values == nil {
		t.Fatalf("want node-limit with incumbent, got %v", r.Status)
	}
	checkFeasible(t, k, r)
	if r.Gap == nil || r.BestBound == nil || r.Gap.Sign() < 0 {
		t.Fatal("gap/bound missing")
	}
	if d := new(big.Rat).Sub(r.BestBound, r.Objective); d.Cmp(r.Gap) != 0 {
		t.Fatal("gap != bound - objective")
	}
}

func TestContextCancel(t *testing.T) {
	m := parity(41)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Solve(ctx, m, Limits{MaxNodes: 1 << 30})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, err := Solve(ctx2, m, Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

// --- brute force cross-checks ---

// bruteInt enumerates all integer points in the box and returns the best
// objective (user sense), or nil if none is feasible.
func bruteInt(m *Model) *big.Rat {
	n := len(m.Vars)
	idx := map[string]int{}
	for i, v := range m.Vars {
		idx[v.Name] = i
	}
	x := make([]int64, n)
	var best *big.Rat
	var rec func(k int)
	rec = func(k int) {
		if k == n {
			ev := func(ts []Term) *big.Rat {
				s := new(big.Rat)
				for _, tm := range ts {
					s.Add(s, new(big.Rat).Mul(tm.Coef, Int(x[idx[tm.Var]])))
				}
				return s
			}
			for _, c := range m.Cons {
				cmp := ev(c.Terms).Cmp(c.RHS)
				if c.Op == LE && cmp > 0 || c.Op == GE && cmp < 0 || c.Op == EQ && cmp != 0 {
					return
				}
			}
			o := ev(m.Objective)
			if best == nil || m.Sense == Maximize && o.Cmp(best) > 0 || m.Sense == Minimize && o.Cmp(best) < 0 {
				best = o
			}
			return
		}
		lo, hi := m.Vars[k].Lo.Num().Int64(), m.Vars[k].Hi.Num().Int64()
		for v := lo; v <= hi; v++ {
			x[k] = v
			rec(k + 1)
		}
	}
	rec(0)
	return best
}

func randModel(rng *rand.Rand, integer bool) *Model {
	n := 2 + rng.Intn(3)
	m := &Model{Sense: Sense(rng.Intn(2))}
	for i := 0; i < n; i++ {
		lo := int64(rng.Intn(7) - 3)
		hi := lo + int64(rng.Intn(5))
		m.Vars = append(m.Vars, Var{Name: fmt.Sprintf("v%d", i), Lo: Int(lo), Hi: Int(hi), Integer: integer})
		m.Objective = append(m.Objective, Term{fmt.Sprintf("v%d", i), Int(int64(rng.Intn(11) - 5))})
	}
	for k := 1 + rng.Intn(4); k > 0; k-- {
		var c Constraint
		for i := 0; i < n; i++ {
			if rng.Intn(4) > 0 {
				c.Terms = append(c.Terms, Term{fmt.Sprintf("v%d", i), Int(int64(rng.Intn(9) - 4))})
			}
		}
		c.Op = Op(rng.Intn(3))
		c.RHS = Int(int64(rng.Intn(13) - 4))
		if c.Op == EQ && rng.Intn(2) == 0 {
			c.Op = LE
		}
		m.Cons = append(m.Cons, c)
	}
	return m
}

func TestMILPvsBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const cases = 400
	counts := map[Status]int{}
	for i := 0; i < cases; i++ {
		m := randModel(rng, true)
		r, err := Solve(context.Background(), m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		checkFeasible(t, m, r)
		want := bruteInt(m)
		counts[r.Status]++
		switch {
		case want == nil && r.Status != Infeasible:
			t.Fatalf("case %d: got %v, want infeasible\n%+v", i, r.Status, m)
		case want != nil && (r.Status != Optimal || r.Objective.Cmp(want) != 0):
			t.Fatalf("case %d: got %v %v, want %s", i, r.Status, r.Objective, want.RatString())
		}
	}
	t.Logf("MILP brute-force cross-check: %d cases, optimal=%d infeasible=%d", cases, counts[Optimal], counts[Infeasible])
	if counts[Optimal] < 100 || counts[Infeasible] < 20 {
		t.Fatalf("poor coverage: %v", counts)
	}
}

// bruteLP enumerates every vertex of {constraints, bounds}: choose n tight
// hyperplanes, solve, test feasibility. Returns the best objective or nil.
// All variables must have finite bounds so the feasible set is a polytope.
func bruteLP(m *Model) *big.Rat {
	n := len(m.Vars)
	idx := map[string]int{}
	for i, v := range m.Vars {
		idx[v.Name] = i
	}
	type hp struct {
		a   []*big.Rat
		rhs *big.Rat
	}
	var hs []hp
	row := func(ts []Term) []*big.Rat {
		a := zeros(n)
		for _, tm := range ts {
			a[idx[tm.Var]].Add(a[idx[tm.Var]], tm.Coef)
		}
		return a
	}
	for _, c := range m.Cons {
		hs = append(hs, hp{row(c.Terms), c.RHS})
	}
	for i, v := range m.Vars {
		a := zeros(n)
		a[i].SetInt64(1)
		hs = append(hs, hp{a, v.Lo}, hp{a, v.Hi})
	}
	feasible := func(x []*big.Rat) bool {
		for i, v := range m.Vars {
			if x[i].Cmp(v.Lo) < 0 || x[i].Cmp(v.Hi) > 0 {
				return false
			}
		}
		for _, c := range m.Cons {
			l := dot(row(c.Terms), x)
			cmp := l.Cmp(c.RHS)
			if c.Op == LE && cmp > 0 || c.Op == GE && cmp < 0 || c.Op == EQ && cmp != 0 {
				return false
			}
		}
		return true
	}
	obj := row(m.Objective)
	var best *big.Rat
	pick := make([]int, n)
	var rec func(k, from int)
	rec = func(k, from int) {
		if k == n {
			A := make([][]*big.Rat, n)
			for i, h := range pick {
				A[i] = append(cloneR(hs[h].a), hs[h].rhs)
			}
			x := gauss(A, n)
			if x == nil || !feasible(x) {
				return
			}
			o := dot(obj, x)
			if best == nil || m.Sense == Maximize && o.Cmp(best) > 0 || m.Sense == Minimize && o.Cmp(best) < 0 {
				best = o
			}
			return
		}
		for h := from; h < len(hs); h++ {
			pick[k] = h
			rec(k+1, h+1)
		}
	}
	rec(0, 0)
	return best
}

func dot(a, b []*big.Rat) *big.Rat {
	s := new(big.Rat)
	for i := range a {
		s.Add(s, new(big.Rat).Mul(a[i], b[i]))
	}
	return s
}

// gauss solves the n x n system (augmented matrix); nil if singular.
func gauss(A [][]*big.Rat, n int) []*big.Rat {
	for c := 0; c < n; c++ {
		p := -1
		for r := c; r < n; r++ {
			if A[r][c].Sign() != 0 {
				p = r
				break
			}
		}
		if p < 0 {
			return nil
		}
		A[c], A[p] = A[p], A[c]
		inv := new(big.Rat).Inv(A[c][c])
		for j := c; j <= n; j++ {
			A[c][j] = new(big.Rat).Mul(A[c][j], inv)
		}
		for r := 0; r < n; r++ {
			if r == c || A[r][c].Sign() == 0 {
				continue
			}
			f := new(big.Rat).Set(A[r][c])
			for j := c; j <= n; j++ {
				A[r][j] = new(big.Rat).Sub(A[r][j], new(big.Rat).Mul(f, A[c][j]))
			}
		}
	}
	x := make([]*big.Rat, n)
	for i := range x {
		x[i] = A[i][n]
	}
	return x
}

func bruteLPCheck(t *testing.T, m *Model, r *Result) {
	t.Helper()
	// Needs finite bounds; add them if absent (large box) only when the
	// solution lies strictly inside, which the caller guarantees by use.
	mm := *m
	mm.Vars = nil
	for _, v := range m.Vars {
		if v.Lo == nil {
			if v.NoLo {
				v.Lo = Int(-1000)
			} else {
				v.Lo = Int(0)
			}
		}
		if v.Hi == nil {
			v.Hi = Int(1000)
		}
		mm.Vars = append(mm.Vars, v)
	}
	want := bruteLP(&mm)
	if want == nil || r.Objective.Cmp(want) != 0 {
		t.Fatalf("objective %v, brute-force vertex enumeration %v", r.Objective, want)
	}
}

func TestLPvsVertexEnumeration(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const cases = 400
	counts := map[Status]int{}
	for i := 0; i < cases; i++ {
		m := randModel(rng, false)
		// Fractional data too.
		for k := range m.Objective {
			m.Objective[k].Coef = Rat(int64(rng.Intn(21)-10), int64(1+rng.Intn(4)))
		}
		r, err := Solve(context.Background(), m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		checkFeasible(t, m, r)
		want := bruteLP(m)
		counts[r.Status]++
		switch {
		case want == nil && r.Status != Infeasible:
			t.Fatalf("case %d: got %v, want infeasible", i, r.Status)
		case want != nil && (r.Status != Optimal || r.Objective.Cmp(want) != 0):
			t.Fatalf("case %d: got %v %v, want %s", i, r.Status, r.Objective, want.RatString())
		}
	}
	t.Logf("LP vertex-enumeration cross-check: %d cases, optimal=%d infeasible=%d", cases, counts[Optimal], counts[Infeasible])
	if counts[Optimal] < 100 || counts[Infeasible] < 10 {
		t.Fatalf("poor coverage: %v", counts)
	}
}

// Degenerate random LPs: many zero right-hand sides provoke ties.
func TestDegenerateRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for i := 0; i < 200; i++ {
		m := randModel(rng, false)
		for k := range m.Cons {
			if rng.Intn(2) == 0 {
				m.Cons[k].RHS = Int(0)
			}
		}
		r, err := Solve(context.Background(), m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		checkFeasible(t, m, r)
		want := bruteLP(m)
		if want == nil && r.Status != Infeasible || want != nil && (r.Status != Optimal || r.Objective.Cmp(want) != 0) {
			t.Fatalf("case %d: got %v %v, want %v", i, r.Status, r.Objective, want)
		}
	}
}

// --- performance (reported, not asserted) ---

func TestPerfDenseLP(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	rng := rand.New(rand.NewSource(1))
	const n, mc = 100, 100
	m := &Model{Sense: Maximize}
	for j := 0; j < n; j++ {
		nm := fmt.Sprintf("x%d", j)
		m.Vars = append(m.Vars, Var{Name: nm})
		m.Objective = append(m.Objective, Term{nm, Int(int64(1 + rng.Intn(9)))})
	}
	for i := 0; i < mc; i++ {
		c := Constraint{Op: LE, RHS: Int(int64(100 + rng.Intn(900)))}
		for j := 0; j < n; j++ {
			c.Terms = append(c.Terms, Term{fmt.Sprintf("x%d", j), Int(int64(1 + rng.Intn(9)))})
		}
		m.Cons = append(m.Cons, c)
	}
	start := time.Now()
	r := solve(t, m)
	t.Logf("PERF dense LP %dx%d: %v, status=%v pivots=%d", n, mc, time.Since(start), r.Status, r.Pivots)
}

func TestPerfKnapsack30(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	rng := rand.New(rand.NewSource(3))
	w := make([]int, 30)
	v := make([]int, 30)
	sum := 0
	for i := range w {
		w[i] = 10 + rng.Intn(40)
		v[i] = w[i] + rng.Intn(10)
		sum += w[i]
	}
	m := knapsack(w, v, sum/2)
	start := time.Now()
	r := solve(t, m)
	t.Logf("PERF knapsack 30 items: %v, status=%v nodes=%d pivots=%d obj=%v", time.Since(start), r.Status, r.Nodes, r.Pivots, r.Objective)
}
