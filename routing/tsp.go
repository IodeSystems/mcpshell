package routing

import (
	"context"
	"math"
	"math/rand"
	"slices"
	"sort"
)

// DefaultIterations is the default kick count of the TSP search for n
// nodes. Measured (one core, random plane points, local search + bound
// included): n=200 ~0.25 s, gap ~1.3%.
func DefaultIterations(n int) int {
	switch {
	case n < 8:
		return 0
	case n <= 50:
		return 300
	case n <= 200:
		return 100
	case n <= 1000:
		return 30
	}
	return 5
}

// SolveTSP solves p: Held-Karp when the node count is at most ExactMax,
// otherwise nearest neighbour, 2-opt and Or-opt with double-bridge kicks,
// and a Held-Karp (1-tree) lower bound.
func SolveTSP(ctx context.Context, p TSP) (*TSPResult, error) {
	m := p.M
	if m == nil || m.N == 0 {
		return nil, invalid("no nodes")
	}
	n := m.N
	if p.Start < 0 || p.Start >= n {
		return nil, invalid("start %d is not a node", p.Start)
	}
	if p.RoundTrip || p.End == p.Start {
		p.RoundTrip, p.End = true, -1
	}
	if p.End >= n {
		return nil, invalid("end %d is not a node", p.End)
	}
	for _, c := range m.C {
		if c < 0 || math.IsNaN(c) {
			return nil, invalid("costs must be >= 0")
		}
		if math.IsInf(c, 1) {
			return nil, invalid("the matrix has missing arcs; fill them first (Closure)")
		}
	}
	exactMax := p.ExactMax
	if exactMax == 0 {
		exactMax = DefaultExactMax
	}
	exactMax = min(exactMax, HeldKarpMax)
	if n <= exactMax {
		if r := heldKarp(ctx, p); r != nil {
			return r, nil
		}
	}
	return tspHeuristic(ctx, p), nil
}

// heldKarp is the O(2^n n^2) dynamic program. It returns nil when ctx stops it.
func heldKarp(ctx context.Context, p TSP) *TSPResult {
	m, n, s := p.M, p.M.N, p.Start
	if n == 1 {
		order := []int{s}
		if p.RoundTrip {
			order = append(order, s)
		}
		return &TSPResult{Status: Optimal, Order: order, Method: "held-karp"}
	}
	// others[k] is the k-th node other than s.
	others := make([]int, 0, n-1)
	for v := 0; v < n; v++ {
		if v != s {
			others = append(others, v)
		}
	}
	k := n - 1
	full := 1<<k - 1
	dp := make([]float64, (full+1)*k)
	par := make([]int8, (full+1)*k)
	for i := range dp {
		dp[i] = Inf
	}
	for j := 0; j < k; j++ {
		dp[(1<<j)*k+j] = m.At(s, others[j])
		par[(1<<j)*k+j] = -1
	}
	for set := 1; set <= full; set++ {
		if set&1023 == 0 && ctx.Err() != nil {
			return nil
		}
		for j := 0; j < k; j++ {
			if set&(1<<j) == 0 {
				continue
			}
			cur := dp[set*k+j]
			if math.IsInf(cur, 1) {
				continue
			}
			vj := others[j]
			for t := 0; t < k; t++ {
				if set&(1<<t) != 0 {
					continue
				}
				ns := set | 1<<t
				if c := cur + m.At(vj, others[t]); c < dp[ns*k+t] {
					dp[ns*k+t], par[ns*k+t] = c, int8(j)
				}
			}
		}
	}
	best, bj := Inf, -1
	for j := 0; j < k; j++ {
		c := dp[full*k+j]
		switch {
		case p.RoundTrip:
			c += m.At(others[j], s)
		case p.End >= 0 && others[j] != p.End:
			continue
		}
		if c < best {
			best, bj = c, j
		}
	}
	rev := []int{}
	for set, j := full, bj; j >= 0; {
		rev = append(rev, others[j])
		pj := int(par[set*k+j])
		set &^= 1 << j
		j = pj
	}
	order := []int{s}
	for i := len(rev) - 1; i >= 0; i-- {
		order = append(order, rev[i])
	}
	if p.RoundTrip {
		order = append(order, s)
	}
	return &TSPResult{Status: Optimal, Order: order, Cost: best, Bound: best, Method: "held-karp"}
}

