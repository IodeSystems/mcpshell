package graphalg

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"testing"
)

// testdata/refs.json comes from testdata/gen_refs.py: networkx 3.7
// (components, SCC, DAG test, cycle basis, maximum_flow_value,
// min_cost_flow_cost, hopcroft_karp_matching, minimum/maximum_spanning_tree,
// bellman_ford, dag_longest_path_length) and scipy linear_sum_assignment.
type refs struct {
	Components []struct {
		N               int
		Arcs            [][2]int
		Weak, Strong    [][]int
		Dag             bool
		UndirectedCycle bool
	}
	Flow []struct {
		N        int
		Arcs     [][2]int
		Cap      []int64
		S, T     int
		Value    int64
	}
	Mincost []struct {
		N        int
		Arcs     [][2]int
		Cap      []int64
		Cost     []int64
		Supply   []int64
		Feasible bool
		Total    *int64
	}
	Matching []struct {
		Nl, Nr  int
		Edges   [][2]int
		Cost    []int64
		Size    int
		MinCost int64
	}
	Assignment []struct {
		Cost    [][]int64
		MinCost int64
		Size    int
	}
	Mst []struct {
		N         int
		Edges     [][2]int
		W         []float64
		Weight    float64
		MaxWeight float64
		Trees     int
	}
	Shortest []struct {
		N        int
		Arcs     [][2]int
		W        []float64
		NegCycle bool
		Dist     map[string]float64
	}
	Dag []struct {
		N            int
		Arcs         [][2]int
		Lag          []float64
		Dur          []float64
		Makespan     float64
		LongestFrom0 map[string]float64
	}
}

