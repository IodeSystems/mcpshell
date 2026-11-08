package solver

import (
	"context"
	"math/big"
)

type node struct {
	lo, hi []*big.Rat
	bound  *big.Rat // parent LP objective (minimization form); nil at the root
}

// Solve solves the model. A pure LP is one simplex run; a model with integer
// variables is solved by depth-first branch and bound on the LP relaxation.
//
// Errors: ErrInvalid (malformed model), *LimitError (variables, constraints,
// pivots), or the context's error. Reaching Limits.MaxNodes is not an error:
// Status is NodeLimit and Values holds the best incumbent, if any.
func Solve(ctx context.Context, m *Model, lim Limits) (*Result, error) {
	lim = lim.fill()
	p, err := m.compile(lim)
	if err != nil {
		return nil, err
	}
	bd := &budget{ctx: ctx, max: lim.MaxPivots}
	res := &Result{}
	done := func() *Result { res.Pivots = bd.pivots; return res }

	var incX []*big.Rat
	var incObj *big.Rat // minimization form
	stack := []node{{p.lo, p.hi, nil}}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if incObj != nil && nd.bound != nil && nd.bound.Cmp(incObj) >= 0 {
			continue
		}
		if res.Nodes >= lim.MaxNodes {
			stack = append(stack, nd)
			res.Status = NodeLimit
			var bb *big.Rat
			for _, s := range stack {
				if s.bound != nil && (bb == nil || s.bound.Cmp(bb) < 0) {
					bb = s.bound
				}
			}
			if bb == nil || (incObj != nil && incObj.Cmp(bb) < 0) {
				bb = incObj
			}
			if bb != nil {
				res.BestBound = p.userObj(bb)
			}
			if incX != nil {
				res.finish(p, m, incX)
				res.Status = NodeLimit
				res.Gap = new(big.Rat).Sub(incObj, bb)
			}
			return done(), nil
		}
		res.Nodes++
		st, x, err := lpSolve(bd, p, nd.lo, nd.hi)
		if err != nil {
			return nil, err
		}
		switch st {
		case lpInfeasible:
			continue
		case lpUnbounded:
			// Only the root can be unbounded: children shrink the region.
			res.Status = Unbounded
			return done(), nil
		}
		obj := p.minObj(x)
		if incObj != nil && obj.Cmp(incObj) >= 0 {
			continue
		}
		j := -1
		for k := 0; k < p.n; k++ {
			if p.integer[k] && !x[k].IsInt() {
				j = k
				break
			}
		}
		if j < 0 {
			incX, incObj = x, obj
			continue
		}
		fl := ratFloor(x[j])
		up := node{cloneR(nd.lo), cloneR(nd.hi), obj}
		up.lo[j] = new(big.Rat).Add(fl, big.NewRat(1, 1))
		dn := node{cloneR(nd.lo), cloneR(nd.hi), obj}
		dn.hi[j] = fl
		stack = append(stack, up, dn) // down is explored first
	}
	if incX == nil {
		res.Status = Infeasible
		return done(), nil
	}
	res.finish(p, m, incX)
	res.Status = Optimal
	return done(), nil
}

func cloneR(a []*big.Rat) []*big.Rat { return append([]*big.Rat(nil), a...) }

// minObj is the objective in minimization form.
func (p *prob) minObj(x []*big.Rat) *big.Rat {
	s := new(big.Rat)
	t := new(big.Rat)
	for j, c := range p.cost {
		if c.Sign() != 0 {
			s.Add(s, t.Mul(c, x[j]))
		}
	}
	return s
}

func (p *prob) userObj(min *big.Rat) *big.Rat {
	r := new(big.Rat).Set(min)
	if p.sense == Maximize {
		r.Neg(r)
	}
	return r
}

// finish fills Objective, Values and Slack from x.
func (r *Result) finish(p *prob, m *Model, x []*big.Rat) {
	r.Values = x
	r.Objective = p.userObj(p.minObj(x))
	t := new(big.Rat)
	for _, c := range p.cons {
		lhs := new(big.Rat)
		for j, a := range c.a {
			if a.Sign() != 0 {
				lhs.Add(lhs, t.Mul(a, x[j]))
			}
		}
		s := new(big.Rat)
		switch c.op {
		case LE:
			s.Sub(c.rhs, lhs)
		case GE:
			s.Sub(lhs, c.rhs)
		}
		r.Slack = append(r.Slack, s)
	}
}
