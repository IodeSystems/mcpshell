package numeric

import (
	"context"
	"math"
	"sort"
)

// RootResult is the end of a Brent root search.
type RootResult struct {
	X, FX      float64
	Iterations int
	Status     Status
}

// Brent finds a root of f in [a, b], where fa = f(a) and fb = f(b) have
// opposite signs (or one is 0), by Brent's method (bisection, secant and
// inverse quadratic steps, as scipy's brentq). It stops when the bracket
// is narrower than 2*(xtol + rtol*|x|)/2 or f(x) = 0.
func Brent(f func(float64) float64, a, b, fa, fb, xtol, rtol float64, maxIter int) RootResult {
	if fa == 0 {
		return RootResult{X: a, FX: 0, Status: Converged}
	}
	if fb == 0 {
		return RootResult{X: b, FX: 0, Status: Converged}
	}
	xpre, xcur := a, b
	fpre, fcur := fa, fb
	xblk, fblk := 0.0, 0.0
	spre, scur := 0.0, 0.0
	for i := 1; i <= maxIter; i++ {
		if fpre != 0 && fcur != 0 && math.Signbit(fpre) != math.Signbit(fcur) {
			xblk, fblk = xpre, fpre
			spre = xcur - xpre
			scur = spre
		}
		if math.Abs(fblk) < math.Abs(fcur) {
			xpre, xcur, xblk = xcur, xblk, xcur
			fpre, fcur, fblk = fcur, fblk, fcur
		}
		delta := (xtol + rtol*math.Abs(xcur)) / 2
		sbis := (xblk - xcur) / 2
		if fcur == 0 || math.Abs(sbis) < delta {
			return RootResult{X: xcur, FX: fcur, Iterations: i, Status: Converged}
		}
		if math.Abs(spre) > delta && math.Abs(fcur) < math.Abs(fpre) {
			var stry float64
			if xpre == xblk {
				// secant
				stry = -fcur * (xcur - xpre) / (fcur - fpre)
			} else {
				// inverse quadratic
				dpre := (fpre - fcur) / (xpre - xcur)
				dblk := (fblk - fcur) / (xblk - xcur)
				stry = -fcur * (fblk*dblk - fpre*dpre) / (dblk * dpre * (fblk - fpre))
			}
			if 2*math.Abs(stry) < math.Min(math.Abs(spre), 3*math.Abs(sbis)-delta) {
				spre, scur = scur, stry
			} else {
				spre, scur = sbis, sbis
			}
		} else {
			spre, scur = sbis, sbis
		}
		xpre, fpre = xcur, fcur
		if math.Abs(scur) > delta {
			xcur += scur
		} else if sbis > 0 {
			xcur += delta
		} else {
			xcur -= delta
		}
		fcur = f(xcur)
	}
	return RootResult{X: xcur, FX: fcur, Iterations: maxIter, Status: MaxIterations}
}

// MinimizeOptions configures NelderMead. Zero values take the defaults.
type MinimizeOptions struct {
	// Lo and Hi are per-coordinate bounds (nil = none; +-Inf = none).
	Lo, Hi []float64
	// XTol, FTol: converged when every vertex is within XTol*(1+|x_best|)
	// of the best one in each coordinate and every value within
	// FTol*(1+|f_best|) of the best value. Default 1e-8 each.
	XTol, FTol float64
	// MaxIter defaults to 200 * dimension; MaxEvals to 400 * dimension.
	MaxIter, MaxEvals int
}

// MinimizeResult is the end of a Nelder-Mead run.
type MinimizeResult struct {
	X                 []float64
	F                 float64
	Iterations, Evals int
	Status            Status
}

