package routing

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"slices"
	"testing"
	"time"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestHaversineKnown(t *testing.T) {
	cases := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want, tol              float64
	}{
		// 1 degree of longitude on the equator: 2*pi*R/360.
		{"equator 1deg", 0, 0, 0, 1, 2 * math.Pi * EarthRadius / 360, 1e-6},
		// Equator to pole: pi*R/2.
		{"quarter meridian", 0, 0, 90, 0, math.Pi * EarthRadius / 2, 1e-6},
		// Antipodes: pi*R.
		{"antipodes", 0, 0, 0, 180, math.Pi * EarthRadius, 1e-6},
		{"same point", 51.5, -0.12, 51.5, -0.12, 0, 0},
		// Rosetta Code haversine task: Nashville BNA to LAX is 2887.26 km at
		// R = 6372.8 km; scaled to R = 6371.0088 km.
		{"BNA-LAX", 36.12, -86.67, 33.94, -118.40, 2887.2599506071106e3 * EarthRadius / 6372.8e3, 1e-3},
		// London-Paris, published as 343.5 km.
		{"London-Paris", 51.5074, -0.1278, 48.8566, 2.3522, 343_560, 500},
	}
	for _, c := range cases {
		if got := Haversine(c.lat1, c.lon1, c.lat2, c.lon2); !near(got, c.want, c.tol) {
			t.Errorf("%s: got %.3f m, want %.3f", c.name, got, c.want)
		}
		if a, b := Haversine(c.lat1, c.lon1, c.lat2, c.lon2), Haversine(c.lat2, c.lon2, c.lat1, c.lon1); !near(a, b, 1e-6) {
			t.Errorf("%s: not symmetric %v %v", c.name, a, b)
		}
	}
	if got := Manhattan(1, 2, 4, -2); got != 7 {
		t.Errorf("manhattan = %v", got)
	}
	if got := Euclidean(0, 0, 3, 4); got != 5 {
		t.Errorf("euclidean = %v", got)
	}
}

// floyd is the all-pairs reference for the path tests.
func floyd(g *Graph) [][]float64 {
	n := g.N()
	d := make([][]float64, n)
	for i := range d {
		d[i] = make([]float64, n)
		for j := range d[i] {
			if i != j {
				d[i][j] = Inf
			}
		}
		for _, a := range g.Adj[i] {
			d[i][a.To] = math.Min(d[i][a.To], a.W)
		}
	}
	for k := 0; k < n; k++ {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if d[i][k]+d[k][j] < d[i][j] {
					d[i][j] = d[i][k] + d[k][j]
				}
			}
		}
	}
	return d
}

