// Package routing has distance, shortest-path, travelling-salesman and
// vehicle-routing algorithms on float64 costs. It does not depend on the
// runtime. Long searches take a context.Context: a stopped search returns
// the best result found so far with Stopped set, not an error.
//
// Status rule: a result is Optimal only when it is proven (an exact method,
// or a heuristic result that meets its lower bound on integer costs).
// Heuristic results are Feasible and carry a lower bound.
package routing

import (
	"errors"
	"fmt"
	"math"
)

// Inf marks a missing arc.
var Inf = math.Inf(1)

// MaxNodes is the largest dense matrix the package builds (n*n float64).
const MaxNodes = 5000

// Matrix is a dense n x n cost matrix, row major. At(i, j) is the cost of
// the arc i -> j; Inf means no arc.
type Matrix struct {
	N int
	C []float64
}

// NewMatrix returns an n x n matrix with 0 on the diagonal and Inf elsewhere.
func NewMatrix(n int) (*Matrix, error) {
	if n > MaxNodes {
		return nil, &LimitError{What: "nodes", Max: MaxNodes, Got: n}
	}
	m := &Matrix{N: n, C: make([]float64, n*n)}
	for i := range m.C {
		if i%(n+1) != 0 {
			m.C[i] = Inf
		}
	}
	return m, nil
}

func (m *Matrix) At(i, j int) float64     { return m.C[i*m.N+j] }
func (m *Matrix) Set(i, j int, v float64) { m.C[i*m.N+j] = v }

// Symmetric reports whether At(i, j) == At(j, i) for all i, j.
func (m *Matrix) Symmetric() bool {
	for i := 0; i < m.N; i++ {
		for j := i + 1; j < m.N; j++ {
			if m.At(i, j) != m.At(j, i) {
				return false
			}
		}
	}
	return true
}

// Arc is one directed edge of a Graph.
type Arc struct {
	To int
	W  float64
}

// Graph is an adjacency list. Nodes are 0..N()-1. Weights must be >= 0.
type Graph struct {
	Adj [][]Arc
}

// NewGraph returns a graph with n nodes and no arcs.
func NewGraph(n int) *Graph { return &Graph{Adj: make([][]Arc, n)} }

// N is the node count.
func (g *Graph) N() int { return len(g.Adj) }

// AddArc adds from -> to with weight w.
func (g *Graph) AddArc(from, to int, w float64) {
	g.Adj[from] = append(g.Adj[from], Arc{to, w})
}

// Path is a node sequence and its total weight.
type Path struct {
	Dist  float64
	Nodes []int
}

// Status of a TSP or VRP result.
type Status int

const (
	// Optimal: proven optimal for the costs given.
	Optimal Status = iota
	// Feasible: a valid plan with a lower bound; not proven optimal.
	Feasible
	// Infeasible: at least one stop could not be served (VRP).
	Infeasible
)

func (s Status) String() string { return [...]string{"optimal", "feasible", "infeasible"}[s] }

// LimitError says which size limit was hit.
type LimitError struct {
	What     string
	Max, Got int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("routing: %d %s is over the limit of %d", e.Got, e.What, e.Max)
}

// ErrInvalid wraps every malformed-input error.
var ErrInvalid = errors.New("routing: invalid input")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// TSP is a travelling-salesman problem on all nodes of M.
//
// RoundTrip true: a tour that starts and ends at Start (End is ignored).
// RoundTrip false: a path from Start that visits every node and ends at End,
// or at any node when End < 0.
type TSP struct {
	M         *Matrix
	Start     int
	End       int // -1: free end (RoundTrip false only)
	RoundTrip bool
	// Integer: every cost is an integer, so float64 sums are exact. The
	// bound then rounds up, and a tour that meets the bound is Optimal.
	Integer bool
	// ExactMax is the largest node count Held-Karp solves. 0 = DefaultExactMax,
	// < 0 = never. At most HeldKarpMax.
	ExactMax int
	// Iterations is the iterated-local-search kick count. 0 = default
	// (DefaultIterations(n)), < 0 = none.
	Iterations int
	Seed       int64
}

