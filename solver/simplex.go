package solver

import (
	"context"
	"math/big"
)

type lpStatus int

const (
	lpOptimal lpStatus = iota
	lpInfeasible
	lpUnbounded
)

// budget counts pivots across one Solve and holds the context.
type budget struct {
	ctx    context.Context
	max    int
	pivots int
}

func (b *budget) tick() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.pivots >= b.max {
		return &LimitError{"pivots", b.max}
	}
	b.pivots++
	return nil
}

const (
	kShift = iota // x = lo + y
	kNeg          // x = hi - y
	kFree         // x = y1 - y2
)

type vmap struct{ kind, c1, c2 int }

// tab is a dense simplex tableau. Row m is the reduced-cost row; column n is
// the right-hand side.
type tab struct {
	m, n  int
	T     [][]*big.Rat
	basis []int
}

// lpSolve solves min cost.x over the constraints of p with the bounds lo, hi
// (nil = infinite). It returns the values of the original variables.
func lpSolve(bd *budget, p *prob, lo, hi []*big.Rat) (lpStatus, []*big.Rat, error) {
	// Variable substitution to y >= 0.
	vm := make([]vmap, p.n)
	ns := 0
	type ubRow struct {
		col int
		ub  *big.Rat
	}
	var ubs []ubRow
	for j := 0; j < p.n; j++ {
		switch {
		case lo[j] != nil:
			if hi[j] != nil {
				if lo[j].Cmp(hi[j]) > 0 {
					return lpInfeasible, nil, nil
				}
				ubs = append(ubs, ubRow{ns, new(big.Rat).Sub(hi[j], lo[j])})
			}
			vm[j] = vmap{kShift, ns, -1}
			ns++
		case hi[j] != nil:
			vm[j] = vmap{kNeg, ns, -1}
			ns++
		default:
			vm[j] = vmap{kFree, ns, ns + 1}
			ns += 2
		}
	}
	type row struct {
		a   []*big.Rat
		op  Op
		rhs *big.Rat
	}
	var rows []row
	tmp := new(big.Rat)
	for _, c := range p.cons {
		a := zeros(ns)
		rhs := new(big.Rat).Set(c.rhs)
		for j, cj := range c.a {
			if cj.Sign() == 0 {
				continue
			}
			v := vm[j]
			switch v.kind {
			case kShift:
				a[v.c1].Add(a[v.c1], cj)
				rhs.Sub(rhs, tmp.Mul(cj, lo[j]))
			case kNeg:
				a[v.c1].Sub(a[v.c1], cj)
				rhs.Sub(rhs, tmp.Mul(cj, hi[j]))
			default:
				a[v.c1].Add(a[v.c1], cj)
				a[v.c2].Sub(a[v.c2], cj)
			}
		}
		rows = append(rows, row{a, c.op, rhs})
	}
	for _, u := range ubs {
		a := zeros(ns)
		a[u.col].SetInt64(1)
		rows = append(rows, row{a, LE, u.ub})
	}
	cs := zeros(ns)
	for j, cj := range p.cost {
		if cj.Sign() == 0 {
			continue
		}
		v := vm[j]
		switch v.kind {
		case kShift:
			cs[v.c1].Add(cs[v.c1], cj)
		case kNeg:
			cs[v.c1].Sub(cs[v.c1], cj)
		default:
			cs[v.c1].Add(cs[v.c1], cj)
			cs[v.c2].Sub(cs[v.c2], cj)
		}
	}

	// Rows with rhs >= 0; count slack and artificial columns.
	m := len(rows)
	nslack, nart := 0, 0
	for i := range rows {
		if rows[i].rhs.Sign() < 0 {
			for _, x := range rows[i].a {
				x.Neg(x)
			}
			rows[i].rhs.Neg(rows[i].rhs)
			switch rows[i].op {
			case LE:
				rows[i].op = GE
			case GE:
				rows[i].op = LE
			}
		}
		switch rows[i].op {
		case LE:
			nslack++
		case GE:
			nslack++
			nart++
		default:
			nart++
		}
	}
	n := ns + nslack + nart
	artStart := ns + nslack
	t := &tab{m: m, n: n, basis: make([]int, m)}
	t.T = make([][]*big.Rat, m+1)
	for i := range t.T {
		t.T[i] = zeros(n + 1)
	}
	sc, ac := ns, artStart
	for i, r := range rows {
		for j := 0; j < ns; j++ {
			t.T[i][j].Set(r.a[j])
		}
		t.T[i][n].Set(r.rhs)
		switch r.op {
		case LE:
			t.T[i][sc].SetInt64(1)
			t.basis[i] = sc
			sc++
		case GE:
			t.T[i][sc].SetInt64(-1)
			sc++
			t.T[i][ac].SetInt64(1)
			t.basis[i] = ac
			ac++
		default:
			t.T[i][ac].SetInt64(1)
			t.basis[i] = ac
			ac++
		}
	}

	// Phase 1.
	if nart > 0 {
		c := zeros(n)
		for j := artStart; j < n; j++ {
			c[j].SetInt64(1)
		}
		t.setCost(c)
		if st, err := t.run(bd, n); err != nil {
			return 0, nil, err
		} else if st == lpUnbounded { // cannot happen: phase-1 objective >= 0
			return lpInfeasible, nil, nil
		}
		if t.T[m][n].Sign() != 0 { // -objective != 0
			return lpInfeasible, nil, nil
		}
		if err := t.dropArtificials(bd, artStart); err != nil {
			return 0, nil, err
		}
	}

	// Phase 2.
	c := zeros(n)
	for j := 0; j < ns; j++ {
		c[j].Set(cs[j])
	}
	t.setCost(c)
	st, err := t.run(bd, artStart)
	if err != nil {
		return 0, nil, err
	}
	if st == lpUnbounded {
		return lpUnbounded, nil, nil
	}

	y := zeros(ns)
	for i, b := range t.basis {
		if b < ns {
			y[b].Set(t.T[i][n])
		}
	}
	x := make([]*big.Rat, p.n)
	for j, v := range vm {
		x[j] = new(big.Rat)
		switch v.kind {
		case kShift:
			x[j].Add(lo[j], y[v.c1])
		case kNeg:
			x[j].Sub(hi[j], y[v.c1])
		default:
			x[j].Sub(y[v.c1], y[v.c2])
		}
	}
	return lpOptimal, x, nil
}