// seq is a node sequence p[0..len-1] with fixed ends; the interior
// p[1..len-2] moves. virt is the free-end sentinel: an arc into it costs 0.
// Moves only make arcs to a node's nearest neighbours (nb).
type seq struct {
	m    *Matrix
	p    []int
	pos  []int // pos[node] = index in p
	nb   [][]int
	virt int
	sym  bool
	eps  float64
	f, b []float64 // prefix sums forward and backward along p (asymmetric only)
}

func (s *seq) c(a, b int) float64 {
	if b == s.virt || a == s.virt {
		return 0
	}
	return s.m.C[a*s.m.N+b]
}

func (s *seq) cost() float64 {
	t := 0.0
	for i := 0; i+1 < len(s.p); i++ {
		t += s.c(s.p[i], s.p[i+1])
	}
	return t
}

// sync rebuilds pos and the prefix sums after p changed.
func (s *seq) sync() {
	for i, v := range s.p {
		s.pos[v] = i
	}
	if s.sym {
		return
	}
	n := len(s.p)
	if len(s.f) != n {
		s.f, s.b = make([]float64, n), make([]float64, n)
	}
	for i := 1; i < n; i++ {
		s.f[i] = s.f[i-1] + s.c(s.p[i-1], s.p[i])
		s.b[i] = s.b[i-1] + s.c(s.p[i], s.p[i-1])
	}
}

// revDelta is the cost change of reversing p[l..r] (1 <= l < r <= last-1).
func (s *seq) revDelta(l, r int) float64 {
	p := s.p
	d := s.c(p[l-1], p[r]) + s.c(p[l], p[r+1]) - s.c(p[l-1], p[l]) - s.c(p[r], p[r+1])
	if !s.sym {
		d += (s.b[r] - s.b[l]) - (s.f[r] - s.f[l])
	}
	return d
}

// twoOpt reverses interior segments while that shortens the sequence. For
// the arc a=p[i-1] -> p[i] it tries each neighbour c of a: reversing
// p[i..pos c] makes the arc a -> c; when c is before a, reversing
// p[pos c+1..i-1] makes c -> a.
func (s *seq) twoOpt(ctx context.Context) bool {
	last := len(s.p) - 1
	any := false
	for improved := true; improved; {
		improved = false
		for i := 1; i < last; i++ {
			if i&63 == 0 && ctx.Err() != nil {
				return any
			}
			for _, c := range s.nb[s.p[i-1]] {
				j := s.pos[c]
				l, r := i, j
				if j < i-1 {
					l, r = j+1, i-1
				}
				if r <= l || l < 1 || r > last-1 {
					continue
				}
				if s.revDelta(l, r) < -s.eps {
					slices.Reverse(s.p[l : r+1])
					s.sync()
					improved, any = true, true
					break
				}
			}
		}
	}
	return any
}

// orOpt moves interior segments of 1-3 nodes, forward or reversed, next to
// a neighbour of an end of the segment, while that shortens the sequence.
func (s *seq) orOpt(ctx context.Context) bool {
	any := false
	last := len(s.p) - 1
	tmp := make([]int, 0, len(s.p))
	seg := make([]int, 0, 3)
	for improved := true; improved; {
		improved = false
		for L := 1; L <= 3; L++ {
			for i := 1; i+L-1 < last; i++ {
				if i&63 == 0 && ctx.Err() != nil {
					return any
				}
				p := s.p
				j := i + L - 1 // segment p[i..j]
				a, x, y, b := p[i-1], p[i], p[j], p[j+1]
				inner, innerRev := 0.0, 0.0
				for t := i; t < j; t++ {
					inner += s.c(p[t], p[t+1])
					innerRev += s.c(p[t+1], p[t])
				}
				remove := s.c(a, x) + s.c(y, b) - s.c(a, b)
				bestK, bestD, bestRev := -1, -s.eps, false
				try := func(k int) { // insert between p[k] and p[k+1]
					if k < 0 || k >= last || (k >= i-1 && k <= j) {
						return
					}
					u, v := p[k], p[k+1]
					base := s.c(u, v)
					if d := s.c(u, x) + s.c(y, v) - base - remove; d < bestD {
						bestK, bestD, bestRev = k, d, false
					}
					if d := s.c(u, y) + s.c(x, v) - base - remove + innerRev - inner; d < bestD {
						bestK, bestD, bestRev = k, d, true
					}
				}
				for _, end := range [2]int{x, y} {
					for _, w := range s.nb[end] {
						try(s.pos[w])
						try(s.pos[w] - 1)
					}
				}
				if bestK < 0 {
					continue
				}
				seg = append(seg[:0], p[i:j+1]...)
				if bestRev {
					slices.Reverse(seg)
				}
				tmp = tmp[:0]
				for t := 0; t < len(p); t++ {
					if t >= i && t <= j {
						continue
					}
					tmp = append(tmp, p[t])
					if t == bestK {
						tmp = append(tmp, seg...)
					}
				}
				copy(p, tmp)
				s.sync()
				improved, any = true, true
			}
		}
	}
	return any
}

