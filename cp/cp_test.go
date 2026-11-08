package cp

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// satisfies checks one assignment against the model, independently of the
// propagators.
func satisfies(m *Model, x []int64) bool {
	for i, v := range m.Vars {
		if v.Values != nil {
			ok := false
			for _, y := range v.Values {
				ok = ok || y == x[i]
			}
			if !ok {
				return false
			}
		} else if x[i] < v.Lo || x[i] > v.Hi {
			return false
		}
	}
	for _, c := range m.Cons {
		switch c.Kind {
		case Linear:
			var s int64
			for k, v := range c.Vars {
				s += c.Coefs[k] * x[v]
			}
			if !cmpOp(s, c.Op, c.RHS) {
				return false
			}
		case AllDifferent:
			seen := map[int64]bool{}
			for _, v := range c.Vars {
				if seen[x[v]] {
					return false
				}
				seen[x[v]] = true
			}
		case Element:
			i := x[c.Vars[0]]
			if i < 0 || i >= int64(len(c.Array)) || c.Array[i] != x[c.Vars[1]] {
				return false
			}
		case Table:
			ok := false
			for _, t := range c.Tuples {
				match := true
				for k, v := range c.Vars {
					match = match && t[k] == x[v]
				}
				ok = ok || match
			}
			if !ok {
				return false
			}
		case Count:
			var n int64
			for _, v := range c.Vars {
				if x[v] == c.Value {
					n++
				}
			}
			if !cmpOp(n, c.Op, c.RHS) {
				return false
			}
		case Max, Min:
			best := x[c.Vars[1]]
			for _, v := range c.Vars[2:] {
				if c.Kind == Max {
					best = max(best, x[v])
				} else {
					best = min(best, x[v])
				}
			}
			if x[c.Vars[0]] != best {
				return false
			}
		case Cumulative, NoOverlap:
			capacity := c.Capacity
			if c.Kind == NoOverlap {
				capacity = 1
			}
			for _, t := range c.Tasks {
				if t.Dur == 0 {
					continue
				}
				// Check at every task start: the load there is the maximum.
				at := x[t.Start]
				var load int64
				for _, u := range c.Tasks {
					d := u.Demand
					if c.Kind == NoOverlap {
						d = 1
					}
					if u.Dur > 0 && x[u.Start] <= at && at < x[u.Start]+u.Dur {
						load += d
					}
				}
				if load > capacity {
					return false
				}
			}
		case Circuit:
			n := len(c.Vars)
			cur, steps := 0, 0
			seen := map[int]bool{}
			for {
				nx := x[c.Vars[cur]]
				if nx < 0 || nx >= int64(n) || seen[cur] {
					return false
				}
				seen[cur] = true
				cur = int(nx)
				steps++
				if cur == 0 {
					break
				}
			}
			if steps != n {
				return false
			}
		}
	}
	return true
}

func cmpOp(a int64, op Op, b int64) bool {
	switch op {
	case LE:
		return a <= b
	case GE:
		return a >= b
	case EQ:
		return a == b
	}
	return a != b
}

// brute enumerates every assignment: the number of solutions and the best
// objective (minimization form of the model's sense).
func brute(m *Model) (int, int64, bool) {
	x := make([]int64, len(m.Vars))
	count := 0
	var best int64
	have := false
	var rec func(i int)
	rec = func(i int) {
		if i == len(m.Vars) {
			if !satisfies(m, x) {
				return
			}
			count++
			var o int64
			for _, t := range m.Objective {
				o += t.Coef * x[t.Var]
			}
			if m.Sense == Maximize {
				o = -o
			}
			if !have || o < best {
				best, have = o, true
			}
			return
		}
		v := m.Vars[i]
		vals := v.Values
		if vals == nil {
			for y := v.Lo; y <= v.Hi; y++ {
				vals = append(vals, y)
			}
		}
		tried := map[int64]bool{}
		for _, y := range vals {
			if tried[y] {
				continue
			}
			tried[y] = true
			x[i] = y
			rec(i + 1)
		}
	}
	rec(0)
	return count, best, have
}

