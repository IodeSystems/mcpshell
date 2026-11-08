package graphalg

import (
	"container/heap"
	"context"
	"math"
	"math/big"
)

// FlowArc is a directed arc of a flow network. Cap < 0 means no limit.
type FlowArc struct {
	From, To  int
	Cap, Cost int64
}

// FlowResult is a maximum flow or minimum-cost flow.
type FlowResult struct {
	Status Status
	// Value is the flow shipped: the maximum flow, or the supply shipped. For
	// an infeasible min-cost flow it is the most that can be shipped.
	Value int64
	// Cost is the total cost (min-cost flow only; nil when not optimal).
	Cost *big.Int
	// Flow is the flow on each input arc (nil when not optimal).
	Flow []int64
	// Source is the source side of a minimum cut (MaxFlow only): the nodes
	// the source reaches in the final residual network.
	Source []bool
}

// MaxInt is the largest capacity, supply or path cost the flow algorithms
// accept: sums below 2^62 cannot overflow int64.
const MaxInt = 1 << 62

// residual network: edges in pairs, e and e^1.
type rnet struct {
	head      [][]int
	to        []int
	cap, cost []int64
}

func newRnet(n int) *rnet { return &rnet{head: make([][]int, n)} }

func (r *rnet) add(u, v int, c, w int64) int {
	id := len(r.to)
	r.head[u] = append(r.head[u], id)
	r.head[v] = append(r.head[v], id+1)
	r.to = append(r.to, v, u)
	r.cap = append(r.cap, c, 0)
	r.cost = append(r.cost, w, -w)
	return id
}

// unboundedPath reports whether t is reachable from s over arcs with no limit.
func unboundedPath(n int, arcs []FlowArc, s, t int) bool {
	adj := make([][]int, n)
	for _, a := range arcs {
		if a.Cap < 0 {
			adj[a.From] = append(adj[a.From], a.To)
		}
	}
	for _, v := range Reach(adj, []int{s}, false, -1) {
		if v.Node == t {
			return true
		}
	}
	return false
}

// finiteCap is the sum of the finite capacities plus extra, or an error past MaxInt.
func finiteCap(arcs []FlowArc, extra int64) (int64, error) {
	sum := extra
	for i, a := range arcs {
		if a.Cap > MaxInt || sum > MaxInt-max(a.Cap, 0) {
			return 0, invalid("capacities sum past 2^62 at arc %d", i)
		}
		sum += max(a.Cap, 0)
	}
	return sum, nil
}

// MaxFlow is the maximum flow from s to t (Dinic). An s-t path of arcs with
// no limit is Unbounded. The context is checked every phase.
func MaxFlow(ctx context.Context, n int, arcs []FlowArc, s, t int) (*FlowResult, error) {
	if s == t {
		return nil, invalid("source and sink are the same node")
	}
	if unboundedPath(n, arcs, s, t) {
		return &FlowResult{Status: Unbounded}, nil
	}
	limit, err := finiteCap(arcs, 1)
	if err != nil {
		return nil, err
	}
	r := newRnet(n)
	ids := make([]int, len(arcs))
	for i, a := range arcs {
		c := a.Cap
		if c < 0 {
			c = limit
		}
		ids[i] = r.add(a.From, a.To, c, 0)
	}
	value, err := r.dinic(ctx, s, t, nil)
	if err != nil {
		return nil, err
	}
	res := &FlowResult{Status: Optimal, Value: value, Flow: make([]int64, len(arcs)), Source: make([]bool, n)}
	for i, id := range ids {
		res.Flow[i] = r.cap[id^1]
	}
	q := []int{s}
	res.Source[s] = true
	for len(q) > 0 {
		u := q[0]
		q = q[1:]
		for _, e := range r.head[u] {
			if v := r.to[e]; r.cap[e] > 0 && !res.Source[v] {
				res.Source[v] = true
				q = append(q, v)
			}
		}
	}
	return res, nil
}

