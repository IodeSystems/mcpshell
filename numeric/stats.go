package numeric

import (
	"errors"
	"math"
)

// MeanVar returns the mean and the sample variance (n - 1) by two passes.
func MeanVar(xs []float64) (mean, variance float64) {
	n := float64(len(xs))
	for _, x := range xs {
		mean += x
	}
	mean /= n
	var ss, comp float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
		comp += d
	}
	variance = (ss - comp*comp/n) / (n - 1)
	return
}

// Alternative is the alternative hypothesis of a test.
type Alternative int

const (
	TwoSided Alternative = iota
	Less                 // the true mean (difference) is less than mu
	Greater
)

// TTestResult is a t test with its confidence interval.
type TTestResult struct {
	T, DF, P float64
	// Estimate is the mean (one-sample), the mean of the differences
	// (paired) or mean(a) - mean(b) (two-sample).
	Estimate, StdErr float64
	// CI is the confidence interval for the estimate (one-sided tests have
	// an infinite end).
	CI [2]float64
}

func tResult(est, se, df, mu float64, alt Alternative, level float64) TTestResult {
	t := (est - mu) / se
	d := StudentT{df}
	r := TTestResult{T: t, DF: df, Estimate: est, StdErr: se}
	switch alt {
	case TwoSided:
		r.P = math.Min(1, 2*d.SF(math.Abs(t)))
		q := d.PPF(1 - (1-level)/2)
		r.CI = [2]float64{est - q*se, est + q*se}
	case Less:
		r.P = d.CDF(t)
		r.CI = [2]float64{math.Inf(-1), est + d.PPF(level)*se}
	case Greater:
		r.P = d.SF(t)
		r.CI = [2]float64{est - d.PPF(level)*se, math.Inf(1)}
	}
	return r
}

// TTest1 tests mean(xs) = mu (a paired test is TTest1 on the differences).
func TTest1(xs []float64, mu float64, alt Alternative, level float64) TTestResult {
	m, v := MeanVar(xs)
	n := float64(len(xs))
	return tResult(m, math.Sqrt(v/n), n-1, mu, alt, level)
}

// TTest2 tests mean(a) - mean(b) = mu: Welch's test, or Student's pooled
// test with equalVar.
func TTest2(a, b []float64, mu float64, equalVar bool, alt Alternative, level float64) TTestResult {
	ma, va := MeanVar(a)
	mb, vb := MeanVar(b)
	na, nb := float64(len(a)), float64(len(b))
	var se, df float64
	if equalVar {
		df = na + nb - 2
		sp := ((na-1)*va + (nb-1)*vb) / df
		se = math.Sqrt(sp * (1/na + 1/nb))
	} else {
		qa, qb := va/na, vb/nb
		se = math.Sqrt(qa + qb)
		df = (qa + qb) * (qa + qb) / (qa*qa/(na-1) + qb*qb/(nb-1))
	}
	return tResult(ma-mb, se, df, mu, alt, level)
}

// WilsonCI is the Wilson score interval for k successes in n trials.
func WilsonCI(k, n, level float64) (lo, hi float64) {
	z := NormPPF(1 - (1-level)/2)
	p := k / n
	z2 := z * z
	den := 1 + z2/n
	c := (p + z2/(2*n)) / den
	h := z * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / den
	return math.Max(0, c-h), math.Min(1, c+h)
}

// Regression is an ordinary least-squares fit.
type Regression struct {
	Coef, StdErr, T, P []float64
	CI                 [][2]float64
	R2, AdjR2          float64
	// Sigma is the residual standard error sqrt(SSR / DF).
	Sigma, SSR        float64
	DF                int
	F, FP             float64 // overall F test against the intercept-only (or zero) model
	DF1               int
	Residuals, Fitted []float64
	Cov               [][]float64
}

// ErrNoDOF is returned by OLS when there are no more rows than terms.
var ErrNoDOF = errors.New("no residual degrees of freedom")

// OLS fits y = X b by QR. intercept says whether X has a constant column
// (for R² and the F test). It returns ErrSingular and the first column
// that depends on the earlier ones when X is rank-deficient.
func OLS(x [][]float64, y []float64, intercept bool, level float64) (*Regression, int, error) {
	n, p := len(x), len(x[0])
	if n <= p {
		return nil, -1, ErrNoDOF
	}
	q := FactorQR(x)
	if j := q.Deficient(); j >= 0 {
		return nil, j, ErrSingular
	}
	b, err := q.Solve(y)
	if err != nil {
		return nil, -1, err
	}
	r := &Regression{Coef: b, DF: n - p}
	r.Fitted = make([]float64, n)
	r.Residuals = make([]float64, n)
	for i := range n {
		s := 0.0
		for j := range p {
			s += x[i][j] * b[j]
		}
		r.Fitted[i] = s
		r.Residuals[i] = y[i] - s
		r.SSR += r.Residuals[i] * r.Residuals[i]
	}
	var sst float64
	if intercept {
		m, _ := MeanVar(y)
		for _, v := range y {
			sst += (v - m) * (v - m)
		}
	} else {
		for _, v := range y {
			sst += v * v
		}
	}
	df := float64(r.DF)
	s2 := r.SSR / df
	r.Sigma = math.Sqrt(s2)
	r.R2 = 1 - r.SSR/sst
	k := p
	if intercept {
		k = p - 1
	}
	r.DF1 = k
	dfTot := float64(n)
	if intercept {
		dfTot = float64(n - 1)
	}
	r.AdjR2 = 1 - (1-r.R2)*dfTot/df
	if k > 0 {
		r.F = ((sst - r.SSR) / float64(k)) / s2
		r.FP = F{float64(k), df}.SF(r.F)
	} else {
		r.F, r.FP = math.NaN(), math.NaN()
	}
	inv := q.RtRInverse()
	r.Cov = make([][]float64, p)
	td := StudentT{df}
	qt := td.PPF(1 - (1-level)/2)
	for j := range p {
		r.Cov[j] = make([]float64, p)
		for l := range p {
			r.Cov[j][l] = inv[j][l] * s2
		}
		se := math.Sqrt(r.Cov[j][j])
		r.StdErr = append(r.StdErr, se)
		t := b[j] / se
		r.T = append(r.T, t)
		r.P = append(r.P, math.Min(1, 2*td.SF(math.Abs(t))))
		r.CI = append(r.CI, [2]float64{b[j] - qt*se, b[j] + qt*se})
	}
	return r, -1, nil
}
