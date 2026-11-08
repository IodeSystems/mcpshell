// Package solver is an exact LP/MILP solver. All arithmetic is math/big.Rat:
// no float64 enters the algorithm, so results are exact and reproducible.
// The package does not depend on the runtime.
package solver

import (
	"errors"
	"fmt"
	"math/big"
)

// Sense is the optimization direction.
type Sense int

const (
	Minimize Sense = iota
	Maximize
)

// Op is a constraint relation.
type Op int

const (
	LE Op = iota // sum <= rhs
	GE           // sum >= rhs
	EQ           // sum == rhs
)

func (o Op) String() string {
	switch o {
	case LE:
		return "<="
	case GE:
		return ">="
	}
	return "="
}

// Var is a decision variable.
// Lo nil means 0, unless NoLo is set (then -infinity).
// Hi nil means +infinity.
type Var struct {
	Name    string
	Lo, Hi  *big.Rat
	NoLo    bool
	Integer bool
}

// Term is coef * variable.
type Term struct {
	Var  string
	Coef *big.Rat
}

// Constraint is sum(Terms) Op RHS. Repeated variables add up.
type Constraint struct {
	Terms []Term
	Op    Op
	RHS   *big.Rat
}

// Model is a linear or mixed-integer program.
type Model struct {
	Sense     Sense
	Objective []Term // may be empty (feasibility problem)
	Vars      []Var
	Cons      []Constraint
}

// Status is the outcome of Solve.
type Status int

const (
	Optimal Status = iota
	Infeasible
	// Unbounded: the LP relaxation is unbounded. For a MILP this assumes an
	// integer-feasible point exists (not checked).
	Unbounded
	// NodeLimit: branch and bound stopped at Limits.MaxNodes. Values, if
	// non-nil, hold the best incumbent.
	NodeLimit
)

func (s Status) String() string {
	return [...]string{"optimal", "infeasible", "unbounded", "node-limit"}[s]
}

// Result of Solve. Values and Slack align with Model.Vars and Model.Cons.
// Slack is rhs-lhs for <=, lhs-rhs for >=, and 0 for =.
type Result struct {
	Status    Status
	Objective *big.Rat   // nil without a solution
	Values    []*big.Rat // nil without a solution
	Slack     []*big.Rat
	Nodes     int // LPs solved (1 for a pure LP)
	Pivots    int
	// MILP only, set on NodeLimit: best bound over open nodes and the
	// absolute gap |incumbent - BestBound| (nil without an incumbent).
	BestBound *big.Rat
	Gap       *big.Rat
}

// Value returns the value of a named variable, or nil.
func (r *Result) Value(m *Model, name string) *big.Rat {
	if r.Values == nil {
		return nil
	}
	for i, v := range m.Vars {
		if v.Name == name {
			return r.Values[i]
		}
	}
	return nil
}

// Limits bound the work. A zero field takes the default.
type Limits struct {
	MaxVars   int // default 1000
	MaxCons   int // default 1000
	MaxNodes  int // default 10000; reaching it gives Status NodeLimit
	MaxPivots int // default 1000000 per LP; reaching it is a LimitError
}

// DefaultLimits returns the defaults.
func DefaultLimits() Limits {
	return Limits{MaxVars: 1000, MaxCons: 1000, MaxNodes: 10000, MaxPivots: 1000000}
}

func (l Limits) fill() Limits {
	d := DefaultLimits()
	if l.MaxVars <= 0 {
		l.MaxVars = d.MaxVars
	}
	if l.MaxCons <= 0 {
		l.MaxCons = d.MaxCons
	}
	if l.MaxNodes <= 0 {
		l.MaxNodes = d.MaxNodes
	}
	if l.MaxPivots <= 0 {
		l.MaxPivots = d.MaxPivots
	}
	return l
}

// LimitError says which limit was hit.
type LimitError struct {
	Limit string // "variables", "constraints" or "pivots"
	Max   int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("solver: limit hit: more than %d %s", e.Max, e.Limit)
}

// ErrInvalid wraps every malformed-model error.
var ErrInvalid = errors.New("solver: invalid model")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Rat returns the exact fraction a/b.
func Rat(a, b int64) *big.Rat { return big.NewRat(a, b) }