// dinic is a maximum flow over the residual edges with cap > 0 that ok
// accepts (nil: all).
func (r *rnet) dinic(ctx context.Context, s, t int, ok func(e int) bool) (int64, error) {
	n := len(r.head)
	level := make([]int, n)
	it := make([]int, n)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		for i := range level {
			level[i] = -1
		}
		level[s] = 0
		q := []int{s}
		for len(q) > 0 {
			u := q[0]
			q = q[1:]
			for _, e := range r.head[u] {
				if v := r.to[e]; r.cap[e] > 0 && level[v] < 0 && (ok == nil || ok(e)) {
					level[v] = level[u] + 1
					q = append(q, v)
				}
			}
		}
		if level[t] < 0 {
			return total, nil
		}
		for i := range it {
			it[i] = 0
		}
		for {
			f := r.blocking(s, t, math.MaxInt64, level, it, ok)
			if f == 0 {
				break
			}
			total += f
		}
	}
}

// blocking pushes one augmenting path in the level graph (iterative DFS).
func (r *rnet) blocking(s, t int, limit int64, level, it []int, ok func(int) bool) int64 {
	var path []int // edge ids
	u := s
	for {
		if u == t {
			f := limit
			for _, e := range path {
				f = min(f, r.cap[e])
			}
			for _, e := range path {
				r.cap[e] -= f
				r.cap[e^1] += f
			}
			return f
		}
		advanced := false
		for ; it[u] < len(r.head[u]); it[u]++ {
			e := r.head[u][it[u]]
			if v := r.to[e]; r.cap[e] > 0 && level[v] == level[u]+1 && (ok == nil || ok(e)) {
				path = append(path, e)
				u = v
				advanced = true
				break
			}
		}
		if advanced {
			continue
		}
		if len(path) == 0 {
			return 0
		}
		// Dead end: retreat and skip the edge into u.
		level[u] = -1
		e := path[len(path)-1]
		path = path[:len(path)-1]
		u = r.to[e^1]
		it[u]++
	}
}

// MinCostFlow ships supply[v] (> 0 supply, < 0 demand; the sum must be 0)
// at minimum cost: successive shortest paths with potentials. Arcs with a
// negative cost are saturated first, so every residual cost starts >= 0.
// A negative-cost cycle of arcs with no limit is Unbounded. Supplies that
// cannot be shipped are Infeasible, with Value the most that can.
func MinCostFlow(ctx context.Context, n int, arcs []FlowArc, supply []int64) (*FlowResult, error) {
	var pos, sum int64
	for v, b := range supply {
		if b > MaxInt || b < -MaxInt || pos > MaxInt-max(b, 0) {
			return nil, invalid("supply at node %d is past 2^62", v)
		}
		pos += max(b, 0)
		sum += b
	}
	if sum != 0 {
		return nil, invalid("supplies sum to %d, not 0", sum)
	}
	var maxCost int64
	for _, a := range arcs {
		if a.Cost > MaxInt || a.Cost < -MaxInt {
			return nil, invalid("cost past 2^62")
		}
		maxCost = max(maxCost, a.Cost, -a.Cost)
	}
	// Potentials are shortest distances, at most (n+2) * maxCost; Dijkstra
	// adds a reduced cost to a distance, so keep that below 2^61.
	if maxCost > 0 && int64(n+2) > MaxInt/4/maxCost {
		return nil, invalid("%d nodes times the largest cost %d is past 2^60", n, maxCost)
	}
	if negativeUnlimitedCycle(n, arcs) {
		return &FlowResult{Status: Unbounded}, nil
	}
	// An optimal flow ships at most limit on an arc with no limit: paths
	// carry at most the supply, and a cycle that is not on unlimited arcs
	// alone carries at most a finite capacity.
	limit, err := finiteCap(arcs, pos)
	if err != nil {
		return nil, err
	}
	var negUnlimited int64
	for _, a := range arcs {
		if a.Cap < 0 && a.Cost < 0 {
			negUnlimited++
		}
	}
	if limit > MaxInt/(negUnlimited+2) {
		return nil, invalid("capacities and supplies are too large to bound the arcs with no limit")
	}
	S, T := n, n+1
	r := newRnet(n + 2)
	b := append([]int64(nil), supply...)
	ids := make([]int, len(arcs))
	for i, a := range arcs {
		c := a.Cap
		if c < 0 {
			c = limit
		}
		ids[i] = r.add(a.From, a.To, c, a.Cost)
		if a.Cost < 0 && c > 0 {
			id := ids[i]
			r.cap[id], r.cap[id^1] = 0, c
			b[a.From] -= c
			b[a.To] += c
		}
	}
	var need int64
	for v, x := range b {
		if x > 0 {
			r.add(S, v, x, 0)
			need += x
		} else if x < 0 {
			r.add(v, T, -x, 0)
		}
	}
	shipped, err := r.ssp(ctx, S, T)
	if err != nil {
		return nil, err
	}
	if shipped < need {
		// The most that can ship of the original supplies: max flow on the
		// network without costs.
		mf := make([]FlowArc, 0, len(arcs)+len(supply))
		for _, a := range arcs {
			mf = append(mf, FlowArc{From: a.From, To: a.To, Cap: a.Cap})
		}
		for v, x := range supply {
			if x > 0 {
				mf = append(mf, FlowArc{From: S, To: v, Cap: x})
			} else if x < 0 {
				mf = append(mf, FlowArc{From: v, To: T, Cap: -x})
			}
		}
		m, err := MaxFlow(ctx, n+2, mf, S, T)
		if err != nil {
			return nil, err
		}
		return &FlowResult{Status: Infeasible, Value: m.Value}, nil
	}
	res := &FlowResult{Status: Optimal, Value: pos, Flow: make([]int64, len(arcs)), Cost: new(big.Int)}
	t := new(big.Int)
	for i, id := range ids {
		res.Flow[i] = r.cap[id^1]
		res.Cost.Add(res.Cost, t.Mul(big.NewInt(res.Flow[i]), big.NewInt(arcs[i].Cost)))
	}
	return res, nil
}

