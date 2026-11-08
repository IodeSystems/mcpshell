package cp

import (
	"errors"
	"fmt"
)

// Var is an integer decision variable: [Lo, Hi], or the listed Values when
// Values is not nil.
type Var struct {
	Name   string
	Lo, Hi int64
	Values []int64
}

// Op is the relation of a linear or count constraint.
type Op int

const (
	LE Op = iota
	GE
	EQ
	NE
)

func (o Op) String() string { return [...]string{"<=", ">=", "=", "!="}[o] }

// Kind selects a constraint type.
type Kind int

const (
	// Linear: sum(Coefs[i]*Vars[i]) Op RHS.
	Linear Kind = iota
	// AllDifferent: the Vars take pairwise different values.
	AllDifferent
	// Element: Vars[1] == Array[Vars[0]] (Vars[0] is a 0-based index).
	Element
	// Table: (Vars...) is one of Tuples.
	Table
	// Count: the number of Vars equal to Value, Op RHS (LE, GE or EQ).
	Count
	// Max: Vars[0] == max(Vars[1:]).
	Max
	// Min: Vars[0] == min(Vars[1:]).
	Min
	// Cumulative: at every time, the Demand of the Tasks running is at most
	// Capacity. A task runs in [start, start+Dur).
	Cumulative
	// NoOverlap: no two Tasks run at the same time.
	NoOverlap
	// Circuit: Vars[i] is the successor of node i (values 0..n-1); the
	// successors form one cycle through all nodes.
	Circuit
)

var kindNames = [...]string{"linear", "allDifferent", "element", "table", "count", "max", "min", "cumulative", "noOverlap", "circuit"}

func (k Kind) String() string { return kindNames[k] }

// Task is an interval for Cumulative and NoOverlap: a start variable, a
// fixed duration and a fixed demand.
type Task struct {
	Start  int
	Dur    int64
	Demand int64
}

// Constraint is one constraint; the fields used depend on Kind.
type Constraint struct {
	Kind     Kind
	Vars     []int
	Coefs    []int64
	Op       Op
	RHS      int64
	Value    int64     // Count
	Array    []int64   // Element
	Tuples   [][]int64 // Table
	Tasks    []Task    // Cumulative, NoOverlap
	Capacity int64     // Cumulative
}

// Term is Coef * Vars[Var] in the objective.
type Term struct {
	Var  int
	Coef int64
}

type Sense int

const (
	Minimize Sense = iota
	Maximize
)

// Model is a CP model. Objective nil means find a solution (or all of them).
type Model struct {
	Vars      []Var
	Cons      []Constraint
	Sense     Sense
	Objective []Term
	// SearchFirst lists variables to branch on before the others, earliest
	// lower bound first (start times: the "set times" order for schedules).
	SearchFirst []int
}

// Status of a search.
type Status int

const (
	// Optimal: the objective value is proven best.
	Optimal Status = iota
	// Feasible: a solution; with an objective, the search stopped before it
	// proved the best one (see Bound). Without an objective, a solution.
	Feasible
	// Infeasible: proven to have no solution.
	Infeasible
	// Unknown: the search stopped before it found a solution or proved none.
	Unknown
)

func (s Status) String() string {
	return [...]string{"optimal", "feasible", "infeasible", "unknown"}[s]
}

// Limits bound the search. A zero field takes the default.
type Limits struct {
	MaxNodes     int // default 1000000
	MaxSolutions int // with All: default 1000
	All          bool
}

// Result of Solve.
type Result struct {
	Status    Status
	Values    []int64 // best (or first) solution, aligned with Model.Vars
	Objective int64
	// Bound: no solution is better than this (minimization: objective >=
	// Bound). Equals Objective when Optimal. Valid only with an objective.
	Bound int64
	// Solutions: with Limits.All, every solution found, in search order.
	Solutions [][]int64
	// Complete: the search ran to the end (the list of solutions is whole,
	// the status is proven).
	Complete bool
	// Stop names what ended an incomplete search: "nodes", "solutions",
	// "time" or "".
	Stop  string
	Nodes int
	Fails int
}

// ErrInvalid wraps every malformed-model error.
var ErrInvalid = errors.New("cp: invalid model")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Size caps keep int64 arithmetic exact: |value| and |rhs| <= 1e12,
// |coef| <= 1e6, and at most 1e6 values in one domain.
const (
	MaxAbsValue = 1_000_000_000_000
	MaxAbsCoef  = 1_000_000
	MaxDomain   = 1 << 20
	maxTerms    = 1000
)

func checkValue(what string, v int64) error {
	if v > MaxAbsValue || v < -MaxAbsValue {
		return invalid("%s: %d is outside +-%d", what, v, int64(MaxAbsValue))
	}
	return nil
}