func randModelCP(rng *rand.Rand) *Model {
	n := 2 + rng.Intn(4)
	m := &Model{}
	for i := 0; i < n; i++ {
		v := Var{Name: fmt.Sprintf("x%d", i)}
		if rng.Intn(4) == 0 {
			for k := rng.Intn(3) + 1; k > 0; k-- {
				v.Values = append(v.Values, int64(rng.Intn(9)-3))
			}
		} else {
			v.Lo = int64(rng.Intn(5) - 2)
			v.Hi = v.Lo + int64(rng.Intn(5))
		}
		m.Vars = append(m.Vars, v)
	}
	pick := func(k int) []int {
		p := rng.Perm(n)
		return p[:k]
	}
	for c := 1 + rng.Intn(3); c > 0; c-- {
		switch rng.Intn(10) {
		case 0, 1:
			k := 1 + rng.Intn(n)
			vs := pick(k)
			var a []int64
			for range vs {
				a = append(a, int64(rng.Intn(7)-3))
			}
			m.Cons = append(m.Cons, Constraint{Kind: Linear, Vars: vs, Coefs: a, Op: Op(rng.Intn(4)), RHS: int64(rng.Intn(11) - 5)})
		case 2:
			m.Cons = append(m.Cons, Constraint{Kind: AllDifferent, Vars: pick(2 + rng.Intn(n-1))})
		case 3:
			var arr []int64
			for k := 1 + rng.Intn(5); k > 0; k-- {
				arr = append(arr, int64(rng.Intn(7)-2))
			}
			m.Cons = append(m.Cons, Constraint{Kind: Element, Vars: pick(2), Array: arr})
		case 4:
			vs := pick(1 + rng.Intn(min(n, 3)))
			var tuples [][]int64
			for k := rng.Intn(8); k > 0; k-- {
				var t []int64
				for range vs {
					t = append(t, int64(rng.Intn(6)-2))
				}
				tuples = append(tuples, t)
			}
			m.Cons = append(m.Cons, Constraint{Kind: Table, Vars: vs, Tuples: tuples})
		case 5:
			m.Cons = append(m.Cons, Constraint{Kind: Count, Vars: pick(1 + rng.Intn(n)), Value: int64(rng.Intn(4) - 1), Op: Op(rng.Intn(3)), RHS: int64(rng.Intn(3))})
		case 6:
			k := Max
			if rng.Intn(2) == 0 {
				k = Min
			}
			m.Cons = append(m.Cons, Constraint{Kind: k, Vars: pick(2 + rng.Intn(n-1))})
		case 7, 8:
			var tasks []Task
			for _, v := range pick(1 + rng.Intn(n)) {
				tasks = append(tasks, Task{Start: v, Dur: int64(rng.Intn(4)), Demand: int64(1 + rng.Intn(3))})
			}
			if rng.Intn(2) == 0 {
				m.Cons = append(m.Cons, Constraint{Kind: NoOverlap, Tasks: tasks})
			} else {
				m.Cons = append(m.Cons, Constraint{Kind: Cumulative, Tasks: tasks, Capacity: int64(rng.Intn(5))})
			}
		case 9:
			k := 1 + rng.Intn(n)
			vs := pick(k)
			for _, v := range vs { // successors need domains that reach 0..k-1
				m.Vars[v].Values = nil
				m.Vars[v].Lo, m.Vars[v].Hi = int64(rng.Intn(2)-1), int64(k-1+rng.Intn(2))
			}
			m.Cons = append(m.Cons, Constraint{Kind: Circuit, Vars: vs})
		}
	}
	if rng.Intn(3) > 0 {
		m.Sense = Sense(rng.Intn(2))
		for i := 0; i < n; i++ {
			if rng.Intn(3) > 0 {
				m.Objective = append(m.Objective, Term{i, int64(rng.Intn(9) - 4)})
			}
		}
	}
	if rng.Intn(3) == 0 {
		m.SearchFirst = pick(1 + rng.Intn(n))
	}
	return m
}