// negativeUnlimitedCycle reports a negative-cost cycle made of arcs with no
// limit (Bellman-Ford from a virtual source joined to every node).
func negativeUnlimitedCycle(n int, arcs []FlowArc) bool {
	dist := make([]int64, n)
	for round := 0; round <= n; round++ {
		changed := false
		for _, a := range arcs {
			if a.Cap < 0 && dist[a.From]+a.Cost < dist[a.To] {
				dist[a.To] = dist[a.From] + a.Cost
				changed = true
			}
		}
		if !changed {
			return false
		}
	}
	return true
}

type costItem struct {
	v int
	d int64
}
type costHeap []costItem

func (h costHeap) Len() int           { return len(h) }
func (h costHeap) Less(i, j int) bool { return h[i].d < h[j].d || h[i].d == h[j].d && h[i].v < h[j].v }
func (h costHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *costHeap) Push(x any)        { *h = append(*h, x.(costItem)) }
func (h *costHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// ssp is primal-dual min-cost flow: Dijkstra on reduced costs finds the
// shortest S-T distance and updates the potentials, then a blocking flow
// (Dinic) on the arcs of reduced cost 0 ships along every shortest path at
// once. Every residual cost must start >= 0. Dijkstra stops when T is
// settled; nodes not settled get T's distance added to their potential,
// which keeps every reduced cost >= 0.
func (r *rnet) ssp(ctx context.Context, S, T int) (int64, error) {
	n := len(r.head)
	pot := make([]int64, n)
	dist := make([]int64, n)
	done := make([]bool, n)
	const unreached = math.MaxInt64
	admissible := func(e int) bool { return r.cost[e]+pot[r.to[e^1]]-pot[r.to[e]] == 0 }
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		for i := range dist {
			dist[i], done[i] = unreached, false
		}
		dist[S] = 0
		h := &costHeap{{S, 0}}
		for h.Len() > 0 {
			it := heap.Pop(h).(costItem)
			u := it.v
			if done[u] {
				continue
			}
			done[u] = true
			if u == T {
				break
			}
			for _, e := range r.head[u] {
				if r.cap[e] <= 0 {
					continue
				}
				v := r.to[e]
				if d := dist[u] + r.cost[e] + pot[u] - pot[v]; d < dist[v] {
					dist[v] = d
					heap.Push(h, costItem{v, d})
				}
			}
		}
		if !done[T] {
			return total, nil
		}
		dt := dist[T]
		for v := range pot {
			if done[v] {
				pot[v] += dist[v]
			} else {
				pot[v] += dt
			}
		}
		f, err := r.dinic(ctx, S, T, admissible)
		if err != nil {
			return 0, err
		}
		total += f
	}
}