func TestShortestPathSmall(t *testing.T) {
	// a-b 1, b-c 2, a-c 5, c-d 1; directed both ways except d->a 1 one way.
	g := NewGraph(4)
	for _, e := range [][3]float64{{0, 1, 1}, {1, 2, 2}, {0, 2, 5}, {2, 3, 1}} {
		g.AddArc(int(e[0]), int(e[1]), e[2])
		g.AddArc(int(e[1]), int(e[0]), e[2])
	}
	g.AddArc(3, 0, 1)
	ctx := context.Background()
	p, ok, err := ShortestPath(ctx, g, 0, 3, nil)
	if err != nil || !ok || p.Dist != 4 || !slices.Equal(p.Nodes, []int{0, 1, 2, 3}) {
		t.Fatalf("0->3: %v %v %v", p, ok, err)
	}
	p, ok, _ = ShortestPath(ctx, g, 3, 0, nil)
	if !ok || p.Dist != 1 || !slices.Equal(p.Nodes, []int{3, 0}) {
		t.Fatalf("3->0: %v", p)
	}
	h := NewGraph(3)
	h.AddArc(0, 1, 1)
	if _, ok, _ := ShortestPath(ctx, h, 0, 2, nil); ok {
		t.Error("unreachable node reported reachable")
	}
	if _, _, err := ShortestPath(ctx, h, 0, 7, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad node: %v", err)
	}
	ks, err := KShortest(ctx, g, 0, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	var dists []float64
	for _, k := range ks {
		dists = append(dists, k.Dist)
	}
	// 0-1-2-3 (4), 0-2-3 (6). Loopless only.
	if !slices.Equal(dists, []float64{4, 6}) {
		t.Errorf("k-shortest dists = %v", dists)
	}
}

func TestShortestPathRandomAStar(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	ctx := context.Background()
	for c := 0; c < 200; c++ {
		n := 2 + rng.Intn(30)
		xs, ys := make([]float64, n), make([]float64, n)
		for i := range xs {
			xs[i], ys[i] = rng.Float64()*100, rng.Float64()*100
		}
		g := NewGraph(n)
		for e := 0; e < n*3; e++ {
			a, b := rng.Intn(n), rng.Intn(n)
			if a == b {
				continue
			}
			// Weight >= straight line, so the euclidean heuristic is admissible.
			g.AddArc(a, b, Euclidean(xs[a], ys[a], xs[b], ys[b])*(1+rng.Float64()))
		}
		ref := floyd(g)
		for q := 0; q < 5; q++ {
			s, d := rng.Intn(n), rng.Intn(n)
			h := func(v int) float64 { return Euclidean(xs[v], ys[v], xs[d], ys[d]) }
			for _, hh := range []func(int) float64{nil, h} {
				p, ok, err := ShortestPath(ctx, g, s, d, hh)
				if err != nil {
					t.Fatal(err)
				}
				if ok != !math.IsInf(ref[s][d], 1) {
					t.Fatalf("case %d: reachable %v, ref %v", c, ok, ref[s][d])
				}
				if ok && !near(p.Dist, ref[s][d], 1e-9) {
					t.Fatalf("case %d %d->%d: %v want %v (astar %v)", c, s, d, p.Dist, ref[s][d], hh != nil)
				}
				if ok && !near(pathCost(g, p.Nodes), p.Dist, 1e-9) {
					t.Fatalf("path cost mismatch")
				}
			}
		}
		// k-shortest: first equals Dijkstra, non-decreasing, loopless.
		s, d := rng.Intn(n), rng.Intn(n)
		ks, _ := KShortest(ctx, g, s, d, 4)
		for i, k := range ks {
			if i == 0 && !near(k.Dist, ref[s][d], 1e-9) {
				t.Fatalf("k[0] = %v want %v", k.Dist, ref[s][d])
			}
			if i > 0 && k.Dist < ks[i-1].Dist-1e-9 {
				t.Fatalf("k-shortest not sorted: %v", ks)
			}
			seen := map[int]bool{}
			for _, v := range k.Nodes {
				if seen[v] {
					t.Fatalf("loop in %v", k.Nodes)
				}
				seen[v] = true
			}
		}
	}
}

func TestClosure(t *testing.T) {
	m, _ := NewMatrix(3)
	m.Set(0, 1, 2)
	m.Set(1, 2, 3)
	m.Set(2, 0, 1)
	i, j, err := Closure(context.Background(), m)
	if err != nil || i != -1 {
		t.Fatalf("closure: %d %d %v", i, j, err)
	}
	if m.At(0, 2) != 5 || m.At(1, 0) != 4 || m.At(2, 1) != 3 {
		t.Errorf("closure values %v", m.C)
	}
	m2, _ := NewMatrix(2)
	m2.Set(0, 1, 1)
	if i, j, _ := Closure(context.Background(), m2); i != 1 || j != 0 {
		t.Errorf("unreachable pair = %d %d", i, j)
	}
	if _, err := NewMatrix(MaxNodes + 1); err == nil {
		t.Error("no size limit")
	}
}

func randMatrix(rng *rand.Rand, n int, sym, integer bool) *Matrix {
	m, _ := NewMatrix(n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j || (sym && j < i) {
				continue
			}
			v := rng.Float64() * 100
			if integer {
				v = float64(rng.Intn(100))
			}
			m.Set(i, j, v)
			if sym {
				m.Set(j, i, v)
			}
		}
	}
	return m
}

// brute is the reference: every order of the nodes other than start.
func brute(p TSP) float64 {
	n := p.M.N
	var rest []int
	for v := 0; v < n; v++ {
		if v != p.Start {
			rest = append(rest, v)
		}
	}
	best := Inf
	var perm func(k int)
	perm = func(k int) {
		if k == len(rest) {
			if !p.RoundTrip && p.End >= 0 && len(rest) > 0 && rest[len(rest)-1] != p.End {
				return
			}
			c, prev := 0.0, p.Start
			for _, v := range rest {
				c += p.M.At(prev, v)
				prev = v
			}
			if p.RoundTrip {
				c += p.M.At(prev, p.Start)
			}
			best = math.Min(best, c)
			return
		}
		for i := k; i < len(rest); i++ {
			rest[k], rest[i] = rest[i], rest[k]
			perm(k + 1)
			rest[k], rest[i] = rest[i], rest[k]
		}
	}
	perm(0)
	return best
}

