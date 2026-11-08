package graphalg

import (
	"container/heap"
	"slices"
)

// Visit is one node reached by Reach.
type Visit struct {
	Node, Depth int
	Parent      int // -1 for a source
}

// Reach lists the nodes reachable from srcs over adj, in breadth-first order
// (dfs false) or depth-first preorder (dfs true; neighbours in adjacency
// order, as recursion would visit them). Depth is the hop count from the
// nearest source for BFS and the tree depth for DFS. maxDepth < 0 is no
// limit.
func Reach(adj [][]int, srcs []int, dfs bool, maxDepth int) []Visit {
	seen := make([]bool, len(adj))
	var out []Visit
	if !dfs {
		for _, s := range srcs {
			if !seen[s] {
				seen[s] = true
				out = append(out, Visit{s, 0, -1})
			}
		}
		for i := 0; i < len(out); i++ {
			u := out[i]
			if maxDepth >= 0 && u.Depth >= maxDepth {
				continue
			}
			for _, v := range adj[u.Node] {
				if !seen[v] {
					seen[v] = true
					out = append(out, Visit{v, u.Depth + 1, u.Node})
				}
			}
		}
		return out
	}
	type frame struct{ v, depth, parent int }
	for _, s := range srcs {
		stack := []frame{{s, 0, -1}}
		for len(stack) > 0 {
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[f.v] {
				continue
			}
			seen[f.v] = true
			out = append(out, Visit{f.v, f.depth, f.parent})
			if maxDepth >= 0 && f.depth >= maxDepth {
				continue
			}
			for i := len(adj[f.v]) - 1; i >= 0; i-- {
				if w := adj[f.v][i]; !seen[w] {
					stack = append(stack, frame{w, f.depth + 1, f.v})
				}
			}
		}
	}
	return out
}

// Components labels the connected components of an undirected graph given
// by its edges. Labels are 0..count-1 in order of each component's lowest
// node.
func Components(n int, from, to []int) (label []int, count int) {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for k := range from {
		a, b := find(from[k]), find(to[k])
		if a != b {
			if a < b {
				parent[b] = a
			} else {
				parent[a] = b
			}
		}
	}
	label = make([]int, n)
	ids := map[int]int{}
	for v := 0; v < n; v++ {
		r := find(v)
		id, ok := ids[r]
		if !ok {
			id = len(ids)
			ids[r] = id
		}
		label[v] = id
	}
	return label, len(ids)
}

// SCC labels the strongly connected components of a directed graph
// (iterative Tarjan). Labels are 0..count-1 in order of each component's
// lowest node.
func SCC(adj [][]int) (label []int, count int) {
	n := len(adj)
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	comp := make([]int, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next := 0
	type frame struct{ v, i int }
	for s := 0; s < n; s++ {
		if index[s] >= 0 {
			continue
		}
		call := []frame{{s, 0}}
		index[s], low[s] = next, next
		next++
		stack = append(stack, s)
		onStack[s] = true
		for len(call) > 0 {
			f := &call[len(call)-1]
			v := f.v
			if f.i < len(adj[v]) {
				w := adj[v][f.i]
				f.i++
				if index[w] < 0 {
					index[w], low[w] = next, next
					next++
					stack = append(stack, w)
					onStack[w] = true
					call = append(call, frame{w, 0})
				} else if onStack[w] && index[w] < low[v] {
					low[v] = index[w]
				}
				continue
			}
			if low[v] == index[v] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp[w] = count
					if w == v {
						break
					}
				}
				count++
			}
			call = call[:len(call)-1]
			if len(call) > 0 {
				u := call[len(call)-1].v
				low[u] = min(low[u], low[v])
			}
		}
	}
	// Relabel by lowest node.
	ids := make([]int, count)
	for i := range ids {
		ids[i] = -1
	}
	label = make([]int, n)
	k := 0
	for v := 0; v < n; v++ {
		c := comp[v]
		if ids[c] < 0 {
			ids[c] = k
			k++
		}
		label[v] = ids[c]
	}
	return label, count
}