// setCost fills the reduced-cost row for cost vector c and the current basis.
func (t *tab) setCost(c []*big.Rat) {
	z := t.T[t.m]
	for j := 0; j < t.n; j++ {
		z[j].Set(c[j])
	}
	z[t.n].SetInt64(0)
	tmp := new(big.Rat)
	for i := 0; i < t.m; i++ {
		cb := c[t.basis[i]]
		if cb.Sign() == 0 {
			continue
		}
		for j := 0; j <= t.n; j++ {
			if t.T[i][j].Sign() != 0 {
				z[j].Sub(z[j], tmp.Mul(cb, t.T[i][j]))
			}
		}
	}
}

// run pivots with Bland's rule until optimal or unbounded. Only columns
// below limit may enter the basis.
func (t *tab) run(bd *budget, limit int) (lpStatus, error) {
	z := t.T[t.m]
	ratio := new(big.Rat)
	best := new(big.Rat)
	for {
		if err := bd.tick(); err != nil {
			return 0, err
		}
		enter := -1
		for j := 0; j < limit; j++ {
			if z[j].Sign() < 0 {
				enter = j
				break
			}
		}
		if enter < 0 {
			return lpOptimal, nil
		}
		leave := -1
		for i := 0; i < t.m; i++ {
			if t.T[i][enter].Sign() <= 0 {
				continue
			}
			ratio.Quo(t.T[i][t.n], t.T[i][enter])
			if leave < 0 {
				leave = i
				best.Set(ratio)
				continue
			}
			cmp := ratio.Cmp(best)
			if cmp < 0 || (cmp == 0 && t.basis[i] < t.basis[leave]) {
				leave = i
				best.Set(ratio)
			}
		}
		if leave < 0 {
			return lpUnbounded, nil
		}
		t.pivot(leave, enter)
	}
}

func (t *tab) pivot(r, c int) {
	pr := t.T[r]
	p := new(big.Rat).Set(pr[c])
	var nz []int
	for j := 0; j <= t.n; j++ {
		if pr[j].Sign() != 0 {
			pr[j].Quo(pr[j], p)
			nz = append(nz, j)
		}
	}
	f := new(big.Rat)
	tmp := new(big.Rat)
	for i := 0; i <= t.m; i++ {
		if i == r || t.T[i][c].Sign() == 0 {
			continue
		}
		f.Set(t.T[i][c])
		row := t.T[i]
		for _, j := range nz {
			row[j].Sub(row[j], tmp.Mul(f, pr[j]))
		}
	}
	if r < t.m {
		t.basis[r] = c
	}
}

// dropArtificials removes artificial variables left in the basis at level 0:
// pivot them out, or delete the row when it is redundant.
func (t *tab) dropArtificials(bd *budget, artStart int) error {
	keep := make([]int, 0, t.m)
	for i := 0; i < t.m; i++ {
		if t.basis[i] < artStart {
			keep = append(keep, i)
			continue
		}
		col := -1
		for j := 0; j < artStart; j++ {
			if t.T[i][j].Sign() != 0 {
				col = j
				break
			}
		}
		if col < 0 {
			continue // redundant row
		}
		if err := bd.tick(); err != nil {
			return err
		}
		t.pivot(i, col)
		keep = append(keep, i)
	}
	if len(keep) == t.m {
		return nil
	}
	nt := make([][]*big.Rat, 0, len(keep)+1)
	nb := make([]int, 0, len(keep))
	for _, i := range keep {
		nt = append(nt, t.T[i])
		nb = append(nb, t.basis[i])
	}
	nt = append(nt, t.T[t.m])
	t.T, t.basis, t.m = nt, nb, len(keep)
	return nil
}