// checkTour verifies the order visits every node once and costs Cost.
func checkTour(t *testing.T, p TSP, r *TSPResult) {
	t.Helper()
	n := p.M.N
	o := r.Order
	want := n
	if p.RoundTrip {
		want = n + 1
		if o[len(o)-1] != p.Start {
			t.Fatalf("round trip does not end at start: %v", o)
		}
		o = o[:n]
	}
	if len(r.Order) != want || o[0] != p.Start {
		t.Fatalf("order %v (n %d)", r.Order, n)
	}
	if !p.RoundTrip && p.End >= 0 && o[len(o)-1] != p.End {
		t.Fatalf("does not end at %d: %v", p.End, o)
	}
	seen := make([]bool, n)
	for _, v := range o {
		if seen[v] {
			t.Fatalf("node %d twice: %v", v, r.Order)
		}
		seen[v] = true
	}
	c := 0.0
	for i := 0; i+1 < len(r.Order); i++ {
		c += p.M.At(r.Order[i], r.Order[i+1])
	}
	if !near(c, r.Cost, 1e-6*math.Max(1, c)) {
		t.Fatalf("cost %v, order sums to %v", r.Cost, c)
	}
	if r.Bound > r.Cost+1e-9 {
		t.Fatalf("bound %v > cost %v", r.Bound, r.Cost)
	}
}

func randTSP(rng *rand.Rand, n int) TSP {
	sym := rng.Intn(2) == 0
	integer := rng.Intn(2) == 0
	p := TSP{M: randMatrix(rng, n, sym, integer), Start: rng.Intn(n), End: -1, Integer: integer, Seed: rng.Int63()}
	switch rng.Intn(3) {
	case 0:
		p.RoundTrip = true
	case 1:
		p.End = rng.Intn(n)
	}
	if p.End == p.Start {
		p.End = -1
		p.RoundTrip = true
	}
	return p
}

