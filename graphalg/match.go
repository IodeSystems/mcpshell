package graphalg

import (
	"context"
	"math/big"
	"sort"
)

// MatchEdge is an edge of a bipartite graph: left node L, right node R.
type MatchEdge struct {
	L, R int
	Cost int64
}

// Matching is a bipartite matching: Edges are the indexes of the chosen
// input edges.
type Matching struct {
	Edges []int
	Cost  *big.Int // weighted only
}

// MaxMatching is a maximum-cardinality matching (Hopcroft-Karp). Among the
// edges between the same pair, the first is chosen.
func MaxMatching(ctx context.Context, nl, nr int, edges []MatchEdge) (*Matching, error) {
	adj := make([][]int, nl) // edge indexes
	for i, e := range edges {
		adj[e.L] = append(adj[e.L], i)
	}
	matchL := make([]int, nl) // edge index or -1
	matchR := make([]int, nr)
	for i := range matchL {
		matchL[i] = -1
	}
	for i := range matchR {
		matchR[i] = -1
	}
	dist := make([]int, nl)
	const none = -1
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// BFS layers from the free left nodes.
		var q []int
		for u := range nl {
			if matchL[u] < 0 {
				dist[u] = 0
				q = append(q, u)
			} else {
				dist[u] = none
			}
		}
		found := false
		for i := 0; i < len(q); i++ {
			u := q[i]
			for _, ei := range adj[u] {
				r := edges[ei].R
				m := matchR[r]
				if m < 0 {
					found = true
				} else if w := edges[m].L; dist[w] == none {
					dist[w] = dist[u] + 1
					q = append(q, w)
				}
			}
		}
		if !found {
			break
		}
		// DFS along the layers (iterative).
		it := make([]int, nl)
		for s := range nl {
			if matchL[s] >= 0 {
				continue
			}
			path := []int{} // edge indexes
			u := s
			for {
				if it[u] == len(adj[u]) {
					dist[u] = none
					if len(path) == 0 {
						break
					}
					last := path[len(path)-1]
					path = path[:len(path)-1]
					u = edges[last].L
					it[u]++
					continue
				}
				ei := adj[u][it[u]]
				r := edges[ei].R
				m := matchR[r]
				if m < 0 {
					path = append(path, ei)
					for _, e := range path {
						matchL[edges[e].L] = e
						matchR[edges[e].R] = e
					}
					break
				}
				if w := edges[m].L; dist[w] == dist[u]+1 {
					path = append(path, ei)
					u = w
					continue
				}
				it[u]++
			}
		}
	}
	res := &Matching{}
	for _, e := range matchL {
		if e >= 0 {
			res.Edges = append(res.Edges, e)
		}
	}
	sort.Ints(res.Edges)
	return res, nil
}

// Assignment is a maximum-cardinality matching of minimum total cost: the
// size of MaxMatching, then a min-cost flow of that many units.
func Assignment(ctx context.Context, nl, nr int, edges []MatchEdge) (*Matching, error) {
	mm, err := MaxMatching(ctx, nl, nr, edges)
	if err != nil {
		return nil, err
	}
	k := int64(len(mm.Edges))
	S, T := nl+nr, nl+nr+1
	arcs := make([]FlowArc, 0, len(edges)+nl+nr)
	for _, e := range edges {
		arcs = append(arcs, FlowArc{From: e.L, To: nl + e.R, Cap: 1, Cost: e.Cost})
	}
	for u := range nl {
		arcs = append(arcs, FlowArc{From: S, To: u, Cap: 1})
	}
	for r := range nr {
		arcs = append(arcs, FlowArc{From: nl + r, To: T, Cap: 1})
	}
	supply := make([]int64, nl+nr+2)
	supply[S], supply[T] = k, -k
	f, err := MinCostFlow(ctx, nl+nr+2, arcs, supply)
	if err != nil {
		return nil, err
	}
	if f.Status != Optimal {
		return nil, invalid("assignment flow is %s", f.Status) // cannot happen: k units fit
	}
	res := &Matching{Cost: f.Cost}
	for i := range edges {
		if f.Flow[i] > 0 {
			res.Edges = append(res.Edges, i)
		}
	}
	return res, nil
}