func (s *seq) localSearch(ctx context.Context) {
	s.sync()
	for {
		s.twoOpt(ctx)
		if !s.orOpt(ctx) || ctx.Err() != nil {
			return
		}
	}
}

// nearest lists the k nearest other nodes of each node by c(i,j)+c(j,i).
func nearest(ctx context.Context, m *Matrix, k int) [][]int {
	n := m.N
	k = min(k, n-1)
	nb := make([][]int, n)
	d := make([]float64, 0, k+1)
	for i := 0; i < n; i++ {
		if i&255 == 0 && ctx.Err() != nil {
			// Out of time: fall back to the first k nodes (moves stay valid).
			for ; i < n; i++ {
				for j := 0; j < n && len(nb[i]) < k; j++ {
					if j != i {
						nb[i] = append(nb[i], j)
					}
				}
			}
			break
		}
		row := make([]int, 0, k+1)
		d = d[:0]
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			w := m.C[i*n+j] + m.C[j*n+i]
			if len(row) == k && w >= d[k-1] {
				continue
			}
			at := sort.SearchFloat64s(d, w)
			for at < len(d) && d[at] == w {
				at++
			}
			row = slices.Insert(row, at, j)
			d = slices.Insert(d, at, w)
			if len(row) > k {
				row, d = row[:k], d[:k]
			}
		}
		nb[i] = row
	}
	return nb
}

// doubleBridge cuts the interior in 4 parts A B C D and reorders to A C B D.
func doubleBridge(p []int, rng *rand.Rand) {
	n := len(p) - 2 // interior p[1..n]
	if n < 8 {
		return
	}
	cuts := rng.Perm(n - 1)[:3]
	a, b, c := cuts[0]+1, cuts[1]+1, cuts[2]+1
	if a > b {
		a, b = b, a
	}
	if b > c {
		b, c = c, b
	}
	if a > b {
		a, b = b, a
	}
	// interior indexes 1..n: [1,a] [a+1,b] [b+1,c] [c+1,n]
	out := make([]int, 0, len(p))
	out = append(out, p[:a+1]...)
	out = append(out, p[b+1:c+1]...)
	out = append(out, p[a+1:b+1]...)
	out = append(out, p[c+1:]...)
	copy(p, out)
}

func tspHeuristic(ctx context.Context, p TSP) *TSPResult {
	m, n := p.M, p.M.N
	virt := -1
	// Build the fixed-ends sequence: start, interior..., terminal.
	nodes := make([]int, 0, n+1)
	nodes = append(nodes, p.Start)
	inInterior := func(v int) bool { return v != p.Start && (p.RoundTrip || v != p.End) }
	// Nearest neighbour from start over the interior nodes.
	used := make([]bool, n)
	used[p.Start] = true
	if !p.RoundTrip && p.End >= 0 {
		used[p.End] = true
	}
	cur := p.Start
	for {
		best, bv := Inf, -1
		for v := 0; v < n; v++ {
			if !used[v] && inInterior(v) && m.At(cur, v) < best {
				best, bv = m.At(cur, v), v
			}
		}
		if bv < 0 {
			break
		}
		used[bv] = true
		nodes = append(nodes, bv)
		cur = bv
	}
	switch {
	case p.RoundTrip:
		nodes = append(nodes, p.Start)
	case p.End >= 0:
		nodes = append(nodes, p.End)
	default:
		virt = n
		nodes = append(nodes, virt)
	}
	s := &seq{m: m, p: nodes, virt: virt, sym: m.Symmetric(), pos: make([]int, n+1), nb: nearest(ctx, m, 10)}
	maxC := 0.0
	for _, c := range m.C {
		maxC = math.Max(maxC, c)
	}
	s.eps = 1e-9 * math.Max(maxC, 1e-300)
	if p.Integer {
		s.eps = 0.5
	}
	s.localSearch(ctx)
	best := append([]int(nil), s.p...)
	bestCost := s.cost()

	iters := p.Iterations
	if iters == 0 {
		iters = DefaultIterations(n)
	}
	rng := rand.New(rand.NewSource(p.Seed))
	done := 0
	for ; done < iters && ctx.Err() == nil; done++ {
		copy(s.p, best)
		doubleBridge(s.p, rng)
		s.localSearch(ctx)
		if c := s.cost(); c < bestCost-s.eps {
			bestCost = c
			copy(best, s.p)
		}
	}
	order := best
	if virt >= 0 {
		order = order[:len(order)-1]
	}
	res := &TSPResult{Status: Feasible, Order: order, Cost: bestCost, Method: "heuristic", Iterations: done}
	res.Bound = tspBound(ctx, p, bestCost)
	if p.Integer && res.Bound >= bestCost {
		res.Status, res.Bound = Optimal, bestCost
	}
	res.Stopped = ctx.Err()
	return res
}