func TestTSPExactVsBrute(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	ctx := context.Background()
	for c := 0; c < 600; c++ {
		n := 1 + rng.Intn(9)
		p := randTSP(rng, n)
		want := brute(p)
		r, err := SolveTSP(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		checkTour(t, p, r)
		if r.Status != Optimal || r.Method != "held-karp" || !near(r.Cost, want, 1e-9) {
			t.Fatalf("case %d n=%d round=%v end=%d: %v %s cost %v want %v", c, n, p.RoundTrip, p.End, r.Status, r.Method, r.Cost, want)
		}
		// The heuristic on the same instance: never below the optimum, bound
		// never above it, and Optimal only when it is.
		p.ExactMax = -1
		h, err := SolveTSP(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		checkTour(t, p, h)
		if h.Cost < want-1e-9 || h.Bound > want+1e-9 {
			t.Fatalf("case %d heuristic: cost %v bound %v optimum %v", c, h.Cost, h.Bound, want)
		}
		if h.Status == Optimal && !near(h.Cost, want, 1e-9) {
			t.Fatalf("case %d: heuristic claims optimal at %v, optimum %v", c, h.Cost, want)
		}
	}
}

func pointsMatrix(rng *rand.Rand, n int) *Matrix {
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range xs {
		xs[i], ys[i] = rng.Float64()*1000, rng.Float64()*1000
	}
	m, _ := NewMatrix(n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			m.Set(i, j, Euclidean(xs[i], ys[i], xs[j], ys[j]))
		}
	}
	return m
}

func TestTSPHeuristicGap(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	ctx := context.Background()
	for _, n := range []int{50, 80, 120, 200} {
		for _, mode := range []string{"round", "open", "fixed"} {
			p := TSP{M: pointsMatrix(rng, n), Start: 0, End: -1, Seed: 1}
			switch mode {
			case "round":
				p.RoundTrip = true
			case "fixed":
				p.End = n - 1
			}
			start := time.Now()
			r, err := SolveTSP(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			checkTour(t, p, r)
			gap := Gap(r.Cost, r.Bound)
			t.Logf("n=%d %s: cost %.1f bound %.1f gap %.2f%% iters %d %v", n, mode, r.Cost, r.Bound, 100*gap, r.Iterations, time.Since(start))
			if r.Status != Feasible || gap > 0.08 {
				t.Errorf("n=%d %s: status %v gap %.3f", n, mode, r.Status, gap)
			}
		}
	}
	// Asymmetric: the bound is weaker but must stay below the cost.
	p := TSP{M: randMatrix(rng, 60, false, true), Start: 0, End: -1, RoundTrip: true, Integer: true}
	r, _ := SolveTSP(ctx, p)
	checkTour(t, p, r)
}

func TestTSPDeterministic(t *testing.T) {
	m := pointsMatrix(rand.New(rand.NewSource(9)), 120)
	p := TSP{M: m, RoundTrip: true, Seed: 42}
	a, _ := SolveTSP(context.Background(), p)
	b, _ := SolveTSP(context.Background(), p)
	if !slices.Equal(a.Order, b.Order) || a.Cost != b.Cost || a.Bound != b.Bound {
		t.Error("same seed, different result")
	}
}

func TestTSPTimeLimit(t *testing.T) {
	m := pointsMatrix(rand.New(rand.NewSource(5)), 1500)
	p := TSP{M: m, RoundTrip: true, Iterations: 1 << 30}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, err := SolveTSP(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("took %v", el)
	}
	if !errors.Is(r.Stopped, context.DeadlineExceeded) || r.Status != Feasible {
		t.Errorf("stopped %v status %v", r.Stopped, r.Status)
	}
	checkTour(t, p, r)
}

func TestTSPErrors(t *testing.T) {
	m, _ := NewMatrix(3)
	if _, err := SolveTSP(context.Background(), TSP{M: m, RoundTrip: true}); err == nil {
		t.Error("missing arcs accepted")
	}
	if _, err := SolveTSP(context.Background(), TSP{M: &Matrix{}}); err == nil {
		t.Error("empty accepted")
	}
}

// validate is an independent check of a VRP plan: every stop at most once,
// capacity, windows, vehicle limits, and the reported numbers.
func validate(t *testing.T, p *VRP, r *VRPResult) {
	t.Helper()
	seen := map[int]bool{}
	tol := 1e-6
	for _, rt := range r.Routes {
		v := p.Vehicles[rt.Vehicle]
		load, dist := 0.0, 0.0
		prev := v.Start
		tm := 0.0
		for k, i := range rt.Stops {
			if seen[i] {
				t.Fatalf("stop %d twice", i)
			}
			seen[i] = true
			st := p.Stops[i]
			load += st.Demand
			dist += p.Dist.At(prev, st.Node)
			if k == 0 {
				tm = rt.Depart
			}
			tm += p.Time.At(prev, st.Node)
			tm = math.Max(tm, st.Open)
			if tm > st.Close+tol {
				t.Fatalf("stop %d served at %v after close %v", i, tm, st.Close)
			}
			if !near(tm, rt.Arrive[k], tol) {
				t.Fatalf("stop %d arrive %v, want %v", i, rt.Arrive[k], tm)
			}
			tm += st.Service
			prev = st.Node
		}
		dist += p.Dist.At(prev, v.End)
		tm += p.Time.At(prev, v.End)
		if rt.Depart < v.Open-tol || tm > v.Close+tol {
			t.Fatalf("vehicle %d window: depart %v back %v", rt.Vehicle, rt.Depart, tm)
		}
		if load > v.Capacity+tol || dist > v.MaxDist+tol || tm-rt.Depart > v.MaxTime+tol {
			t.Fatalf("vehicle %d over: load %v dist %v time %v", rt.Vehicle, load, dist, tm-rt.Depart)
		}
		if !near(load, rt.Load, tol) || !near(dist, rt.Dist, 1e-6*math.Max(1, dist)) || !near(tm, rt.Back, tol) {
			t.Fatalf("route numbers: load %v/%v dist %v/%v back %v/%v", load, rt.Load, dist, rt.Dist, tm, rt.Back)
		}
	}
	for _, u := range r.Unassigned {
		if seen[u.Stop] {
			t.Fatalf("stop %d routed and unassigned", u.Stop)
		}
		seen[u.Stop] = true
		if u.Reason(Fnum) == "" {
			t.Fatal("no reason")
		}
	}
	if len(seen) != len(p.Stops) {
		t.Fatalf("%d of %d stops accounted for", len(seen), len(p.Stops))
	}
	if (len(r.Unassigned) > 0) != (r.Status == Infeasible) {
		t.Fatalf("status %v with %d unassigned", r.Status, len(r.Unassigned))
	}
	if r.Status != Infeasible && r.Bound > r.Cost+1e-9 {
		t.Fatalf("bound %v > cost %v", r.Bound, r.Cost)
	}
}

func randVRP(rng *rand.Rand, stops, vehicles int, windows bool) *VRP {
	n := stops + 1
	m := pointsMatrix(rng, n)
	p := &VRP{Dist: m, Time: m, Seed: rng.Int63()}
	for i := 1; i < n; i++ {
		st := Stop{Node: i, Demand: float64(1 + rng.Intn(9)), Close: Inf}
		if windows {
			st.Open = rng.Float64() * 3000
			st.Close = st.Open + 300 + rng.Float64()*1500
			st.Service = 10
		}
		p.Stops = append(p.Stops, st)
	}
	for v := 0; v < vehicles; v++ {
		p.Vehicles = append(p.Vehicles, Vehicle{Capacity: float64(15 + rng.Intn(30)), MaxDist: Inf, MaxTime: Inf, Close: Inf})
	}
	return p
}

func TestVRPRandomValid(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	ctx := context.Background()
	for c := 0; c < 60; c++ {
		p := randVRP(rng, 3+rng.Intn(40), 1+rng.Intn(6), rng.Intn(2) == 0)
		p.Objective = Objective(rng.Intn(3))
		if rng.Intn(3) == 0 {
			p.Vehicles[0].MaxDist = 3000
			p.Vehicles[0].Close = 4000
		}
		r, err := SolveVRP(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, p, r)
	}
}

func TestVRPUnassignedReasons(t *testing.T) {
	m, _ := NewMatrix(3)
	for i := range m.C {
		m.C[i] = 10
	}
	for i := 0; i < 3; i++ {
		m.Set(i, i, 0)
	}
	p := &VRP{Dist: m, Time: m,
		Stops:    []Stop{{Node: 1, Demand: 50, Close: Inf}, {Node: 2, Demand: 1, Close: 5}},
		Vehicles: []Vehicle{{Capacity: 10, MaxDist: Inf, MaxTime: Inf, Close: Inf}},
		Integer:  true}
	r, err := SolveVRP(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, p, r)
	if r.Status != Infeasible || len(r.Unassigned) != 2 {
		t.Fatalf("got %+v", r)
	}
	if r.Unassigned[0].Reason(Fnum) != "demand 50 is over the largest vehicle capacity 10" {
		t.Errorf("reason 0: %s", r.Unassigned[0].Reason(Fnum))
	}
	if r.Unassigned[1].Reason(Fnum) != "window closes at 5; the earliest arrival is 10" {
		t.Errorf("reason 1: %s", r.Unassigned[1].Reason(Fnum))
	}
}

func TestVRPFleetTooSmall(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	p := randVRP(rng, 30, 1, false)
	p.Vehicles[0].Capacity = 20
	r, _ := SolveVRP(context.Background(), p)
	validate(t, p, r)
	if r.Status != Infeasible {
		t.Fatalf("status %v", r.Status)
	}
	for _, u := range r.Unassigned {
		if u.Reason(Fnum) == "" {
			t.Fatal("empty reason")
		}
	}
}

func TestVRPTimeLimit(t *testing.T) {
	p := randVRP(rand.New(rand.NewSource(4)), 300, 20, true)
	p.Iterations = 1 << 30
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, err := SolveVRP(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Errorf("took %v", el)
	}
	if !errors.Is(r.Stopped, context.DeadlineExceeded) {
		t.Errorf("stopped = %v", r.Stopped)
	}
	validate(t, p, r)
}

// TestTimings logs the reference timings (go test -run Timings -v).
func TestTimings(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	rng := rand.New(rand.NewSource(8))
	for _, n := range []int{13, 15, 16, 18} {
		p := TSP{M: pointsMatrix(rng, n), RoundTrip: true}
		start := time.Now()
		r, _ := SolveTSP(ctx, TSP{M: p.M, RoundTrip: true, ExactMax: n})
		t.Logf("TSP %d exact: %v (%s)", n, time.Since(start), r.Method)
	}
	p := TSP{M: pointsMatrix(rng, 200), RoundTrip: true}
	start := time.Now()
	r, _ := SolveTSP(ctx, p)
	t.Logf("TSP 200 heuristic: %v gap %.2f%% iters %d", time.Since(start), 100*Gap(r.Cost, r.Bound), r.Iterations)
	for _, windows := range []bool{false, true} {
		vp := randVRP(rng, 100, 10, windows)
		for i := range vp.Vehicles {
			vp.Vehicles[i].Capacity = 60 // demand ~500 of 600
		}
		start = time.Now()
		vr, _ := SolveVRP(ctx, vp)
		validate(t, vp, vr)
		t.Logf("VRP 100 stops / 10 vehicles (windows %v): %v routes %d unassigned %d cost %.0f bound %.0f gap %.1f%% iters %d", windows, time.Since(start), len(vr.Routes), len(vr.Unassigned), vr.Cost, vr.Bound, 100*Gap(vr.Cost, vr.Bound), vr.Iterations)
	}
}
