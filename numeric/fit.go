package numeric

import (
	"context"
	"errors"
	"math"
)

// FitProblem is a weighted nonlinear least-squares problem:
// minimize sum_i (W_i * (Y_i - model_i(p)))^2.
type FitProblem struct {
	// Model writes the model value at every data point into out for
	// parameters p. It may write NaN or Inf: the step is then rejected.
	Model func(p []float64, out []float64)
	Y     []float64
	// W are weights 1/sigma_i (nil = all 1).
	W  []float64
	P0 []float64
	// Lo, Hi bound each parameter (nil or +-Inf = none). A step that leaves
	// the box is cut where it meets the boundary.
	Lo, Hi []float64
	// MaxIter defaults to 200; FTol and XTol to 1.5e-8 (scipy's defaults);
	// GTol to 1e-10.
	MaxIter          int
	FTol, XTol, GTol float64
}

// FitResult is the end of a Levenberg-Marquardt fit.
type FitResult struct {
	P []float64
	// Cov is s^2 (J^T W^2 J)^-1 with s^2 = SSR / (n - p) (scipy curve_fit
	// with absolute_sigma=False). nil when J^T J is singular.
	Cov []float64
	// Dependent is the first parameter that is not identifiable (a column
	// of J dependent on the others) when Cov is nil, else -1.
	Dependent  int
	SSR        float64 // weighted sum of squared residuals
	Fitted     []float64
	Iterations int
	Evals      int
	Status     Status
	Reason     string // "ftol", "xtol", "gtol", "exact" when Converged
	AtBound    []bool
	// DOF is n - p; with DOF <= 0 there is no residual variance and Cov
	// is nil.
	DOF int
}

// ErrBadStart is returned when the model is not finite at P0.
var ErrBadStart = errors.New("the model is not finite at the starting parameters")