// tspBound is the Held-Karp 1-tree bound with subgradient steps on the
// symmetric costs w(i,j) = min(c(i,j), c(j,i)) of a modified graph whose
// tours cost at most the optimum: a fixed-end path gets w(start, end) = 0; a
// free-end path gets a dummy node joined to every node at cost 0. Any
// penalty vector gives a valid bound, so a stopped search still returns one.
func tspBound(ctx context.Context, p TSP, ub float64) float64 {
	m := p.M
	n := m.N
	N := n
	if !p.RoundTrip && p.End < 0 {
		N = n + 1
	}
	if N < 3 {
		return ub // a 1- or 2-node tour is the only tour
	}
	W := make([]float64, N*N)
	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			if i < n && j < n {
				W[i*N+j] = math.Min(m.C[i*n+j], m.C[j*n+i])
			}
		}
	}
	if !p.RoundTrip && p.End >= 0 {
		W[p.Start*N+p.End], W[p.End*N+p.Start] = 0, 0
	}
	w := func(i, j int) float64 { return W[i*N+j] }
	pi := make([]float64, N)
	deg := make([]int, N)
	key := make([]float64, N)
	par := make([]int, N)
	in := make([]bool, N)
	oneTree := func() float64 {
		// Prim over nodes 1..N-1, then node 0 joins with its two cheapest arcs.
		for i := range key {
			key[i], par[i], in[i], deg[i] = Inf, -1, false, 0
		}
		total := 0.0
		key[1] = 0
		for it := 1; it < N; it++ {
			u := -1
			for v := 1; v < N; v++ {
				if !in[v] && (u < 0 || key[v] < key[u]) {
					u = v
				}
			}
			in[u] = true
			if par[u] >= 0 {
				total += key[u]
				deg[u]++
				deg[par[u]]++
			}
			for v := 1; v < N; v++ {
				if !in[v] {
					if c := w(u, v) + pi[u] + pi[v]; c < key[v] {
						key[v], par[v] = c, u
					}
				}
			}
		}
		m1, m2, i1, i2 := Inf, Inf, -1, -1
		for v := 1; v < N; v++ {
			c := w(0, v) + pi[0] + pi[v]
			if c < m1 {
				m1, m2, i1, i2 = c, m1, v, i1
			} else if c < m2 {
				m2, i2 = c, v
			}
		}
		deg[0] = 2
		deg[i1]++
		deg[i2]++
		total += m1 + m2
		for _, x := range pi {
			total -= 2 * x
		}
		return total
	}
	best := math.Inf(-1)
	bestTol := 0.0
	iters := 100 + N/2 // measured: at N=200, 2N iterations gain 1% of bound over N/2
	if lim := int(1.5e8 / float64(N*N)); iters > lim {
		iters = max(lim, 10)
	}
	lambda := 2.0
	since := 0
	for it := 0; it < iters; it++ {
		if it&7 == 0 && ctx.Err() != nil && it > 0 {
			break
		}
		L := oneTree()
		absPi := 0.0
		for _, x := range pi {
			absPi += math.Abs(x)
		}
		if L > best {
			best, since = L, 0
			bestTol = 1e-9*(math.Abs(ub)+2*absPi) + 1e-12
		} else if since++; since > N/4+5 {
			lambda /= 2
			since = 0
		}
		norm := 0.0
		for _, d := range deg {
			norm += float64((d - 2) * (d - 2))
		}
		if norm == 0 || lambda < 1e-6 {
			break // a tour: the bound is exact for it
		}
		t := lambda * math.Max(ub-L, 1e-9*math.Abs(ub)+1e-12) / norm
		for i := range pi {
			pi[i] += t * float64(deg[i]-2)
		}
	}
	b := best - bestTol
	if p.Integer {
		b = math.Ceil(b)
	}
	return math.Max(0, math.Min(b, ub))
}
