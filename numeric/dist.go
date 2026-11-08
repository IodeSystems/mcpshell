package numeric

import (
	"fmt"
	"math"
)

// Dist is a probability distribution. For a discrete distribution PDF is
// the probability mass function (0 off the integers), and CDF(x) is
// CDF(floor x).
type Dist interface {
	PDF(x float64) float64
	CDF(x float64) float64
	// SF is 1 - CDF, computed so that small upper tails keep their digits.
	SF(x float64) float64
	// PPF is the quantile: the x with CDF(x) = p; for a discrete
	// distribution the smallest k with CDF(k) >= p.
	PPF(p float64) float64
	Mean() float64
	Var() float64
	Discrete() bool
	// Support is the closed range outside which PDF is 0.
	Support() (lo, hi float64)
}

// DistParam describes one parameter of a named distribution.
type DistParam struct {
	Name string
	// Default is used when the parameter is not given; NaN = required.
	Default float64
	// Integer parameters must be whole numbers.
	Integer bool
	// Check returns "" or what the value must be.
	Check func(v float64) string
}

func positive(v float64) string {
	if v > 0 && !math.IsInf(v, 0) {
		return ""
	}
	return "a number > 0"
}

func probability(v float64) string {
	if v >= 0 && v <= 1 {
		return ""
	}
	return "a probability from 0 to 1"
}

func nonneg(v float64) string {
	if v >= 0 && !math.IsInf(v, 0) {
		return ""
	}
	return "a number >= 0"
}

func anyFinite(v float64) string {
	if finite(v) {
		return ""
	}
	return "a finite number"
}

// DistSpec is a named distribution and its parameters.
type DistSpec struct {
	Name   string
	Params []DistParam
	Make   func(p []float64) (Dist, error)
}

var nan = math.NaN()

// Distributions lists the named distributions, in the order the guide
// shows them.
var Distributions = []DistSpec{
	{"normal", []DistParam{{"mu", 0, false, anyFinite}, {"sigma", 1, false, positive}},
		func(p []float64) (Dist, error) { return Normal{p[0], p[1]}, nil }},
	{"t", []DistParam{{"df", nan, false, positive}},
		func(p []float64) (Dist, error) { return StudentT{p[0]}, nil }},
	{"chi2", []DistParam{{"df", nan, false, positive}},
		func(p []float64) (Dist, error) { return Gamma{p[0] / 2, 2}, nil }},
	{"f", []DistParam{{"df1", nan, false, positive}, {"df2", nan, false, positive}},
		func(p []float64) (Dist, error) { return F{p[0], p[1]}, nil }},
	{"binomial", []DistParam{{"n", nan, true, nonneg}, {"p", nan, false, probability}},
		func(p []float64) (Dist, error) { return Binomial{p[0], p[1]}, nil }},
	{"poisson", []DistParam{{"lambda", nan, false, positive}},
		func(p []float64) (Dist, error) { return Poisson{p[0]}, nil }},
	{"exponential", []DistParam{{"rate", 1, false, positive}},
		func(p []float64) (Dist, error) { return Gamma{1, 1 / p[0]}, nil }},
	{"gamma", []DistParam{{"shape", nan, false, positive}, {"scale", 1, false, positive}},
		func(p []float64) (Dist, error) { return Gamma{p[0], p[1]}, nil }},
	{"beta", []DistParam{{"a", nan, false, positive}, {"b", nan, false, positive}},
		func(p []float64) (Dist, error) { return Beta{p[0], p[1]}, nil }},
	{"uniform", []DistParam{{"lo", 0, false, anyFinite}, {"hi", 1, false, anyFinite}},
		func(p []float64) (Dist, error) {
			if !(p[1] > p[0]) {
				return nil, fmt.Errorf("hi must be greater than lo")
			}
			return Uniform{p[0], p[1]}, nil
		}},
	{"lognormal", []DistParam{{"mu", 0, false, anyFinite}, {"sigma", 1, false, positive}},
		func(p []float64) (Dist, error) { return LogNormal{p[0], p[1]}, nil }},
}