// FindCycle returns a directed cycle of adj in arc order (the arc from the
// last node to the first closes it), or nil. A self-loop is a cycle of one
// node.
func FindCycle(adj [][]int) []int {
	n := len(adj)
	color := make([]byte, n) // 0 new, 1 on the path, 2 done
	type frame struct{ v, i int }
	for s := 0; s < n; s++ {
		if color[s] != 0 {
			continue
		}
		path := []frame{{s, 0}}
		color[s] = 1
		for len(path) > 0 {
			f := &path[len(path)-1]
			if f.i < len(adj[f.v]) {
				w := adj[f.v][f.i]
				f.i++
				switch color[w] {
				case 0:
					color[w] = 1
					path = append(path, frame{w, 0})
				case 1:
					var cyc []int
					for j := len(path) - 1; j >= 0; j-- {
						cyc = append(cyc, path[j].v)
						if path[j].v == w {
							break
						}
					}
					slices.Reverse(cyc)
					return cyc
				}
				continue
			}
			color[f.v] = 2
			path = path[:len(path)-1]
		}
	}
	return nil
}

// FindCycleUndirected returns a cycle of an undirected graph given by its
// edges, or nil. Each edge is used once, so two edges between the same pair
// are a cycle of two nodes and a self-loop a cycle of one. Callers that
// treat a pair as one edge pass it once.
func FindCycleUndirected(n int, from, to []int) []int {
	type half struct{ to, edge int }
	adj := make([][]half, n)
	for k := range from {
		if from[k] == to[k] {
			return []int{from[k]}
		}
		adj[from[k]] = append(adj[from[k]], half{to[k], k})
		adj[to[k]] = append(adj[to[k]], half{from[k], k})
	}
	depth := make([]int, n)
	parent := make([]int, n)
	for i := range depth {
		depth[i] = -1
	}
	type frame struct{ v, in, i int }
	for s := 0; s < n; s++ {
		if depth[s] >= 0 {
			continue
		}
		depth[s], parent[s] = 0, -1
		stack := []frame{{s, -1, 0}}
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.i == len(adj[f.v]) {
				stack = stack[:len(stack)-1]
				continue
			}
			h := adj[f.v][f.i]
			f.i++
			if h.edge == f.in {
				continue
			}
			if depth[h.to] < 0 {
				depth[h.to], parent[h.to] = depth[f.v]+1, f.v
				stack = append(stack, frame{h.to, h.edge, 0})
				continue
			}
			// Back edge to an ancestor (DFS on an undirected graph has no
			// cross edges): the tree path from h.to down to f.v.
			var cyc []int
			for v := f.v; v != h.to; v = parent[v] {
				cyc = append(cyc, v)
			}
			cyc = append(cyc, h.to)
			slices.Reverse(cyc)
			return cyc
		}
	}
	return nil
}

type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// TopoSort orders the nodes of a directed graph so every arc goes forward.
// Among nodes that are ready, the lowest index comes first (Kahn's algorithm
// with a heap), so the order follows the input where it can. A cycle returns
// a *CycleError naming one.
func TopoSort(adj [][]int) ([]int, error) {
	n := len(adj)
	indeg := make([]int, n)
	for _, out := range adj {
		for _, v := range out {
			indeg[v]++
		}
	}
	h := &intHeap{}
	for v := 0; v < n; v++ {
		if indeg[v] == 0 {
			*h = append(*h, v)
		}
	}
	heap.Init(h)
	order := make([]int, 0, n)
	for h.Len() > 0 {
		u := heap.Pop(h).(int)
		order = append(order, u)
		for _, v := range adj[u] {
			indeg[v]--
			if indeg[v] == 0 {
				heap.Push(h, v)
			}
		}
	}
	if len(order) < n {
		return nil, &CycleError{FindCycle(adj)}
	}
	return order, nil
}