// DefaultExactMax is the default largest node count solved exactly.
const DefaultExactMax = 13

// HeldKarpMax is the largest node count Held-Karp accepts: 2^(n-1)*(n-1)
// float64 and int8 entries (~85 MB at 20).
const HeldKarpMax = 20

// TSPResult is a tour or path. Order lists node indexes in visit order; a
// round trip ends with Start again.
type TSPResult struct {
	Status Status
	Order  []int
	Cost   float64
	// Bound is a lower bound on the optimal cost (== Cost when Optimal).
	Bound  float64
	Method string // "held-karp" or "heuristic"
	// Iterations is the number of kicks the search ran.
	Iterations int
	// Stopped is the context error that ended the search early, or nil.
	Stopped error
}

// Gap is (Cost - Bound) / Cost, 0 when Cost is 0.
func Gap(cost, bound float64) float64 {
	if cost <= 0 || bound >= cost {
		return 0
	}
	return (cost - bound) / cost
}

// Objective of a VRP.
type Objective int

const (
	MinDistance Objective = iota // total distance
	MinTime                      // total route duration (travel, waiting, service)
	MinVehicles                  // vehicles used, then total distance
)

// Stop is one customer. Close Inf = no window end.
type Stop struct {
	Node    int
	Demand  float64
	Open    float64
	Close   float64
	Service float64
}

// Vehicle is one vehicle. Inf in Capacity, MaxDist, MaxTime or Close = no limit.
// The route starts at node Start no earlier than Open and ends at node End
// no later than Close.
type Vehicle struct {
	Capacity float64
	MaxDist  float64
	MaxTime  float64
	Start    int
	End      int
	Open     float64
	Close    float64
}

// VRP is a capacitated vehicle-routing problem with time windows.
// Dist and Time are matrices on the same nodes (Time may be Dist).
type VRP struct {
	Dist, Time *Matrix
	Stops      []Stop
	Vehicles   []Vehicle
	Objective  Objective
	// Integer: every distance, time and window value is an integer (as TSP.Integer).
	Integer bool
	// Iterations is the ruin-and-recreate count. 0 = default, < 0 = none.
	Iterations int
	Seed       int64
}

// Route is the plan of one vehicle. Stops are indexes into VRP.Stops.
// Arrive[i] is the time service starts at Stops[i] (after any wait for the
// window). Time is Back - Depart.
type Route struct {
	Vehicle      int
	Stops        []int
	Load         float64
	Dist, Time   float64
	Depart, Back float64
	Arrive       []float64
}

// Violation is why a route breaks a rule.
type Violation int

const (
	OK Violation = iota
	OverCapacity
	LateWindow // a stop's window closes before the vehicle can start service
	OverShift  // the vehicle cannot be back by its Close
	OverTime   // route duration over MaxTime
	OverDist   // route distance over MaxDist
	NoRoom     // fits no vehicle alone, for more than one reason
	Crowded    // fits alone, but the plan has no room left
)

func (v Violation) String() string {
	return [...]string{"ok", "capacity", "window", "vehicle window", "maxTime", "maxDist", "no vehicle", "no room"}[v]
}

// Unassigned is a stop with no route and why. For OverCapacity, Value is
// the demand and Limit the largest capacity; for LateWindow, Value is the
// earliest arrival and Limit the window close.
type Unassigned struct {
	Stop         int
	Why          Violation
	Value, Limit float64
}

// VRPResult is a plan. Cost is the objective value (vehicles used for
// MinVehicles). Bound is a lower bound on the optimal cost when all stops
// are served.
type VRPResult struct {
	Status     Status
	Routes     []Route
	Unassigned []Unassigned
	Cost       float64
	Dist, Time float64 // totals over routes
	Bound      float64
	Iterations int
	Stopped    error
}
