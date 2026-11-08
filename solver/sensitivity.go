package solver

import (
	"context"
	"math/big"
)

// Sensitivity is the LP sensitivity report at the optimal basis. All values
// are in the model's own sense (a maximize model reports how its maximum
// moves). Slices align with Model.Cons and Model.Vars. A nil range end is
// unlimited.
type Sensitivity struct {
	// ShadowPrice[i] is d(objective)/d(rhs of constraint i), valid while the
	// rhs stays inside [RHSLo[i], RHSHi[i]].
	ShadowPrice  []*big.Rat
	RHSLo, RHSHi []*big.Rat
	// ReducedCost[j] is d(objective)/d(x_j) when x_j is moved off its value
	// and the basic variables adjust: 0 for a variable strictly between its
	// bounds, the cost of pushing a variable that sits at a bound.
	ReducedCost []*big.Rat
	// The current solution stays optimal while the objective coefficient of
	// x_j stays inside [ObjLo[j], ObjHi[j]].
	ObjLo, ObjHi []*big.Rat
}

// SolveSensitivity solves an LP and returns its sensitivity report. A model
// with integer variables is ErrInvalid: shadow prices and ranges are LP
// notions. The report is nil unless Status is Optimal.
func SolveSensitivity(ctx context.Context, m *Model, lim Limits) (*Result, *Sensitivity, error) {
	lim = lim.fill()
	p, err := m.compile(lim)
	if err != nil {
		return nil, nil, err
	}
	for j, isInt := range p.integer {
		if isInt {
			return nil, nil, invalid("sensitivity needs an LP; %q is an integer variable", m.Vars[j].Name)
		}
	}
	bd := &budget{ctx: ctx, max: lim.MaxPivots}
	res := &Result{Nodes: 1}
	st, x, state, err := lpSolveState(bd, p, p.lo, p.hi)
	res.Pivots = bd.pivots
	if err != nil {
		return nil, nil, err
	}
	switch st {
	case lpInfeasible:
		res.Status = Infeasible
		return res, nil, nil
	case lpUnbounded:
		res.Status = Unbounded
		return res, nil, nil
	}
	res.Status = Optimal
	res.finish(p, m, x)
	return res, state.sensitivity(p), nil
}

func (s *lpState) sensitivity(p *prob) *Sensitivity {
	t := s.t
	z := t.T[t.m]
	ncons := len(p.cons)
	out := &Sensitivity{}
	tmp := new(big.Rat)
	flip := p.sense == Maximize // user objective = -(minimization objective)

	// Duals of the user constraints in minimization form: pi_i = -z of the
	// row's unit column, negated again when the row was negated.
	pi := make([]*big.Rat, ncons)
	for i := range ncons {
		v := new(big.Rat).Neg(z[s.unitCol[i]])
		if s.neg[i] {
			v.Neg(v)
		}
		pi[i] = v
		sp := new(big.Rat).Set(v)
		if flip {
			sp.Neg(sp)
		}
		out.ShadowPrice = append(out.ShadowPrice, sp)
	}

	// Right-hand-side ranges: x_B + d*col >= 0 over the kept rows, where col
	// is the unit column; a redundant row that uses the constraint pins it.
	for i := range ncons {
		col := s.unitCol[i]
		var inc, dec *big.Rat // allowed change of the tableau rhs; nil = unlimited
		pinned := false
		for _, row := range t.dropped {
			if row[col].Sign() != 0 {
				pinned = true
			}
		}
		if pinned {
			inc, dec = new(big.Rat), new(big.Rat)
		} else {
			for r := 0; r < t.m; r++ {
				a := t.T[r][col]
				if a.Sign() == 0 {
					continue
				}
				lim := new(big.Rat).Quo(t.T[r][t.n], a) // x_B / a
				if a.Sign() > 0 {                       // decreasing d drives this row to 0
					if dec == nil || lim.Cmp(dec) < 0 {
						dec = lim
					}
				} else {
					lim.Neg(lim)
					if inc == nil || lim.Cmp(inc) < 0 {
						inc = lim
					}
				}
			}
		}
		if s.neg[i] {
			inc, dec = dec, inc
		}
		rhs := p.cons[i].rhs
		out.RHSLo = append(out.RHSLo, offset(rhs, dec, -1))
		out.RHSHi = append(out.RHSHi, offset(rhs, inc, 1))
	}

	// Reduced costs from the duals: d_j = c_j - pi.A_j (minimization form).
	for j := 0; j < p.n; j++ {
		d := new(big.Rat).Set(p.cost[j])
		for i, c := range p.cons {
			if c.a[j].Sign() != 0 {
				d.Sub(d, tmp.Mul(pi[i], c.a[j]))
			}
		}
		if flip {
			d.Neg(d)
		}
		out.ReducedCost = append(out.ReducedCost, d)
	}

	// Objective ranges. Changing c_j by delta changes the tableau costs by
	// delta*g (g = +-1 on x_j's columns), and each reduced cost z_k by
	// delta*h_k, h_k = g_k - sum_r g_basis[r] * T[r][k]. The basis stays
	// optimal while every z_k + delta*h_k >= 0 for the columns that may enter.
	isBasic := make([]bool, t.n)
	for _, b := range t.basis {
		isBasic[b] = true
	}
	for j := 0; j < p.n; j++ {
		g := map[int]int64{}
		v := s.vm[j]
		switch v.kind {
		case kShift:
			g[v.c1] = 1
		case kNeg:
			g[v.c1] = -1
		default:
			g[v.c1], g[v.c2] = 1, -1
		}
		var up, down *big.Rat // allowed increase and decrease of c_j; nil = unlimited
		type gRow struct {
			r  int
			gb int64
		}
		var rows []gRow // the (at most two) rows whose basic column is x_j's
		for r, b := range t.basis {
			if gb := g[b]; gb != 0 {
				rows = append(rows, gRow{r, gb})
			}
		}
		h := new(big.Rat)
		for k := 0; k < s.artStart; k++ {
			if isBasic[k] {
				continue
			}
			h.SetInt64(g[k])
			for _, gr := range rows {
				if a := t.T[gr.r][k]; a.Sign() != 0 {
					if gr.gb > 0 {
						h.Sub(h, a)
					} else {
						h.Add(h, a)
					}
				}
			}
			switch h.Sign() {
			case 1: // a decrease of c_j lowers z_k
				lim := new(big.Rat).Quo(z[k], h)
				if down == nil || lim.Cmp(down) < 0 {
					down = lim
				}
			case -1:
				lim := new(big.Rat).Quo(z[k], h)
				lim.Neg(lim)
				if up == nil || lim.Cmp(up) < 0 {
					up = lim
				}
			}
		}
		c := new(big.Rat).Set(p.obj[j]) // user coefficient
		if flip {                       // user coefficient = -c_j
			up, down = down, up
		}
		out.ObjLo = append(out.ObjLo, offset(c, down, -1))
		out.ObjHi = append(out.ObjHi, offset(c, up, 1))
	}
	return out
}

// offset is base + sign*d, or nil when d is nil (unlimited).
func offset(base, d *big.Rat, sign int) *big.Rat {
	if d == nil {
		return nil
	}
	r := new(big.Rat).Set(d)
	if sign < 0 {
		r.Neg(r)
	}
	return r.Add(r, base)
}
