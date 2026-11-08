package graphalg

import "sort"

// MST is a minimum spanning forest (Kruskal) of an undirected graph given
// by its edges: the indexes of the chosen edges, in the order chosen, and
// the number of trees (1 when the graph is connected). maximize picks a
// maximum spanning forest. Ties go to the lower edge index.
func MST(n int, from, to []int, w []float64, maximize bool) (chosen []int, trees int) {
	order := make([]int, len(from))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		if maximize {
			return w[order[a]] > w[order[b]]
		}
		return w[order[a]] < w[order[b]]
	})
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	trees = n
	for _, e := range order {
		a, b := find(from[e]), find(to[e])
		if a == b {
			continue
		}
		parent[a] = b
		chosen = append(chosen, e)
		trees--
	}
	return chosen, trees
}