// validate checks the model and returns the variable domains.
func (m *Model) validate() ([]*dom, error) {
	if len(m.Vars) == 0 {
		return nil, invalid("no variables")
	}
	doms := make([]*dom, len(m.Vars))
	for i, v := range m.Vars {
		if v.Values != nil {
			if len(v.Values) == 0 {
				return nil, invalid("variable %q has an empty list of values", v.Name)
			}
			lo, hi := v.Values[0], v.Values[0]
			for _, x := range v.Values {
				if err := checkValue("variable "+v.Name, x); err != nil {
					return nil, err
				}
				lo, hi = min(lo, x), max(hi, x)
			}
			if hi-lo >= MaxDomain {
				return nil, invalid("variable %q: values span %d..%d, more than %d apart", v.Name, lo, hi, MaxDomain)
			}
			doms[i] = newDomValues(v.Values)
			continue
		}
		if err := checkValue("variable "+v.Name, v.Lo); err != nil {
			return nil, err
		}
		if err := checkValue("variable "+v.Name, v.Hi); err != nil {
			return nil, err
		}
		if v.Lo > v.Hi {
			return nil, invalid("variable %q: lower bound %d > upper bound %d", v.Name, v.Lo, v.Hi)
		}
		if v.Hi-v.Lo >= MaxDomain {
			return nil, invalid("variable %q: domain %d..%d has more than %d values", v.Name, v.Lo, v.Hi, MaxDomain)
		}
		doms[i] = newDom(v.Lo, v.Hi)
	}
	n := len(m.Vars)
	idx := func(where string, j int) error {
		if j < 0 || j >= n {
			return invalid("%s: variable index %d out of range", where, j)
		}
		return nil
	}
	for ci, c := range m.Cons {
		where := fmt.Sprintf("constraint %d (%s)", ci, c.Kind)
		for _, j := range c.Vars {
			if err := idx(where, j); err != nil {
				return nil, err
			}
		}
		for _, t := range c.Tasks {
			if err := idx(where, t.Start); err != nil {
				return nil, err
			}
			if t.Dur < 0 || t.Demand < 0 {
				return nil, invalid("%s: negative duration or demand", where)
			}
			if err := checkValue(where+" duration", t.Dur); err != nil {
				return nil, err
			}
		}
		switch c.Kind {
		case Linear:
			if len(c.Coefs) != len(c.Vars) {
				return nil, invalid("%s: %d coefficients for %d variables", where, len(c.Coefs), len(c.Vars))
			}
			if len(c.Vars) > maxTerms {
				return nil, invalid("%s: more than %d terms", where, maxTerms)
			}
			for k, a := range c.Coefs {
				if err := checkTerm(where, a, doms[c.Vars[k]], m.Vars[c.Vars[k]].Name); err != nil {
					return nil, err
				}
			}
			if err := checkValue(where+" rhs", c.RHS); err != nil {
				return nil, err
			}
		case Element:
			if len(c.Vars) != 2 || len(c.Array) == 0 {
				return nil, invalid("%s: needs an index, a value and a non-empty array", where)
			}
			for _, x := range c.Array {
				if err := checkValue(where+" array", x); err != nil {
					return nil, err
				}
			}
		case Table:
			for _, t := range c.Tuples {
				if len(t) != len(c.Vars) {
					return nil, invalid("%s: a tuple has %d values for %d variables", where, len(t), len(c.Vars))
				}
			}
		case Count:
			if c.Op == NE {
				return nil, invalid("%s: count takes le, ge or eq", where)
			}
		case Max, Min:
			if len(c.Vars) < 2 {
				return nil, invalid("%s: needs a target and at least one variable", where)
			}
		case Cumulative:
			if c.Capacity < 0 {
				return nil, invalid("%s: negative capacity", where)
			}
		case Circuit:
			if len(c.Vars) == 0 {
				return nil, invalid("%s: no nodes", where)
			}
		case AllDifferent, NoOverlap:
		default:
			return nil, invalid("%s: unknown kind", where)
		}
	}
	if len(m.Objective) > maxTerms {
		return nil, invalid("objective: more than %d terms", maxTerms)
	}
	for _, t := range m.Objective {
		if err := idx("objective", t.Var); err != nil {
			return nil, err
		}
		if err := checkTerm("objective", t.Coef, doms[t.Var], m.Vars[t.Var].Name); err != nil {
			return nil, err
		}
	}
	for _, j := range m.SearchFirst {
		if err := idx("search order", j); err != nil {
			return nil, err
		}
	}
	return doms, nil
}

// maxAbsTerm caps |coef*value| of one term, so a sum of maxTerms terms
// stays inside int64.
const maxAbsTerm = 1_000_000_000_000_000

func checkTerm(where string, a int64, d *dom, name string) error {
	if a > MaxAbsCoef || a < -MaxAbsCoef {
		return invalid("%s: coefficient %d is outside +-%d", where, a, int64(MaxAbsCoef))
	}
	big := max(d.hi, -d.lo)
	if a != 0 && big > maxAbsTerm/max(a, -a) {
		return invalid("%s: coefficient %d times the range of %q (%d..%d) is too large; keep |coef*value| <= 1e15", where, a, name, d.lo, d.hi)
	}
	return nil
}