// Int returns the integer a as a Rat.
func Int(a int64) *big.Rat { return new(big.Rat).SetInt64(a) }

// prob is a validated model with variables resolved to indexes.
type prob struct {
	n       int
	lo, hi  []*big.Rat // nil = infinite
	integer []bool
	cons    []icon
	cost    []*big.Rat // minimization form
	sense   Sense
	obj     []*big.Rat // user objective, dense
}

type icon struct {
	a   []*big.Rat
	op  Op
	rhs *big.Rat
}

func (m *Model) compile(lim Limits) (*prob, error) {
	if len(m.Vars) == 0 {
		return nil, invalid("no variables")
	}
	if len(m.Vars) > lim.MaxVars {
		return nil, &LimitError{"variables", lim.MaxVars}
	}
	if len(m.Cons) > lim.MaxCons {
		return nil, &LimitError{"constraints", lim.MaxCons}
	}
	if m.Sense != Minimize && m.Sense != Maximize {
		return nil, invalid("unknown sense %d", m.Sense)
	}
	p := &prob{n: len(m.Vars), sense: m.Sense}
	idx := make(map[string]int, p.n)
	for j, v := range m.Vars {
		if v.Name == "" {
			return nil, invalid("variable %d has no name", j)
		}
		if _, dup := idx[v.Name]; dup {
			return nil, invalid("duplicate variable %q", v.Name)
		}
		idx[v.Name] = j
		var lo, hi *big.Rat
		switch {
		case v.Lo != nil:
			lo = new(big.Rat).Set(v.Lo)
		case !v.NoLo:
			lo = new(big.Rat)
		}
		if v.Hi != nil {
			hi = new(big.Rat).Set(v.Hi)
		}
		if v.Integer {
			if lo != nil {
				lo = ratCeil(lo)
			}
			if hi != nil {
				hi = ratFloor(hi)
			}
		}
		if lo != nil && hi != nil && lo.Cmp(hi) > 0 {
			return nil, invalid("variable %q: lower bound %s > upper bound %s", v.Name, lo.RatString(), hi.RatString())
		}
		p.lo = append(p.lo, lo)
		p.hi = append(p.hi, hi)
		p.integer = append(p.integer, v.Integer)
	}
	dense := func(ts []Term, what string) ([]*big.Rat, error) {
		a := zeros(p.n)
		for _, t := range ts {
			j, ok := idx[t.Var]
			if !ok {
				return nil, invalid("%s: unknown variable %q", what, t.Var)
			}
			if t.Coef == nil {
				return nil, invalid("%s: nil coefficient for %q", what, t.Var)
			}
			a[j].Add(a[j], t.Coef)
		}
		return a, nil
	}
	for i, c := range m.Cons {
		if c.RHS == nil {
			return nil, invalid("constraint %d: nil right-hand side", i)
		}
		if c.Op != LE && c.Op != GE && c.Op != EQ {
			return nil, invalid("constraint %d: unknown operator %d", i, c.Op)
		}
		a, err := dense(c.Terms, fmt.Sprintf("constraint %d", i))
		if err != nil {
			return nil, err
		}
		p.cons = append(p.cons, icon{a, c.Op, new(big.Rat).Set(c.RHS)})
	}
	obj, err := dense(m.Objective, "objective")
	if err != nil {
		return nil, err
	}
	p.obj = obj
	p.cost = make([]*big.Rat, p.n)
	for j := range obj {
		p.cost[j] = new(big.Rat).Set(obj[j])
		if m.Sense == Maximize {
			p.cost[j].Neg(p.cost[j])
		}
	}
	return p, nil
}

func zeros(n int) []*big.Rat {
	a := make([]*big.Rat, n)
	for i := range a {
		a[i] = new(big.Rat)
	}
	return a
}

func ratFloor(r *big.Rat) *big.Rat {
	q := new(big.Int).Div(r.Num(), r.Denom()) // Euclidean; denom > 0 so floor
	return new(big.Rat).SetInt(q)
}

func ratCeil(r *big.Rat) *big.Rat {
	q := new(big.Int).Neg(new(big.Int).Div(new(big.Int).Neg(r.Num()), r.Denom()))
	return new(big.Rat).SetInt(q)
}
