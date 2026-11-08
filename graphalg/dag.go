package graphalg

import (
	"context"
	"math"
	"slices"
)

var inf = math.Inf(1)

// Schedule is a critical path (CPM) result. Times are from project start 0.
type Schedule struct {
	ES, EF, LS, LF, Slack []float64
	Critical              []bool
	Makespan              float64
	// Path is one critical path from a task that starts at 0 to one that
	// ends at Makespan, following arcs with no slack between them.
	Path []int
}

// CPM computes earliest and latest start and finish of each task. dur[v] is
// the duration of task v (>= 0); an arc u -> v with weight lag means v
// starts at least lag after u finishes. A task is critical when its slack
// (LS - ES) is <= tol: 0 for exact (integer-valued) inputs, a small
// tolerance for float ones. A cycle returns a *CycleError.
func CPM(adj [][]Arc, dur []float64, tol float64) (*Schedule, error) {
	n := len(adj)
	plain := make([][]int, n)
	for u, out := range adj {
		for _, a := range out {
			plain[u] = append(plain[u], a.To)
		}
	}
	order, err := TopoSort(plain)
	if err != nil {
		return nil, err
	}
	s := &Schedule{
		ES: make([]float64, n), EF: make([]float64, n),
		LS: make([]float64, n), LF: make([]float64, n),
		Slack: make([]float64, n), Critical: make([]bool, n),
	}
	for _, u := range order {
		s.EF[u] = s.ES[u] + dur[u]
		s.Makespan = math.Max(s.Makespan, s.EF[u])
		for _, a := range adj[u] {
			s.ES[a.To] = math.Max(s.ES[a.To], s.EF[u]+a.W)
		}
	}
	for i := range s.LF {
		s.LF[i] = s.Makespan
	}
	for i := n - 1; i >= 0; i-- {
		u := order[i]
		for _, a := range adj[u] {
			s.LF[u] = math.Min(s.LF[u], s.LS[a.To]-a.W)
		}
		s.LS[u] = s.LF[u] - dur[u]
		s.Slack[u] = s.LS[u] - s.ES[u]
		s.Critical[u] = s.Slack[u] <= tol
	}
	// One critical path: from the first critical task (topological order)
	// that starts at 0, follow tight arcs to critical tasks. A critical task
	// that does not end at the makespan has such an arc (its LF comes from
	// a successor whose LS is tight), so the walk ends at the makespan.
	tight := func(x, y float64) bool { return math.Abs(x-y) <= tol }
	for _, u := range order {
		if s.Critical[u] && tight(s.ES[u], 0) {
			s.Path = append(s.Path, u)
			for {
				next := -1
				for _, a := range adj[u] {
					if s.Critical[a.To] && tight(s.EF[u]+a.W, s.ES[a.To]) {
						next = a.To
						break
					}
				}
				if next < 0 {
					break
				}
				u = next
				s.Path = append(s.Path, u)
			}
			break
		}
	}
	return s, nil
}

// Longest is the longest path from src in a DAG: dist[v] is -Inf when v
// cannot be reached; prev[v] is the node before v, or -1. A cycle anywhere
// in the graph returns a *CycleError (a longest path needs a DAG).
func Longest(adj [][]Arc, src int) (dist []float64, prev []int, err error) {
	n := len(adj)
	plain := make([][]int, n)
	for u, out := range adj {
		for _, a := range out {
			plain[u] = append(plain[u], a.To)
		}
	}
	order, err := TopoSort(plain)
	if err != nil {
		return nil, nil, err
	}
	dist = make([]float64, n)
	prev = make([]int, n)
	for i := range dist {
		dist[i], prev[i] = -inf, -1
	}
	dist[src] = 0
	for _, u := range order {
		if math.IsInf(dist[u], -1) {
			continue
		}
		for _, a := range adj[u] {
			if d := dist[u] + a.W; d > dist[a.To] {
				dist[a.To], prev[a.To] = d, u
			}
		}
	}
	return dist, prev, nil
}

// NegativeCycleError reports a cycle of negative total weight reachable
// from the source. Nodes is the cycle in arc order.
type NegativeCycleError struct{ Nodes []int }

func (e *NegativeCycleError) Error() string { return "graphalg: negative cycle" }

// BellmanFord is the shortest path tree from src with weights of any sign.
// dist[v] is +Inf when v cannot be reached. A negative cycle reachable from
// src returns a *NegativeCycleError. The context is checked every round.
func BellmanFord(ctx context.Context, adj [][]Arc, src int) (dist []float64, prev []int, err error) {
	n := len(adj)
	dist = make([]float64, n)
	prev = make([]int, n)
	for i := range dist {
		dist[i], prev[i] = inf, -1
	}
	dist[src] = 0
	last := -1
	for round := 0; round < n; round++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		last = -1
		for u, out := range adj {
			if math.IsInf(dist[u], 1) {
				continue
			}
			for _, a := range out {
				if d := dist[u] + a.W; d < dist[a.To] {
					dist[a.To], prev[a.To] = d, u
					last = a.To
				}
			}
		}
		if last < 0 {
			return dist, prev, nil
		}
	}
	// A change in round n: last is on or after a negative cycle. Walking
	// prev n times lands on the cycle.
	v := last
	for range n {
		v = prev[v]
	}
	cyc := []int{v}
	for u := prev[v]; u != v; u = prev[u] {
		cyc = append(cyc, u)
	}
	slices.Reverse(cyc)
	return nil, nil, &NegativeCycleError{cyc}
}
