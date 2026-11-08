package routing

import (
	"container/heap"
	"context"
	"math"
	"slices"
)

// ShortestFrom runs Dijkstra from src over g. dist[v] is Inf when v cannot
// be reached; prev[v] is the node before v on a shortest path, or -1.
func ShortestFrom(ctx context.Context, g *Graph, src int) (dist []float64, prev []int, err error) {
	if err := checkNode(g, src, "from"); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	dist, prev = dijkstra(g, src, -1, nil)
	return dist, prev, nil
}

// ShortestPath is the shortest path from src to dst. h, if not nil, is a
// consistent lower bound on the distance from a node to dst (A*); nil is
// Dijkstra. ok is false when dst cannot be reached.
func ShortestPath(ctx context.Context, g *Graph, src, dst int, h func(int) float64) (p Path, ok bool, err error) {
	if err := checkNode(g, src, "from"); err != nil {
		return p, false, err
	}
	if err := checkNode(g, dst, "to"); err != nil {
		return p, false, err
	}
	if err := ctx.Err(); err != nil {
		return p, false, err
	}
	dist, prev := dijkstra(g, src, dst, h)
	if math.IsInf(dist[dst], 1) {
		return p, false, nil
	}
	return Path{Dist: dist[dst], Nodes: walkBack(prev, dst)}, true, nil
}

// PathTo rebuilds the node sequence to v from ShortestFrom's prev.
func PathTo(prev []int, v int) []int { return walkBack(prev, v) }

func walkBack(prev []int, v int) []int {
	var out []int
	for ; v >= 0; v = prev[v] {
		out = append(out, v)
	}
	slices.Reverse(out)
	return out
}

func checkNode(g *Graph, v int, what string) error {
	if v < 0 || v >= g.N() {
		return invalid("%s node %d is not in the graph (0..%d)", what, v, g.N()-1)
	}
	return nil
}

type pqItem struct {
	v int
	k float64 // dist + h
}

type pq []pqItem

func (q pq) Len() int           { return len(q) }
func (q pq) Less(i, j int) bool { return q[i].k < q[j].k }
func (q pq) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)        { *q = append(*q, x.(pqItem)) }
func (q *pq) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// dijkstra is Dijkstra (h nil) or A* (h a consistent heuristic towards
// dst). With dst >= 0 it stops when dst is settled. Lazy deletion: a node
// may be in the heap more than once.
func dijkstra(g *Graph, src, dst int, h func(int) float64) ([]float64, []int) {
	return dijkstraBan(g, src, dst, h, nil, nil)
}

// dijkstraBan is dijkstra with banned nodes and arcs (Yen's spur search).
func dijkstraBan(g *Graph, src, dst int, h func(int) float64, banNode []bool, banArc map[[2]int]bool) ([]float64, []int) {
	n := g.N()
	dist := make([]float64, n)
	prev := make([]int, n)
	done := make([]bool, n)
	for i := range dist {
		dist[i], prev[i] = Inf, -1
	}
	dist[src] = 0
	key := func(v int) float64 {
		if h == nil {
			return dist[v]
		}
		return dist[v] + h(v)
	}
	q := &pq{{src, key(src)}}
	for q.Len() > 0 {
		it := heap.Pop(q).(pqItem)
		u := it.v
		if done[u] {
			continue
		}
		done[u] = true
		if u == dst {
			break
		}
		for _, a := range g.Adj[u] {
			if done[a.To] || (banNode != nil && banNode[a.To]) || (banArc != nil && banArc[[2]int{u, a.To}]) {
				continue
			}
			if d := dist[u] + a.W; d < dist[a.To] {
				dist[a.To], prev[a.To] = d, u
				heap.Push(q, pqItem{a.To, key(a.To)})
			}
		}
	}
	return dist, prev
}

// KShortest returns up to k loopless paths from src to dst, shortest first
// (Yen's algorithm). Paths of equal length come in discovery order.
func KShortest(ctx context.Context, g *Graph, src, dst, k int) ([]Path, error) {
	first, ok, err := ShortestPath(ctx, g, src, dst, nil)
	if err != nil || !ok || k < 1 {
		return nil, err
	}
	paths := []Path{first}
	var cand []Path
	seen := map[string]bool{key(first.Nodes): true}
	for len(paths) < k {
		last := paths[len(paths)-1].Nodes
		for i := 0; i < len(last)-1; i++ {
			if err := ctx.Err(); err != nil {
				return paths, err
			}
			spur := last[i]
			root := last[:i+1]
			banArc := map[[2]int]bool{}
			for _, p := range paths {
				if len(p.Nodes) > i && slices.Equal(p.Nodes[:i+1], root) {
					banArc[[2]int{p.Nodes[i], p.Nodes[i+1]}] = true
				}
			}
			banNode := make([]bool, g.N())
			for _, v := range root[:i] {
				banNode[v] = true
			}
			dist, prev := dijkstraBan(g, spur, dst, nil, banNode, banArc)
			if math.IsInf(dist[dst], 1) {
				continue
			}
			nodes := append(slices.Clone(root[:i]), walkBack(prev, dst)...)
			kk := key(nodes)
			if seen[kk] {
				continue
			}
			seen[kk] = true
			cand = append(cand, Path{Dist: pathCost(g, nodes), Nodes: nodes})
		}
		if len(cand) == 0 {
			break
		}
		best := 0
		for i := range cand {
			if cand[i].Dist < cand[best].Dist {
				best = i
			}
		}
		paths = append(paths, cand[best])
		cand = slices.Delete(cand, best, best+1)
	}
	return paths, nil
}

// pathCost sums the cheapest arc between each consecutive pair.
func pathCost(g *Graph, nodes []int) float64 {
	total := 0.0
	for i := 0; i+1 < len(nodes); i++ {
		w := Inf
		for _, a := range g.Adj[nodes[i]] {
			if a.To == nodes[i+1] && a.W < w {
				w = a.W
			}
		}
		total += w
	}
	return total
}

func key(nodes []int) string {
	b := make([]byte, 0, len(nodes)*4)
	for _, v := range nodes {
		b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return string(b)
}