// FindDist returns the named distribution's spec.
func FindDist(name string) (DistSpec, bool) {
	for _, d := range Distributions {
		if d.Name == name {
			return d, true
		}
	}
	return DistSpec{}, false
}

// Normal is N(Mu, Sigma^2).
type Normal struct{ Mu, Sigma float64 }

func (d Normal) PDF(x float64) float64 {
	z := (x - d.Mu) / d.Sigma
	return math.Exp(-z*z/2) / (d.Sigma * math.Sqrt(2*math.Pi))
}
func (d Normal) CDF(x float64) float64 { return math.Erfc(-(x-d.Mu)/(d.Sigma*math.Sqrt2)) / 2 }
func (d Normal) SF(x float64) float64  { return math.Erfc((x-d.Mu)/(d.Sigma*math.Sqrt2)) / 2 }
func (d Normal) PPF(p float64) float64 { return d.Mu + d.Sigma*NormPPF(p) }
func (d Normal) Mean() float64         { return d.Mu }
func (d Normal) Var() float64          { return d.Sigma * d.Sigma }
func (Normal) Discrete() bool          { return false }
func (Normal) Support() (float64, float64) {
	return math.Inf(-1), math.Inf(1)
}

// LogNormal: ln X ~ N(Mu, Sigma^2).
type LogNormal struct{ Mu, Sigma float64 }

func (d LogNormal) PDF(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return Normal{d.Mu, d.Sigma}.PDF(math.Log(x)) / x
}
func (d LogNormal) CDF(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return Normal{d.Mu, d.Sigma}.CDF(math.Log(x))
}
func (d LogNormal) SF(x float64) float64 {
	if x <= 0 {
		return 1
	}
	return Normal{d.Mu, d.Sigma}.SF(math.Log(x))
}
func (d LogNormal) PPF(p float64) float64 { return math.Exp(d.Mu + d.Sigma*NormPPF(p)) }
func (d LogNormal) Mean() float64         { return math.Exp(d.Mu + d.Sigma*d.Sigma/2) }
func (d LogNormal) Var() float64 {
	s2 := d.Sigma * d.Sigma
	return math.Expm1(s2) * math.Exp(2*d.Mu+s2)
}
func (LogNormal) Discrete() bool              { return false }
func (LogNormal) Support() (float64, float64) { return 0, math.Inf(1) }

// StudentT is Student's t with DF degrees of freedom.
type StudentT struct{ DF float64 }

func (d StudentT) PDF(x float64) float64 {
	v := d.DF
	return math.Exp(-lbeta(v/2, 0.5)-(v+1)/2*math.Log1p(x*x/v)) / math.Sqrt(v)
}

// tail is P(T > |x|).
func (d StudentT) tail(x float64) float64 {
	v := d.DF
	if math.IsInf(x, 0) {
		return 0
	}
	// I_{v/(v+x^2)}(v/2, 1/2) = P(|T| > |x|); for x^2 large relative to v,
	// v/(v+x^2) is computed without cancellation.
	i, _ := BetaI(v/(v+x*x), v/2, 0.5)
	return i / 2
}

func (d StudentT) CDF(x float64) float64 {
	if x < 0 {
		return d.tail(x)
	}
	return 1 - d.tail(x)
}
func (d StudentT) SF(x float64) float64 {
	if x > 0 {
		return d.tail(x)
	}
	return 1 - d.tail(x)
}
func (d StudentT) PPF(p float64) float64 {
	return invertContinuous(d, p, NormPPF(p))
}
func (StudentT) Mean() float64 { return 0 }
func (d StudentT) Var() float64 {
	switch {
	case d.DF > 2:
		return d.DF / (d.DF - 2)
	case d.DF > 1:
		return math.Inf(1)
	}
	return math.NaN()
}
func (StudentT) Discrete() bool              { return false }
func (StudentT) Support() (float64, float64) { return math.Inf(-1), math.Inf(1) }

