// Package graphalg has graph algorithms on index graphs: traversal,
// components, cycles, topological order, critical path, shortest paths with
// negative weights, maximum flow, minimum-cost flow, bipartite matching and
// minimum spanning trees. It does not depend on the runtime. Shortest paths
// with weights >= 0 are in package routing (Dijkstra, A*).
//
// Nodes are 0..n-1. Callers build the adjacency: an undirected edge is two
// arcs. Order is deterministic: ties go to the lower node or arc index, so
// the same input gives the same output.
//
// Flows and matching costs are int64: callers scale exact (rational) inputs
// to integers, so results are exact. Path, tree and schedule weights are
// float64; on integer-valued inputs below 2^53 their sums are exact.
package graphalg

import (
	"errors"
	"fmt"
)

// Arc is a weighted directed arc.
type Arc struct {
	To int
	W  float64
}

// Adj builds out-adjacency lists over n nodes from arc endpoints.
func Adj(n int, from, to []int) [][]int {
	adj := make([][]int, n)
	for k := range from {
		adj[from[k]] = append(adj[from[k]], to[k])
	}
	return adj
}

// ErrInvalid wraps every malformed-input error.
var ErrInvalid = errors.New("graphalg: invalid input")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// CycleError reports a directed cycle where a DAG is required. Nodes is the
// cycle in order; the arc Nodes[len-1] -> Nodes[0] closes it.
type CycleError struct{ Nodes []int }

func (e *CycleError) Error() string { return fmt.Sprintf("graphalg: cycle of %d nodes", len(e.Nodes)) }

// Status of an optimization result.
type Status int

const (
	// Optimal: proven optimal (every algorithm here is exact).
	Optimal Status = iota
	// Infeasible: the supplies or the amount cannot be shipped.
	Infeasible
	// Unbounded: the objective has no finite optimum.
	Unbounded
)

func (s Status) String() string { return [...]string{"optimal", "infeasible", "unbounded"}[s] }
