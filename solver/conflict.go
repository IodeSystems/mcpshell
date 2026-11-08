package solver

import (
	"context"
	"errors"
)

// Conflict explains an infeasible model: a set of constraints and variable
// bounds that cannot hold together. Indexes refer to Model.Cons and
// Model.Vars. A variable's default lower bound of 0 is a bound too.
type Conflict struct {
	Cons []int
	Lo   []int // variables whose lower bound is in the conflict
	Hi   []int // variables whose upper bound is in the conflict
	// Irreducible: dropping any one member makes the rest feasible (an IIS).
	// False when the search stopped early (time, or a MILP check reached the
	// node limit): the set is still infeasible but may be smaller.
	Irreducible bool
	// Integer: the set is feasible without the integer restrictions; it
	// conflicts only because the listed variables must be whole numbers.
	Integer bool
	Solves  int
}

type member struct {
	kind int // 0 constraint, 1 lower bound, 2 upper bound
	i    int
}

// ExplainInfeasible finds a conflict by the deletion filter: drop each
// constraint and bound in turn, and keep it out when the rest is still
// infeasible. It costs one solve per member. The model must be infeasible
// (ErrInvalid otherwise). When ctx ends during the search, the set found so
// far is returned with Irreducible false and no error.
func ExplainInfeasible(ctx context.Context, m *Model, lim Limits) (*Conflict, error) {
	lim = lim.fill()
	if _, err := m.compile(lim); err != nil {
		return nil, err
	}
	var all []member
	for i := range m.Cons {
		all = append(all, member{0, i})
	}
	for j, v := range m.Vars {
		if v.Lo != nil || !v.NoLo {
			all = append(all, member{1, j})
		}
		if v.Hi != nil {
			all = append(all, member{2, j})
		}
	}
	out := &Conflict{Irreducible: true}
	// feasible reports whether the model with only the members in keep is
	// feasible; known is false when the solve could not decide.
	feasible := func(keep map[member]bool, relax bool) (feas, known bool, err error) {
		sub := &Model{Sense: m.Sense}
		for _, v := range m.Vars {
			sub.Vars = append(sub.Vars, Var{Name: v.Name, Integer: v.Integer && !relax})
		}
		for _, mb := range all {
			if !keep[mb] {
				continue
			}
			switch mb.kind {
			case 0:
				sub.Cons = append(sub.Cons, m.Cons[mb.i])
			case 1:
				sub.Vars[mb.i].Lo = m.Vars[mb.i].Lo
			case 2:
				sub.Vars[mb.i].Hi = m.Vars[mb.i].Hi
			}
		}
		for j := range sub.Vars { // a variable whose lower bound is not a member
			if !keep[member{1, j}] {
				sub.Vars[j].NoLo, sub.Vars[j].Lo = true, nil
			}
		}
		out.Solves++
		// A feasibility question: no objective, so a MILP stops at its first
		// integer point.
		sub.Objective = nil
		r, err := Solve(ctx, sub, lim)
		if err != nil {
			return false, false, err
		}
		switch r.Status {
		case Infeasible:
			return false, true, nil
		case NodeLimit:
			if r.Values == nil {
				return false, false, nil
			}
		}
		return true, true, nil
	}
	stopped := func(err error) bool {
		return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
	}

	keep := map[member]bool{}
	for _, mb := range all {
		keep[mb] = true
	}
	feas, known, err := feasible(keep, false)
	if err != nil {
		return nil, err
	}
	if feas || !known {
		if !known {
			return nil, invalid("cannot explain: the model was not proven infeasible within limits.nodes")
		}
		return nil, invalid("the model is feasible; there is no conflict to explain")
	}
	for _, mb := range all {
		delete(keep, mb)
		feas, known, err := feasible(keep, false)
		if err != nil {
			if stopped(err) {
				keep[mb] = true
				out.Irreducible = false
				break
			}
			return nil, err
		}
		if !known {
			out.Irreducible = false
		}
		if feas || !known {
			keep[mb] = true // needed, or undecided: keep it to stay infeasible
		}
	}
	for _, mb := range all {
		if !keep[mb] {
			continue
		}
		switch mb.kind {
		case 0:
			out.Cons = append(out.Cons, mb.i)
		case 1:
			out.Lo = append(out.Lo, mb.i)
		case 2:
			out.Hi = append(out.Hi, mb.i)
		}
	}
	for _, v := range m.Vars {
		if v.Integer {
			if feas, _, err := feasible(keep, true); err == nil && feas {
				out.Integer = true
			}
			break
		}
	}
	return out, nil
}