// Gamma has shape K and scale Theta (chi2(df) = Gamma(df/2, 2),
// exponential(rate) = Gamma(1, 1/rate)).
type Gamma struct{ K, Theta float64 }

func (d Gamma) PDF(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		switch {
		case d.K < 1:
			return math.Inf(1)
		case d.K == 1:
			return 1 / d.Theta
		}
		return 0
	}
	lg, _ := math.Lgamma(d.K)
	z := x / d.Theta
	return math.Exp((d.K-1)*math.Log(z)-z-lg) / d.Theta
}
func (d Gamma) CDF(x float64) float64 {
	if x <= 0 {
		return 0
	}
	p, _ := GammaPQ(d.K, x/d.Theta)
	return p
}
func (d Gamma) SF(x float64) float64 {
	if x <= 0 {
		return 1
	}
	_, q := GammaPQ(d.K, x/d.Theta)
	return q
}
func (d Gamma) PPF(p float64) float64 {
	if d.K == 1 { // exponential: closed form
		if p >= 1 {
			return math.Inf(1)
		}
		return -math.Log1p(-p) * d.Theta
	}
	// Wilson-Hilferty start.
	z := NormPPF(p)
	k := d.K
	g := k * math.Pow(1-1/(9*k)+z/(3*math.Sqrt(k)), 3)
	if !(g > 0) {
		g = k * 0.1
	}
	return invertContinuous(d, p, g*d.Theta)
}
func (d Gamma) Mean() float64             { return d.K * d.Theta }
func (d Gamma) Var() float64              { return d.K * d.Theta * d.Theta }
func (Gamma) Discrete() bool              { return false }
func (Gamma) Support() (float64, float64) { return 0, math.Inf(1) }

// Beta is Beta(A, B) on [0, 1].
type Beta struct{ A, B float64 }

func (d Beta) PDF(x float64) float64 {
	if x < 0 || x > 1 {
		return 0
	}
	if (x == 0 && d.A < 1) || (x == 1 && d.B < 1) {
		return math.Inf(1)
	}
	if (x == 0 && d.A > 1) || (x == 1 && d.B > 1) {
		return 0
	}
	return math.Exp((d.A-1)*math.Log(x) + (d.B-1)*math.Log1p(-x) - lbeta(d.A, d.B))
}
func (d Beta) CDF(x float64) float64 { i, _ := BetaI(x, d.A, d.B); return i }
func (d Beta) SF(x float64) float64  { _, ic := BetaI(x, d.A, d.B); return ic }
func (d Beta) PPF(p float64) float64 {
	return invertContinuous(d, p, d.Mean())
}
func (d Beta) Mean() float64 { return d.A / (d.A + d.B) }
func (d Beta) Var() float64 {
	s := d.A + d.B
	return d.A * d.B / (s * s * (s + 1))
}
func (Beta) Discrete() bool              { return false }
func (Beta) Support() (float64, float64) { return 0, 1 }

// F is Fisher's F with D1 and D2 degrees of freedom.
type F struct{ D1, D2 float64 }

func (d F) PDF(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		switch {
		case d.D1 < 2:
			return math.Inf(1)
		case d.D1 == 2:
			return 1
		}
		return 0
	}
	a, b := d.D1/2, d.D2/2
	return math.Exp(a*math.Log(d.D1) + b*math.Log(d.D2) + (a-1)*math.Log(x) - (a+b)*math.Log(d.D2+d.D1*x) - lbeta(a, b))
}