func TestRandomVsBrute(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	ctx := context.Background()
	stats := map[string]int{}
	for i := 0; i < 3000; i++ {
		m := randModelCP(rng)
		count, best, have := brute(m)
		// Every solution, exactly once.
		all, err := Solve(ctx, m, Limits{All: true, MaxSolutions: 1 << 20})
		if errors.Is(err, ErrInvalid) && m.Objective != nil {
			mm := *m
			mm.Objective = nil
			all, err = Solve(ctx, &mm, Limits{All: true, MaxSolutions: 1 << 20})
		}
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		seen := map[string]bool{}
		for _, s := range all.Solutions {
			if !satisfies(m, s) {
				t.Fatalf("case %d: bad solution %v\n%+v", i, s, m)
			}
			k := fmt.Sprint(s)
			if seen[k] {
				t.Fatalf("case %d: duplicate solution %v", i, s)
			}
			seen[k] = true
		}
		if len(all.Solutions) != count || !all.Complete {
			t.Fatalf("case %d: %d solutions (complete %v), brute force %d\n%+v", i, len(all.Solutions), all.Complete, count, m)
		}
		r, err := Solve(ctx, m, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case !have:
			if r.Status != Infeasible {
				t.Fatalf("case %d: got %v, want infeasible", i, r.Status)
			}
			stats["infeasible"]++
		case len(m.Objective) == 0:
			if r.Status != Feasible || !satisfies(m, r.Values) {
				t.Fatalf("case %d: got %v %v", i, r.Status, r.Values)
			}
			stats["satisfy"]++
		default:
			want := best
			if m.Sense == Maximize {
				want = -best
			}
			if r.Status != Optimal || r.Objective != want || r.Bound != want || !satisfies(m, r.Values) {
				t.Fatalf("case %d: got %v obj %d bound %d, want optimal %d\n%+v", i, r.Status, r.Objective, r.Bound, want, m)
			}
			stats["optimal"]++
		}
		for _, c := range m.Cons {
			stats[c.Kind.String()]++
		}
	}
	t.Logf("%v", stats)
	if stats["infeasible"] < 300 || stats["optimal"] < 300 || stats["satisfy"] < 100 {
		t.Fatalf("poor coverage: %v", stats)
	}
}