// NelderMead minimizes f from x0 with the adaptive Nelder-Mead simplex
// (Gao and Han 2012 coefficients, as scipy's adaptive=True). The initial
// simplex steps 5% along each axis (0.00025 for a zero coordinate), as
// scipy does. Points are clipped into the bounds. f may return NaN or
// +Inf: such a point is worse than any finite one.
func NelderMead(ctx context.Context, f func([]float64) float64, x0 []float64, o MinimizeOptions) MinimizeResult {
	n := len(x0)
	fn := float64(n)
	alpha, gamma := 1.0, 1+2/fn
	rho, sigma := 0.75-1/(2*fn), 1-1/fn
	if n == 1 {
		gamma, rho, sigma = 2, 0.5, 0.5
	}
	if o.XTol == 0 {
		o.XTol = 1e-8
	}
	if o.FTol == 0 {
		o.FTol = 1e-8
	}
	if o.MaxIter == 0 {
		o.MaxIter = 200 * n
	}
	if o.MaxEvals == 0 {
		o.MaxEvals = 400 * n
	}
	clip := func(x []float64) {
		for i := range x {
			if o.Lo != nil && x[i] < o.Lo[i] {
				x[i] = o.Lo[i]
			}
			if o.Hi != nil && x[i] > o.Hi[i] {
				x[i] = o.Hi[i]
			}
		}
	}
	evals := 0
	eval := func(x []float64) float64 {
		evals++
		v := f(x)
		if math.IsNaN(v) {
			return math.Inf(1)
		}
		return v
	}
	sim := make([][]float64, n+1)
	fs := make([]float64, n+1)
	sim[0] = append([]float64(nil), x0...)
	clip(sim[0])
	for i := range n {
		p := append([]float64(nil), sim[0]...)
		if p[i] != 0 {
			p[i] *= 1.05
		} else {
			p[i] = 0.00025
		}
		clip(p)
		if p[i] == sim[0][i] { // at a bound: step the other way
			if sim[0][i] != 0 {
				p[i] = sim[0][i] * 0.95
			} else {
				p[i] = -0.00025
			}
			clip(p)
		}
		sim[i+1] = p
	}
	for i := range sim {
		fs[i] = eval(sim[i])
	}
	order := func() {
		idx := make([]int, n+1)
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return fs[idx[a]] < fs[idx[b]] })
		s2, f2 := make([][]float64, n+1), make([]float64, n+1)
		for i, j := range idx {
			s2[i], f2[i] = sim[j], fs[j]
		}
		sim, fs = s2, f2
	}
	point := func(c []float64, t float64, w []float64) []float64 {
		p := make([]float64, n)
		for i := range p {
			p[i] = c[i] + t*(c[i]-w[i])
		}
		clip(p)
		return p
	}
	order()
	iter := 0
	status := MaxIterations
	for iter < o.MaxIter && evals < o.MaxEvals {
		if ctx.Err() != nil {
			status = Stopped
			break
		}
		// Convergence test.
		done := true
		for i := 1; i <= n && done; i++ {
			if !(math.Abs(fs[i]-fs[0]) <= o.FTol*(1+math.Abs(fs[0]))) {
				done = false
			}
			for j := 0; j < n && done; j++ {
				if !(math.Abs(sim[i][j]-sim[0][j]) <= o.XTol*(1+math.Abs(sim[0][j]))) {
					done = false
				}
			}
		}
		if done {
			status = Converged
			break
		}
		iter++
		c := make([]float64, n)
		for i := range n {
			for j := range n {
				c[j] += sim[i][j]
			}
		}
		for j := range c {
			c[j] /= fn
		}
		worst := sim[n]
		xr := point(c, alpha, worst)
		fr := eval(xr)
		switch {
		case fr < fs[0]:
			xe := point(c, alpha*gamma, worst)
			fe := eval(xe)
			if fe < fr {
				sim[n], fs[n] = xe, fe
			} else {
				sim[n], fs[n] = xr, fr
			}
		case fr < fs[n-1]:
			sim[n], fs[n] = xr, fr
		default:
			shrink := false
			if fr < fs[n] {
				xc := point(c, alpha*rho, worst)
				fc := eval(xc)
				if fc <= fr {
					sim[n], fs[n] = xc, fc
				} else {
					shrink = true
				}
			} else {
				xcc := point(c, -rho, worst)
				fcc := eval(xcc)
				if fcc < fs[n] {
					sim[n], fs[n] = xcc, fcc
				} else {
					shrink = true
				}
			}
			if shrink {
				for i := 1; i <= n; i++ {
					p := make([]float64, n)
					for j := range p {
						p[j] = sim[0][j] + sigma*(sim[i][j]-sim[0][j])
					}
					clip(p)
					sim[i], fs[i] = p, eval(p)
				}
			}
		}
		order()
	}
	return MinimizeResult{X: sim[0], F: fs[0], Iterations: iter, Evals: evals, Status: status}
}