// cdfSF uses whichever of z = d1 x/(d1 x + d2) and 1 - z = d2/(d1 x + d2)
// is below 1/2 as the beta argument, so neither rounds to 1.
func (d F) cdfSF(x float64) (cdf, sf float64) {
	if x <= 0 {
		return 0, 1
	}
	if math.IsInf(x, 1) {
		return 1, 0
	}
	den := d.D1*x + d.D2
	if z := d.D1 * x / den; z < 0.5 {
		return BetaI(z, d.D1/2, d.D2/2)
	}
	i, ic := BetaI(d.D2/den, d.D2/2, d.D1/2)
	return ic, i
}
func (d F) CDF(x float64) float64 { c, _ := d.cdfSF(x); return c }
func (d F) SF(x float64) float64  { _, s := d.cdfSF(x); return s }
func (d F) PPF(p float64) float64 {
	g := 1.0
	if d.D2 > 2 {
		g = d.D2 / (d.D2 - 2)
	}
	return invertContinuous(d, p, g)
}
func (d F) Mean() float64 {
	if d.D2 > 2 {
		return d.D2 / (d.D2 - 2)
	}
	return math.NaN()
}
func (d F) Var() float64 {
	if d.D2 > 4 {
		n, m := d.D1, d.D2
		return 2 * m * m * (n + m - 2) / (n * (m - 2) * (m - 2) * (m - 4))
	}
	return math.NaN()
}
func (F) Discrete() bool              { return false }
func (F) Support() (float64, float64) { return 0, math.Inf(1) }

// Uniform is U(Lo, Hi).
type Uniform struct{ Lo, Hi float64 }

func (d Uniform) PDF(x float64) float64 {
	if x < d.Lo || x > d.Hi {
		return 0
	}
	return 1 / (d.Hi - d.Lo)
}
func (d Uniform) CDF(x float64) float64 {
	return math.Min(1, math.Max(0, (x-d.Lo)/(d.Hi-d.Lo)))
}
func (d Uniform) SF(x float64) float64 {
	return math.Min(1, math.Max(0, (d.Hi-x)/(d.Hi-d.Lo)))
}
func (d Uniform) PPF(p float64) float64 { return d.Lo + p*(d.Hi-d.Lo) }
func (d Uniform) Mean() float64         { return (d.Lo + d.Hi) / 2 }
func (d Uniform) Var() float64 {
	w := d.Hi - d.Lo
	return w * w / 12
}
func (Uniform) Discrete() bool                { return false }
func (d Uniform) Support() (float64, float64) { return d.Lo, d.Hi }

// Binomial is the number of successes in N trials with probability P.
type Binomial struct{ N, P float64 }

func (d Binomial) PDF(x float64) float64 {
	if x != math.Floor(x) || x < 0 || x > d.N {
		return 0
	}
	switch d.P {
	case 0:
		if x == 0 {
			return 1
		}
		return 0
	case 1:
		if x == d.N {
			return 1
		}
		return 0
	}
	l1, _ := math.Lgamma(d.N + 1)
	l2, _ := math.Lgamma(x + 1)
	l3, _ := math.Lgamma(d.N - x + 1)
	return math.Exp(l1 - l2 - l3 + x*math.Log(d.P) + (d.N-x)*math.Log1p(-d.P))
}
func (d Binomial) CDF(x float64) float64 {
	k := math.Floor(x)
	switch {
	case k < 0:
		return 0
	case k >= d.N:
		return 1
	case d.P == 0:
		return 1
	case d.P == 1:
		return 0
	}
	// P(X <= k) = I_{1-p}(n-k, k+1)
	_, ic := BetaI(d.P, k+1, d.N-k)
	return ic
}
func (d Binomial) SF(x float64) float64 {
	k := math.Floor(x)
	switch {
	case k < 0:
		return 1
	case k >= d.N:
		return 0
	case d.P == 0:
		return 0
	case d.P == 1:
		return 1
	}
	i, _ := BetaI(d.P, k+1, d.N-k)
	return i
}
func (d Binomial) PPF(p float64) float64 {
	return invertDiscrete(d, p, d.Mean()+math.Sqrt(d.Var())*NormPPF(p), d.N)
}
func (d Binomial) Mean() float64               { return d.N * d.P }
func (d Binomial) Var() float64                { return d.N * d.P * (1 - d.P) }
func (Binomial) Discrete() bool                { return true }
func (d Binomial) Support() (float64, float64) { return 0, d.N }

// Poisson has mean Lambda.
type Poisson struct{ Lambda float64 }