// n-queens: 92 solutions for n = 8.
func TestQueens(t *testing.T) {
	const n = 8
	m := &Model{}
	var q, d1, d2 []int
	for i := 0; i < n; i++ {
		m.Vars = append(m.Vars, Var{Name: fmt.Sprint("q", i), Lo: 0, Hi: n - 1},
			Var{Name: fmt.Sprint("a", i), Lo: -n, Hi: 2 * n}, Var{Name: fmt.Sprint("b", i), Lo: -n, Hi: 2 * n})
		qi, ai, bi := 3*i, 3*i+1, 3*i+2
		q, d1, d2 = append(q, qi), append(d1, ai), append(d2, bi)
		m.Cons = append(m.Cons,
			Constraint{Kind: Linear, Vars: []int{ai, qi}, Coefs: []int64{1, -1}, Op: EQ, RHS: int64(i)},
			Constraint{Kind: Linear, Vars: []int{bi, qi}, Coefs: []int64{1, 1}, Op: EQ, RHS: int64(i)})
	}
	m.Cons = append(m.Cons, Constraint{Kind: AllDifferent, Vars: q}, Constraint{Kind: AllDifferent, Vars: d1}, Constraint{Kind: AllDifferent, Vars: d2})
	r, err := Solve(context.Background(), m, Limits{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Solutions) != 92 || !r.Complete {
		t.Fatalf("got %d solutions", len(r.Solutions))
	}
}

// ft06-sized job shop: 3 jobs x 3 machines, known optimum by brute force of
// orders is small; here a hand-checked instance with makespan 11.
func TestJobShop(t *testing.T) {
	// job: [(machine, duration)...]
	jobs := [][][2]int64{
		{{0, 3}, {1, 2}, {2, 2}},
		{{0, 2}, {2, 1}, {1, 4}},
		{{1, 4}, {2, 3}},
	}
	const horizon = 30
	m := &Model{}
	byMachine := map[int64][]Task{}
	var ends []int
	for j, ops := range jobs {
		prev := -1
		var prevDur int64
		for k, op := range ops {
			s := len(m.Vars)
			m.Vars = append(m.Vars, Var{Name: fmt.Sprintf("s%d_%d", j, k), Lo: 0, Hi: horizon})
			m.SearchFirst = append(m.SearchFirst, s)
			byMachine[op[0]] = append(byMachine[op[0]], Task{Start: s, Dur: op[1], Demand: 1})
			if prev >= 0 { // s >= prev + prevDur
				m.Cons = append(m.Cons, Constraint{Kind: Linear, Vars: []int{s, prev}, Coefs: []int64{1, -1}, Op: GE, RHS: prevDur})
			}
			prev, prevDur = s, op[1]
		}
		e := len(m.Vars)
		m.Vars = append(m.Vars, Var{Name: fmt.Sprintf("e%d", j), Lo: 0, Hi: horizon})
		m.Cons = append(m.Cons, Constraint{Kind: Linear, Vars: []int{e, prev}, Coefs: []int64{1, -1}, Op: EQ, RHS: prevDur})
		ends = append(ends, e)
	}
	for _, ts := range byMachine {
		m.Cons = append(m.Cons, Constraint{Kind: NoOverlap, Tasks: ts})
	}
	mk := len(m.Vars)
	m.Vars = append(m.Vars, Var{Name: "makespan", Lo: 0, Hi: horizon})
	m.Cons = append(m.Cons, Constraint{Kind: Max, Vars: append([]int{mk}, ends...)})
	m.Objective = []Term{{mk, 1}}
	r, err := Solve(context.Background(), m, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != Optimal || !satisfies(m, r.Values) {
		t.Fatalf("got %+v", r)
	}
	// Machine 1 carries 2 + 4 + 4 = 10 units, and job 2 must spend 4 on
	// machine 1 before 3 on machine 2, so 10 is a lower bound; check the
	// solver's answer against brute force over start times is too big, so
	// pin the value found by an exhaustive search with All on makespan <= 9.
	t.Logf("makespan %d nodes %d", r.Objective, r.Nodes)
	mm := *m
	mm.Objective = nil
	mm.Cons = append(append([]Constraint(nil), m.Cons...), Constraint{Kind: Linear, Vars: []int{mk}, Coefs: []int64{1}, Op: LE, RHS: r.Objective - 1})
	rr, _ := Solve(context.Background(), &mm, Limits{})
	if rr.Status != Infeasible {
		t.Fatalf("a makespan below %d exists: %v", r.Objective, rr.Status)
	}
}

func TestNodeLimitAndTime(t *testing.T) {
	// A pigeonhole: 9 pigeons, 8 holes, with no global reasoning beyond Hall
	// on bounds... Hall proves it at once, so use a linear version instead.
	m := &Model{}
	for i := 0; i < 12; i++ {
		m.Vars = append(m.Vars, Var{Name: fmt.Sprint("x", i), Lo: 0, Hi: 10})
	}
	var vs []int
	var a []int64
	for i := range m.Vars {
		vs = append(vs, i)
		a = append(a, 2)
	}
	m.Cons = []Constraint{{Kind: Linear, Vars: vs, Coefs: a, Op: EQ, RHS: 61}} // odd: no solution
	m.Cons = append(m.Cons, Constraint{Kind: Linear, Vars: vs[:6], Coefs: a[:6], Op: NE, RHS: 7})
	r, err := Solve(context.Background(), m, Limits{MaxNodes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != Unknown || r.Complete || r.Stop != "nodes" {
		t.Fatalf("got %v complete %v stop %q", r.Status, r.Complete, r.Stop)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r, err = Solve(ctx, m, Limits{MaxNodes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if r.Stop != "time" {
		t.Fatalf("stop %q", r.Stop)
	}
}

func TestValidate(t *testing.T) {
	for name, m := range map[string]*Model{
		"empty":      {},
		"bounds":     {Vars: []Var{{Name: "x", Lo: 2, Hi: 1}}},
		"huge":       {Vars: []Var{{Name: "x", Lo: 0, Hi: 1 << 40}}},
		"coef":       {Vars: []Var{{Name: "x", Lo: 0, Hi: 1}}, Cons: []Constraint{{Kind: Linear, Vars: []int{0}, Coefs: []int64{1 << 30}}}},
		"term":       {Vars: []Var{{Name: "x", Lo: 0, Hi: 1}, {Name: "y", Values: []int64{1e12}}}, Objective: []Term{{1, 1e6}}},
		"index":      {Vars: []Var{{Name: "x", Lo: 0, Hi: 1}}, Cons: []Constraint{{Kind: AllDifferent, Vars: []int{3}}}},
		"count ne":   {Vars: []Var{{Name: "x", Lo: 0, Hi: 1}}, Cons: []Constraint{{Kind: Count, Vars: []int{0}, Op: NE}}},
		"all+object": {Vars: []Var{{Name: "x", Lo: 0, Hi: 1}}, Objective: []Term{{0, 1}}},
	} {
		lim := Limits{All: name == "all+object"}
		if _, err := Solve(context.Background(), m, lim); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

// ft06 (Fisher and Thompson 6x6), optimum makespan 55. Each job row is
// (machine, duration) pairs.
var ft06 = [][]int64{
	{2, 1, 0, 3, 1, 6, 3, 7, 5, 3, 4, 6},
	{1, 8, 2, 5, 4, 10, 5, 10, 0, 10, 3, 4},
	{2, 5, 3, 4, 5, 8, 0, 9, 1, 1, 4, 7},
	{1, 5, 0, 5, 2, 5, 3, 3, 4, 8, 5, 9},
	{2, 9, 1, 3, 4, 5, 5, 4, 0, 3, 3, 1},
	{1, 3, 3, 3, 5, 9, 0, 10, 4, 4, 2, 1},
}

func jobShopModel(jobs [][]int64, horizon int64) (*Model, int) {
	m := &Model{}
	byMachine := map[int64][]Task{}
	var ends []int
	for j, row := range jobs {
		prev := -1
		var prevDur int64
		for k := 0; k < len(row); k += 2 {
			s := len(m.Vars)
			m.Vars = append(m.Vars, Var{Name: fmt.Sprintf("s%d_%d", j, k/2), Lo: 0, Hi: horizon})
			m.SearchFirst = append(m.SearchFirst, s)
			byMachine[row[k]] = append(byMachine[row[k]], Task{Start: s, Dur: row[k+1], Demand: 1})
			if prev >= 0 {
				m.Cons = append(m.Cons, Constraint{Kind: Linear, Vars: []int{s, prev}, Coefs: []int64{1, -1}, Op: GE, RHS: prevDur})
			}
			prev, prevDur = s, row[k+1]
		}
		e := len(m.Vars)
		m.Vars = append(m.Vars, Var{Name: fmt.Sprintf("e%d", j), Lo: 0, Hi: horizon})
		m.Cons = append(m.Cons, Constraint{Kind: Linear, Vars: []int{e, prev}, Coefs: []int64{1, -1}, Op: EQ, RHS: prevDur})
		ends = append(ends, e)
	}
	for mc := int64(0); mc < int64(len(byMachine)); mc++ {
		m.Cons = append(m.Cons, Constraint{Kind: NoOverlap, Tasks: byMachine[mc]})
	}
	mk := len(m.Vars)
	m.Vars = append(m.Vars, Var{Name: "makespan", Lo: 0, Hi: horizon})
	m.Cons = append(m.Cons, Constraint{Kind: Max, Vars: append([]int{mk}, ends...)})
	m.Objective = []Term{{mk, 1}}
	return m, mk
}

// Reported, not asserted beyond validity.
func TestPerfFT06(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	m, _ := jobShopModel(ft06, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	r, err := Solve(ctx, m, Limits{MaxNodes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if r.Values != nil && !satisfies(m, r.Values) {
		t.Fatal("invalid schedule")
	}
	t.Logf("PERF ft06: %v status=%v makespan=%d bound=%d nodes=%d fails=%d stop=%q", time.Since(start), r.Status, r.Objective, r.Bound, r.Nodes, r.Fails, r.Stop)
}