func loadRefs(t *testing.T) refs {
	t.Helper()
	b, err := os.ReadFile("testdata/refs.json")
	if err != nil {
		t.Fatal(err)
	}
	var r refs
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func split(arcs [][2]int) (from, to []int) {
	for _, a := range arcs {
		from, to = append(from, a[0]), append(to, a[1])
	}
	return
}

func groups(label []int, count int) [][]int {
	g := make([][]int, count)
	for v, l := range label {
		g[l] = append(g[l], v)
	}
	sort.Slice(g, func(a, b int) bool { return slices.Compare(g[a], g[b]) < 0 })
	return g
}

func wadj(n int, arcs [][2]int, w []float64) [][]Arc {
	adj := make([][]Arc, n)
	for k, a := range arcs {
		adj[a[0]] = append(adj[a[0]], Arc{a[1], w[k]})
	}
	return adj
}

// checkCycle verifies that cyc is a directed cycle of adj.
func checkCycle(t *testing.T, adj [][]int, cyc []int, what string) {
	t.Helper()
	if len(cyc) == 0 {
		t.Fatalf("%s: empty cycle", what)
	}
	for i, u := range cyc {
		v := cyc[(i+1)%len(cyc)]
		if !slices.Contains(adj[u], v) {
			t.Fatalf("%s: cycle %v has no arc %d -> %d", what, cyc, u, v)
		}
	}
}

func TestComponentsMatchNetworkx(t *testing.T) {
	for i, c := range loadRefs(t).Components {
		from, to := split(c.Arcs)
		adj := Adj(c.N, from, to)
		l, k := Components(c.N, from, to)
		if g := groups(l, k); !slices.EqualFunc(g, c.Weak, slices.Equal) {
			t.Errorf("case %d weak: got %v want %v", i, g, c.Weak)
		}
		l, k = SCC(adj)
		if g := groups(l, k); !slices.EqualFunc(g, c.Strong, slices.Equal) {
			t.Errorf("case %d strong: got %v want %v", i, g, c.Strong)
		}
		order, err := TopoSort(adj)
		if c.Dag {
			if err != nil {
				t.Fatalf("case %d: DAG reported %v", i, err)
			}
			pos := make([]int, c.N)
			for p, v := range order {
				pos[v] = p
			}
			for _, a := range c.Arcs {
				if pos[a[0]] >= pos[a[1]] {
					t.Fatalf("case %d: arc %v goes backward in %v", i, a, order)
				}
			}
			if FindCycle(adj) != nil {
				t.Errorf("case %d: cycle in a DAG", i)
			}
		} else {
			var ce *CycleError
			if !errors.As(err, &ce) {
				t.Fatalf("case %d: cyclic graph sorted: %v", i, err)
			}
			checkCycle(t, adj, ce.Nodes, "topo")
		}
		// Undirected: each unordered pair once.
		seen := map[[2]int]bool{}
		var uf, ut []int
		uadj := make([][]int, c.N)
		for _, a := range c.Arcs {
			p := [2]int{min(a[0], a[1]), max(a[0], a[1])}
			if !seen[p] {
				seen[p] = true
				uf, ut = append(uf, a[0]), append(ut, a[1])
				uadj[a[0]] = append(uadj[a[0]], a[1])
				uadj[a[1]] = append(uadj[a[1]], a[0])
			}
		}
		cyc := FindCycleUndirected(c.N, uf, ut)
		if (cyc != nil) != c.UndirectedCycle {
			t.Errorf("case %d: undirected cycle %v, want found=%v", i, cyc, c.UndirectedCycle)
		}
		if cyc != nil {
			if len(cyc) < 3 {
				t.Errorf("case %d: undirected cycle %v shorter than 3 on a simple graph", i, cyc)
			}
			checkCycle(t, uadj, cyc, "undirected")
		}
	}
}

func TestMaxFlowMatchesNetworkx(t *testing.T) {
	ctx := context.Background()
	for i, c := range loadRefs(t).Flow {
		arcs := make([]FlowArc, len(c.Arcs))
		for k, a := range c.Arcs {
			arcs[k] = FlowArc{From: a[0], To: a[1], Cap: c.Cap[k]}
		}
		r, err := MaxFlow(ctx, c.N, arcs, c.S, c.T)
		if err != nil {
			t.Fatal(err)
		}
		if r.Value != c.Value {
			t.Errorf("case %d: max flow %d, networkx %d", i, r.Value, c.Value)
		}
		checkFlow(t, i, c.N, arcs, r.Flow, func(v int) int64 {
			switch v {
			case c.S:
				return r.Value
			case c.T:
				return -r.Value
			}
			return 0
		})
		// The cut's capacity equals the flow value (max-flow min-cut).
		var cut int64
		for _, a := range arcs {
			if r.Source[a.From] && !r.Source[a.To] {
				cut += a.Cap
			}
		}
		if cut != r.Value || !r.Source[c.S] || r.Source[c.T] {
			t.Errorf("case %d: cut %d, flow %d", i, cut, r.Value)
		}
	}
}

// checkFlow tests capacity and conservation: out - in = net(v).
func checkFlow(t *testing.T, i, n int, arcs []FlowArc, flow []int64, net func(int) int64) {
	t.Helper()
	bal := make([]int64, n)
	for k, a := range arcs {
		if flow[k] < 0 || a.Cap >= 0 && flow[k] > a.Cap {
			t.Fatalf("case %d: arc %d flow %d cap %d", i, k, flow[k], a.Cap)
		}
		bal[a.From] += flow[k]
		bal[a.To] -= flow[k]
	}
	for v := range n {
		if bal[v] != net(v) {
			t.Fatalf("case %d: node %d balance %d, want %d", i, v, bal[v], net(v))
		}
	}
}

func TestMinCostFlowMatchesNetworkx(t *testing.T) {
	ctx := context.Background()
	for i, c := range loadRefs(t).Mincost {
		arcs := make([]FlowArc, len(c.Arcs))
		for k, a := range c.Arcs {
			arcs[k] = FlowArc{From: a[0], To: a[1], Cap: c.Cap[k], Cost: c.Cost[k]}
		}
		r, err := MinCostFlow(ctx, c.N, arcs, c.Supply)
		if err != nil {
			t.Fatal(err)
		}
		if !c.Feasible {
			if r.Status != Infeasible {
				t.Errorf("case %d: %s, networkx infeasible", i, r.Status)
			}
			continue
		}
		if r.Status != Optimal || r.Cost.Int64() != *c.Total {
			t.Errorf("case %d: %s cost %v, networkx %d", i, r.Status, r.Cost, *c.Total)
			continue
		}
		checkFlow(t, i, c.N, arcs, r.Flow, func(v int) int64 { return c.Supply[v] })
	}
}

func TestMatchingMatchesNetworkx(t *testing.T) {
	ctx := context.Background()
	r := loadRefs(t)
	for i, c := range r.Matching {
		edges := make([]MatchEdge, len(c.Edges))
		for k, e := range c.Edges {
			edges[k] = MatchEdge{L: e[0], R: e[1], Cost: c.Cost[k]}
		}
		m, err := MaxMatching(ctx, c.Nl, c.Nr, edges)
		if err != nil {
			t.Fatal(err)
		}
		checkMatching(t, i, edges, m.Edges)
		if len(m.Edges) != c.Size {
			t.Errorf("case %d: matching %d, networkx %d", i, len(m.Edges), c.Size)
		}
		a, err := Assignment(ctx, c.Nl, c.Nr, edges)
		if err != nil {
			t.Fatal(err)
		}
		checkMatching(t, i, edges, a.Edges)
		if len(a.Edges) != c.Size || a.Cost.Int64() != c.MinCost {
			t.Errorf("case %d: assignment %d cost %v, networkx %d cost %d", i, len(a.Edges), a.Cost, c.Size, c.MinCost)
		}
	}
	for i, c := range r.Assignment {
		var edges []MatchEdge
		for l, row := range c.Cost {
			for rr, x := range row {
				edges = append(edges, MatchEdge{L: l, R: rr, Cost: x})
			}
		}
		a, err := Assignment(ctx, len(c.Cost), len(c.Cost[0]), edges)
		if err != nil {
			t.Fatal(err)
		}
		if len(a.Edges) != c.Size || a.Cost.Int64() != c.MinCost {
			t.Errorf("dense case %d: %d cost %v, scipy %d cost %d", i, len(a.Edges), a.Cost, c.Size, c.MinCost)
		}
	}
}

func checkMatching(t *testing.T, i int, edges []MatchEdge, chosen []int) {
	t.Helper()
	l, r := map[int]bool{}, map[int]bool{}
	for _, e := range chosen {
		if l[edges[e].L] || r[edges[e].R] {
			t.Fatalf("case %d: node used twice in %v", i, chosen)
		}
		l[edges[e].L], r[edges[e].R] = true, true
	}
}

func TestMSTMatchesNetworkx(t *testing.T) {
	for i, c := range loadRefs(t).Mst {
		from, to := split(c.Edges)
		for _, maximize := range []bool{false, true} {
			chosen, trees := MST(c.N, from, to, c.W, maximize)
			sum := 0.0
			for _, e := range chosen {
				sum += c.W[e]
			}
			want := c.Weight
			if maximize {
				want = c.MaxWeight
			}
			if sum != want || trees != c.Trees || len(chosen) != c.N-c.Trees {
				t.Errorf("case %d max=%v: weight %v trees %d, networkx %v trees %d", i, maximize, sum, trees, want, c.Trees)
			}
		}
	}
}

func TestBellmanFordMatchesNetworkx(t *testing.T) {
	ctx := context.Background()
	for i, c := range loadRefs(t).Shortest {
		adj := wadj(c.N, c.Arcs, c.W)
		dist, prev, err := BellmanFord(ctx, adj, 0)
		if c.NegCycle {
			var ne *NegativeCycleError
			if !errors.As(err, &ne) {
				t.Fatalf("case %d: want negative cycle, got %v", i, err)
			}
			sum := 0.0
			for k, u := range ne.Nodes {
				v := ne.Nodes[(k+1)%len(ne.Nodes)]
				best := math.Inf(1)
				for _, a := range adj[u] {
					if a.To == v {
						best = math.Min(best, a.W)
					}
				}
				if math.IsInf(best, 1) {
					t.Fatalf("case %d: cycle %v has no arc %d -> %d", i, ne.Nodes, u, v)
				}
				sum += best
			}
			if sum >= 0 {
				t.Errorf("case %d: cycle %v weight %v", i, ne.Nodes, sum)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for v := range c.N {
			want, ok := c.Dist[strconv.Itoa(v)]
			if !ok {
				want = math.Inf(1)
			}
			if dist[v] != want {
				t.Errorf("case %d node %d: %v, networkx %v", i, v, dist[v], want)
			}
			if ok && v != 0 && prev[v] < 0 {
				t.Errorf("case %d node %d: no predecessor", i, v)
			}
		}
	}
}

func TestCPMAndLongestMatchNetworkx(t *testing.T) {
	for i, c := range loadRefs(t).Dag {
		adj := wadj(c.N, c.Arcs, c.Lag)
		s, err := CPM(adj, c.Dur, 0)
		if err != nil {
			t.Fatal(err)
		}
		if s.Makespan != c.Makespan {
			t.Errorf("case %d: makespan %v, networkx %v", i, s.Makespan, c.Makespan)
		}
		for v := range c.N {
			if s.Slack[v] < 0 || s.EF[v] > s.Makespan || s.LS[v] < s.ES[v] {
				t.Errorf("case %d task %d: ES %v LS %v EF %v", i, v, s.ES[v], s.LS[v], s.EF[v])
			}
			for _, a := range adj[v] {
				if s.ES[a.To] < s.EF[v]+a.W || s.LS[a.To] < s.LF[v]+a.W {
					t.Errorf("case %d: arc %d -> %d violated", i, v, a.To)
				}
			}
		}
		if c.N > 0 {
			p := s.Path
			if len(p) == 0 || s.ES[p[0]] != 0 || s.EF[p[len(p)-1]] != s.Makespan {
				t.Errorf("case %d: critical path %v does not span 0..%v", i, p, s.Makespan)
			}
		}
		dist, _, err := Longest(adj, 0)
		if err != nil {
			t.Fatal(err)
		}
		for v := range c.N {
			want, ok := c.LongestFrom0[strconv.Itoa(v)]
			if !ok {
				want = math.Inf(-1)
			}
			if dist[v] != want {
				t.Errorf("case %d node %d: longest %v, want %v", i, v, dist[v], want)
			}
		}
	}
}

// A hand-checked project network (activity on node, durations in days):
//
//	A(3) -> B(2) -> D(4) -> F(1)
//	A    -> C(5) -> E(1) -> F
//	C    -> D
//
// Forward: A 0-3, B 3-5, C 3-8, D 8-12, E 8-9, F 12-13. Makespan 13.
// Backward: F 12-13, D 8-12, E 11-12, B 6-8, C 3-8, A 0-3.
// Slack: B 3, E 3, others 0. Critical path A C D F.
func TestCPMHandChecked(t *testing.T) {
	const A, B, C, D, E, F = 0, 1, 2, 3, 4, 5
	arcs := [][2]int{{A, B}, {A, C}, {B, D}, {C, D}, {C, E}, {D, F}, {E, F}}
	s, err := CPM(wadj(6, arcs, make([]float64, len(arcs))), []float64{3, 2, 5, 4, 1, 1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s.Makespan != 13 {
		t.Fatalf("makespan %v", s.Makespan)
	}
	wantES := []float64{0, 3, 3, 8, 8, 12}
	wantLS := []float64{0, 6, 3, 8, 11, 12}
	if !slices.Equal(s.ES, wantES) || !slices.Equal(s.LS, wantLS) {
		t.Errorf("ES %v LS %v", s.ES, s.LS)
	}
	if !slices.Equal(s.Path, []int{A, C, D, F}) {
		t.Errorf("path %v", s.Path)
	}
	if !slices.Equal(s.Critical, []bool{true, false, true, true, false, true}) {
		t.Errorf("critical %v", s.Critical)
	}
	// A lag of 2 between C and E makes E critical-adjacent: E 10-11, slack 1.
	arcs2 := append(slices.Clone(arcs[:4]), [2]int{C, E}, [2]int{D, F}, [2]int{E, F})
	lag := []float64{0, 0, 0, 0, 2, 0, 0}
	s, _ = CPM(wadj(6, arcs2, lag), []float64{3, 2, 5, 4, 1, 1}, 0)
	if s.ES[E] != 10 || s.Slack[E] != 1 {
		t.Errorf("lag: E ES %v slack %v", s.ES[E], s.Slack[E])
	}
	// A cycle is an error naming it.
	_, err = CPM(wadj(3, [][2]int{{0, 1}, {1, 2}, {2, 1}}, []float64{0, 0, 0}), []float64{1, 1, 1}, 0)
	var ce *CycleError
	if !errors.As(err, &ce) || !slices.Equal(ce.Nodes, []int{1, 2}) {
		t.Errorf("cycle: %v", err)
	}
}

func TestReach(t *testing.T) {
	adj := [][]int{{1, 2}, {3}, {3}, {}, {0}}
	got := Reach(adj, []int{0}, false, -1)
	want := []Visit{{0, 0, -1}, {1, 1, 0}, {2, 1, 0}, {3, 2, 1}}
	if !slices.Equal(got, want) {
		t.Errorf("bfs %v", got)
	}
	got = Reach(adj, []int{0}, true, -1)
	want = []Visit{{0, 0, -1}, {1, 1, 0}, {3, 2, 1}, {2, 1, 0}}
	if !slices.Equal(got, want) {
		t.Errorf("dfs %v", got)
	}
	if got = Reach(adj, []int{0}, false, 1); len(got) != 3 {
		t.Errorf("maxDepth 1: %v", got)
	}
}

func TestFlowUnlimitedAndNegative(t *testing.T) {
	ctx := context.Background()
	// s -> a unlimited, a -> t cap 5: max flow 5.
	r, _ := MaxFlow(ctx, 3, []FlowArc{{0, 1, -1, 0}, {1, 2, 5, 0}}, 0, 2)
	if r.Status != Optimal || r.Value != 5 {
		t.Errorf("unlimited arc: %v %d", r.Status, r.Value)
	}
	r, _ = MaxFlow(ctx, 2, []FlowArc{{0, 1, -1, 0}}, 0, 1)
	if r.Status != Unbounded {
		t.Errorf("unlimited path: %v", r.Status)
	}
	// Negative cycle of unlimited arcs: unbounded.
	r, _ = MinCostFlow(ctx, 2, []FlowArc{{0, 1, -1, -1}, {1, 0, -1, 0}}, []int64{0, 0})
	if r.Status != Unbounded {
		t.Errorf("negative unlimited cycle: %v", r.Status)
	}
	// Negative cycle with a finite arc: bounded; flow 3 around it, cost -3*4+3*1.
	r, _ = MinCostFlow(ctx, 2, []FlowArc{{0, 1, -1, -4}, {1, 0, 3, 1}}, []int64{0, 0})
	if r.Status != Optimal || r.Cost.Int64() != -9 || r.Flow[0] != 3 {
		t.Errorf("bounded negative cycle: %v %v %v", r.Status, r.Cost, r.Flow)
	}
	if _, err := MinCostFlow(ctx, 2, nil, []int64{1, 0}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unbalanced supplies: %v", err)
	}
}