// Fit runs Levenberg-Marquardt with Marquardt's diagonal scaling and
// Nielsen's damping update. The Jacobian is by forward differences during
// the search and by central differences at the end, for the covariance.
func Fit(ctx context.Context, pr FitProblem) (*FitResult, error) {
	n, np := len(pr.Y), len(pr.P0)
	if pr.MaxIter == 0 {
		pr.MaxIter = 200
	}
	if pr.FTol == 0 {
		pr.FTol = 1.5e-8
	}
	if pr.XTol == 0 {
		pr.XTol = 1.5e-8
	}
	if pr.GTol == 0 {
		pr.GTol = 1e-10
	}
	w := pr.W
	if w == nil {
		w = make([]float64, n)
		for i := range w {
			w[i] = 1
		}
	}
	lo, hi := pr.Lo, pr.Hi
	project := func(p []float64) {
		for j := range p {
			if lo != nil && p[j] < lo[j] {
				p[j] = lo[j]
			}
			if hi != nil && p[j] > hi[j] {
				p[j] = hi[j]
			}
		}
	}
	evals := 0
	f := make([]float64, n)
	// resid fills r with weighted residuals at p and returns the SSR
	// (+Inf when the model is not finite).
	resid := func(p, r []float64) float64 {
		evals++
		pr.Model(p, f)
		s := 0.0
		for i := range n {
			r[i] = w[i] * (pr.Y[i] - f[i])
			s += r[i] * r[i]
		}
		if !finite(s) {
			return math.Inf(1)
		}
		return s
	}
	p := append([]float64(nil), pr.P0...)
	project(p)
	r := make([]float64, n)
	ssr := resid(p, r)
	if math.IsInf(ssr, 1) {
		return nil, ErrBadStart
	}
	// J holds d(weighted model)/dp, column-major: J[j][i].
	J := make([][]float64, np)
	for j := range J {
		J[j] = make([]float64, n)
	}
	rt := make([]float64, n)
	jacobian := func(p []float64, central bool) {
		pp := append([]float64(nil), p...)
		for j := range np {
			h := math.Sqrt(eps) * math.Max(math.Abs(p[j]), 1)
			if central {
				h = math.Cbrt(eps) * math.Max(math.Abs(p[j]), 1)
			}
			// Step inside the bounds.
			up := hi == nil || p[j]+h <= hi[j]
			if !up {
				h = -h
			}
			pp[j] = p[j] + h
			resid(pp, rt)
			if central && (lo == nil || p[j]-h >= lo[j]) && (hi == nil || p[j]-h <= hi[j]) {
				pp[j] = p[j] - h
				fm := make([]float64, n)
				resid(pp, fm)
				for i := range n {
					// r = w(y - f): d(wf)/dp = -(dr/dp)
					J[j][i] = -(rt[i] - fm[i]) / (2 * h)
				}
			} else {
				for i := range n {
					J[j][i] = -(rt[i] - r[i]) / h
				}
			}
			pp[j] = p[j]
		}
	}
	A := make([]float64, np*np)
	g := make([]float64, np)
	normal := func() {
		for a := range np {
			for b := a; b < np; b++ {
				s := 0.0
				ja, jb := J[a], J[b]
				for i := range n {
					s += ja[i] * jb[i]
				}
				A[a*np+b], A[b*np+a] = s, s
			}
			s := 0.0
			for i := range n {
				s += J[a][i] * r[i]
			}
			g[a] = s
		}
	}

	res := &FitResult{Status: MaxIterations, Dependent: -1}
	lambda, nu := -1.0, 2.0
	trial := make([]float64, np)
	rtrial := make([]float64, n)
	for it := 1; it <= pr.MaxIter; it++ {
		if ctx.Err() != nil {
			res.Status = Stopped
			break
		}
		res.Iterations = it
		if ssr == 0 {
			res.Status, res.Reason = Converged, "exact"
			break
		}
		jacobian(p, false)
		normal()
		// Gradient test: max |cos angle| between r and the columns of J.
		// A parameter at a bound whose descent direction points out of the
		// box is held for this step (a simple active set). Projection alone
		// lets damped steps creep toward the bound without reaching it.
		held := make([]bool, np)
		for j := range np {
			held[j] = (hi != nil && p[j] >= hi[j] && g[j] > 0) || (lo != nil && p[j] <= lo[j] && g[j] < 0)
		}
		gmax := 0.0
		for j := range np {
			if A[j*np+j] > 0 && !held[j] {
				gmax = math.Max(gmax, math.Abs(g[j])/math.Sqrt(A[j*np+j]*ssr))
			}
		}
		if gmax <= pr.GTol {
			res.Status, res.Reason = Converged, "gtol"
			break
		}
		if lambda < 0 {
			mx := 0.0
			for j := range np {
				mx = math.Max(mx, A[j*np+j])
			}
			lambda = 1e-3 * mx
			if lambda == 0 {
				lambda = 1e-3
			}
		}
		accepted := false
		for inner := 0; inner < 60 && !accepted; inner++ {
			// (A + lambda D) delta = g, D = diag(A) (1 where it is 0).
			M := make([][]float64, np)
			for a := range np {
				M[a] = make([]float64, np)
				copy(M[a], A[a*np:(a+1)*np])
				d := A[a*np+a]
				if d == 0 {
					d = 1
				}
				M[a][a] += lambda * d
			}
			rhs := make([][]float64, np)
			for a := range np {
				rhs[a] = []float64{g[a]}
				if held[a] {
					for b := range np {
						M[a][b], M[b][a] = 0, 0
					}
					M[a][a], rhs[a][0] = 1, 0
				}
			}
			lu := FactorLU(M)
			sol, err := lu.Solve(rhs)
			if err != nil {
				lambda *= nu
				nu *= 2
				continue
			}
			// Truncate the step at the box: the parameter that limits it
			// lands on its bound (and is held next time if it pushes out).
			// Projecting the full step instead would keep its other
			// components at full length, and one overshooting step can
			// send a width to its lower bound, where the model degenerates.
			t := 1.0
			for j := range np {
				dj := sol[j][0]
				if dj > 0 && hi != nil && p[j]+dj > hi[j] {
					t = math.Min(t, (hi[j]-p[j])/dj)
				}
				if dj < 0 && lo != nil && p[j]+dj < lo[j] {
					t = math.Min(t, (lo[j]-p[j])/dj)
				}
			}
			for j := range np {
				trial[j] = p[j] + t*sol[j][0]
				if hi != nil && p[j]+sol[j][0] > hi[j] && t == (hi[j]-p[j])/sol[j][0] {
					trial[j] = hi[j]
				}
				if lo != nil && p[j]+sol[j][0] < lo[j] && t == (lo[j]-p[j])/sol[j][0] {
					trial[j] = lo[j]
				}
			}
			project(trial)
			delta := make([]float64, np)
			// small: every component of the step is within xtol of its
			// parameter (per parameter, so a large one does not hide a
			// relatively large step in a small one).
			small := true
			for j := range np {
				delta[j] = trial[j] - p[j]
				if math.Abs(delta[j]) > pr.XTol*(math.Abs(p[j])+pr.XTol) {
					small = false
				}
			}
			snew := resid(trial, rtrial)
			// Predicted reduction of the linear model: 2 d.g - d.A.d.
			pred := 0.0
			for a := range np {
				s := 0.0
				for b := range np {
					s += A[a*np+b] * delta[b]
				}
				pred += 2*delta[a]*g[a] - delta[a]*s
			}
			actual := ssr - snew
			// A projected step can lower the SSR although the linear model
			// predicts no gain for it: accept it, but damp more (rho 0).
			if snew < ssr {
				rho := 0.0
				if pred > 0 {
					rho = actual / pred
				}
				accepted = true
				copy(p, trial)
				copy(r, rtrial)
				old := ssr
				ssr = snew
				lambda *= math.Max(1.0/3, 1-math.Pow(2*rho-1, 3))
				nu = 2
				switch {
				case pred > 0 && actual <= pr.FTol*old && pred <= pr.FTol*old && rho <= 2:
					res.Status, res.Reason = Converged, "ftol"
				case small:
					// Only an accepted step counts: after rejections the
					// damping alone makes the step small.
					res.Status, res.Reason = Converged, "xtol"
				}
			} else {
				lambda *= nu
				nu *= 2
			}
			if !finite(lambda) || lambda > 1e300 {
				break
			}
		}
		if res.Status == Converged {
			break
		}
		if !accepted {
			// No step lowers the SSR: a minimum to working precision, or a
			// model that is not smooth enough for the difference Jacobian.
			res.Status = Stalled
			break
		}
	}
	res.P = p
	res.SSR = ssr
	res.Fitted = make([]float64, n)
	pr.Model(p, res.Fitted)
	evals++
	res.AtBound = make([]bool, np)
	for j := range np {
		res.AtBound[j] = (lo != nil && p[j] == lo[j]) || (hi != nil && p[j] == hi[j])
	}
	res.DOF = n - np
	resid(p, r)
	jacobian(p, true)
	res.Evals = evals
	if res.DOF <= 0 {
		return res, nil
	}
	jj := make([][]float64, n)
	for i := range n {
		jj[i] = make([]float64, np)
		for j := range np {
			jj[i][j] = J[j][i]
		}
	}
	q := FactorQR(jj)
	if d := q.Deficient(); d >= 0 {
		res.Dependent = d
		return res, nil
	}
	inv := q.RtRInverse()
	s2 := ssr / float64(res.DOF)
	res.Cov = make([]float64, np*np)
	for a := range np {
		for b := range np {
			res.Cov[a*np+b] = inv[a][b] * s2
		}
	}
	return res, nil
}