func (d Poisson) PDF(x float64) float64 {
	if x != math.Floor(x) || x < 0 {
		return 0
	}
	lg, _ := math.Lgamma(x + 1)
	return math.Exp(x*math.Log(d.Lambda) - d.Lambda - lg)
}
func (d Poisson) CDF(x float64) float64 {
	k := math.Floor(x)
	if k < 0 {
		return 0
	}
	if math.IsInf(k, 1) {
		return 1
	}
	_, q := GammaPQ(k+1, d.Lambda)
	return q
}
func (d Poisson) SF(x float64) float64 {
	k := math.Floor(x)
	if k < 0 {
		return 1
	}
	if math.IsInf(k, 1) {
		return 0
	}
	p, _ := GammaPQ(k+1, d.Lambda)
	return p
}
func (d Poisson) PPF(p float64) float64 {
	return invertDiscrete(d, p, d.Lambda+math.Sqrt(d.Lambda)*NormPPF(p), math.Inf(1))
}
func (d Poisson) Mean() float64             { return d.Lambda }
func (d Poisson) Var() float64              { return d.Lambda }
func (Poisson) Discrete() bool              { return true }
func (Poisson) Support() (float64, float64) { return 0, math.Inf(1) }

// invertContinuous finds x with CDF(x) = p, starting the bracket at guess.
// Below the median it solves CDF(x) = p, above it SF(x) = 1 - p, so each
// tail keeps its digits.
func invertContinuous(d Dist, p, guess float64) float64 {
	lo, hi := d.Support()
	switch {
	case math.IsNaN(p) || p < 0 || p > 1:
		return math.NaN()
	case p == 0:
		return lo
	case p == 1:
		return hi
	}
	var g func(x float64) float64
	if p <= 0.5 {
		g = func(x float64) float64 { return d.CDF(x) - p }
	} else {
		q := 1 - p
		g = func(x float64) float64 { return q - d.SF(x) }
	}
	if !finite(guess) || guess <= lo || guess >= hi {
		guess = d.Mean()
		if !finite(guess) || guess <= lo || guess >= hi {
			guess = (lo + hi) / 2
			if !finite(guess) {
				guess = math.Max(lo, 0) + 1
			}
		}
	}
	// Expand a bracket [a, b] with g(a) <= 0 <= g(b).
	a, b := guess, guess
	ga := g(a)
	gb := ga
	step := math.Max(math.Abs(guess), 1) * 0.5
	for i := 0; ga > 0 && i < 2000; i++ {
		b, gb = a, ga
		a -= step
		step *= 2
		if a <= lo {
			a = lo
			ga = g(a)
			break
		}
		ga = g(a)
	}
	step = math.Max(math.Abs(guess), 1) * 0.5
	for i := 0; gb < 0 && i < 2000; i++ {
		a, ga = b, gb
		b += step
		step *= 2
		if b >= hi {
			b = hi
			gb = g(b)
			break
		}
		gb = g(b)
	}
	if ga == 0 {
		return a
	}
	if gb == 0 {
		return b
	}
	r := Brent(g, a, b, ga, gb, 0, 4*eps, 200)
	return r.X
}

// invertDiscrete returns the smallest integer k with CDF(k) >= p.
func invertDiscrete(d Dist, p, guess, top float64) float64 {
	switch {
	case math.IsNaN(p) || p < 0 || p > 1:
		return math.NaN()
	case p == 0:
		return -1 // scipy: ppf(0) = support lower bound - 1
	case p == 1:
		return top
	}
	k := math.Floor(guess)
	if !finite(k) || k < 0 {
		k = 0
	}
	if k > top {
		k = top
	}
	// cmp reports CDF(k) >= p, using SF above the median so that p near 1
	// is compared with the upper tail's digits.
	ge := func(k float64) bool {
		if p > 0.5 {
			return d.SF(k) <= 1-p
		}
		return d.CDF(k) >= p
	}
	if ge(k) {
		for k > 0 && ge(k-1) {
			k--
		}
		return k
	}
	for !ge(k) && k < top {
		k++
	}
	return k
}
