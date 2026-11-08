package numeric

import (
	"container/heap"
	"context"
	"math"
	"sort"
)

// Interp interpolates through (xs[i], ys[i]); xs strictly increasing, at
// least 2 points.
type Interp struct {
	xs, ys []float64
	// m are the second derivatives of a natural cubic spline (nil = linear).
	m []float64
}

// NewLinear is piecewise-linear interpolation.
func NewLinear(xs, ys []float64) *Interp { return &Interp{xs: xs, ys: ys} }

// NewSpline is the natural cubic spline (second derivative 0 at both ends,
// scipy CubicSpline bc_type="natural"). Two points give a line.
func NewSpline(xs, ys []float64) *Interp {
	n := len(xs)
	m := make([]float64, n)
	if n > 2 {
		// Tridiagonal system for the interior second derivatives.
		a := make([]float64, n)
		b := make([]float64, n)
		c := make([]float64, n)
		d := make([]float64, n)
		for i := 1; i < n-1; i++ {
			h0, h1 := xs[i]-xs[i-1], xs[i+1]-xs[i]
			a[i], b[i], c[i] = h0, 2*(h0+h1), h1
			d[i] = 6 * ((ys[i+1]-ys[i])/h1 - (ys[i]-ys[i-1])/h0)
		}
		for i := 2; i < n-1; i++ {
			w := a[i] / b[i-1]
			b[i] -= w * c[i-1]
			d[i] -= w * d[i-1]
		}
		for i := n - 2; i >= 1; i-- {
			m[i] = (d[i] - c[i]*m[i+1]) / b[i]
		}
	}
	return &Interp{xs: xs, ys: ys, m: m}
}

// At evaluates at x. Outside [xs[0], xs[n-1]] it extends the end
// segment's line or cubic, as numpy/scipy extrapolate (the caller decides
// whether that is allowed).
func (p *Interp) At(x float64) float64 {
	xs, ys := p.xs, p.ys
	n := len(xs)
	i := sort.SearchFloat64s(xs, x) - 1
	if i < 0 {
		i = 0
	}
	if i > n-2 {
		i = n - 2
	}
	h := xs[i+1] - xs[i]
	if p.m == nil {
		t := (x - xs[i]) / h
		return ys[i] + t*(ys[i+1]-ys[i])
	}
	a := (xs[i+1] - x) / h
	b := (x - xs[i]) / h
	return a*ys[i] + b*ys[i+1] + ((a*a*a-a)*p.m[i]+(b*b*b-b)*p.m[i+1])*h*h/6
}

// Trapezoid integrates sampled points by the trapezoid rule.
func Trapezoid(xs, ys []float64) float64 {
	s := 0.0
	for i := 1; i < len(xs); i++ {
		s += (xs[i] - xs[i-1]) * (ys[i] + ys[i-1]) / 2
	}
	return s
}

// Gauss-Kronrod 7-15 nodes and weights (QUADPACK qk15).
var (
	gkX = [8]float64{0.991455371120812639206854697526329, 0.949107912342758524526189684047851,
		0.864864423359769072789712788640926, 0.741531185599394439863864773280788,
		0.586087235467691130294144845693013, 0.405845151377397166906606412076961,
		0.207784955007898467600689403773245, 0}
	gkW = [8]float64{0.022935322010529224963732008058970, 0.063092092629978553290700663189204,
		0.104790010322250183839876322541518, 0.140653259715525918745189590510238,
		0.169004726639267902826583426598550, 0.190350578064785409913256402421014,
		0.204432940075298892414161999234649, 0.209482141084727828012999174891714}
	gW = [4]float64{0.129484966168869693270611432679082, 0.279705391489276667901467771423780,
		0.381830050505118944950369775488975, 0.417959183673469387755102040816327}
)

type gkSeg struct {
	a, b, val, err float64
}

func gk15(f func(float64) float64, a, b float64) gkSeg {
	c, h := (a+b)/2, (b-a)/2
	fc := f(c)
	k := fc * gkW[7]
	g := fc * gW[3]
	for j := range 7 {
		dx := h * gkX[j]
		f1, f2 := f(c-dx), f(c+dx)
		k += gkW[j] * (f1 + f2)
		if j%2 == 1 {
			g += gW[j/2] * (f1 + f2)
		}
	}
	return gkSeg{a, b, k * h, math.Abs((k - g) * h)}
}

type segHeap []gkSeg

func (s segHeap) Len() int           { return len(s) }
func (s segHeap) Less(i, j int) bool { return s[i].err > s[j].err }
func (s segHeap) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s *segHeap) Push(x any)        { *s = append(*s, x.(gkSeg)) }
func (s *segHeap) Pop() any {
	old := *s
	x := old[len(old)-1]
	*s = old[:len(old)-1]
	return x
}

// IntegrateResult is the end of an adaptive integration.
type IntegrateResult struct {
	Value, Error float64
	Evals        int
	Segments     int
	Status       Status
	// NonFinite: f returned NaN or Inf at a node.
	NonFinite bool
}

// Integrate integrates f over [a, b] (either end may be infinite) by
// globally adaptive Gauss-Kronrod 7-15 bisection, as QUADPACK qag, without
// the epsilon extrapolation of qags. It stops when the error estimate is
// at most max(epsAbs, epsRel*|value|), or at limit segments.
func Integrate(ctx context.Context, f func(float64) float64, a, b, epsAbs, epsRel float64, limit int) IntegrateResult {
	sign := 1.0
	if a > b {
		a, b, sign = b, a, -1
	}
	evals := 0
	nonFinite := false
	g := func(x float64) float64 {
		evals++
		v := f(x)
		if !finite(v) {
			nonFinite = true
		}
		return v
	}
	// Infinite ends: map onto a finite interval.
	h := g
	switch {
	case math.IsInf(a, -1) && math.IsInf(b, 1):
		h = func(t float64) float64 { // x = t / (1 - t^2), t in (-1, 1)
			d := 1 - t*t
			return g(t/d) * (1 + t*t) / (d * d)
		}
		a, b = -1, 1
	case math.IsInf(b, 1):
		lo := a
		h = func(t float64) float64 { // x = lo + t / (1 - t), t in [0, 1)
			d := 1 - t
			return g(lo+t/d) / (d * d)
		}
		a, b = 0, 1
	case math.IsInf(a, -1):
		hi := b
		h = func(t float64) float64 { // x = hi - (1 - t) / t, t in (0, 1]
			return g(hi-(1-t)/t) / (t * t)
		}
		a, b = 0, 1
	}
	if a == b {
		return IntegrateResult{Status: Converged}
	}
	segs := &segHeap{gk15(h, a, b)}
	total, errSum := (*segs)[0].val, (*segs)[0].err
	status := MaxIterations
	for {
		if errSum <= math.Max(epsAbs, epsRel*math.Abs(total)) || nonFinite {
			status = Converged
			break
		}
		if segs.Len() >= limit {
			break
		}
		if ctx.Err() != nil {
			status = Stopped
			break
		}
		s := heap.Pop(segs).(gkSeg)
		m := (s.a + s.b) / 2
		if m <= s.a || m >= s.b { // cannot split further
			heap.Push(segs, s)
			break
		}
		l, r := gk15(h, s.a, m), gk15(h, m, s.b)
		heap.Push(segs, l)
		heap.Push(segs, r)
		total, errSum = 0, 0
		for _, x := range *segs {
			total += x.val
			errSum += x.err
		}
	}
	if nonFinite {
		status = MaxIterations
	}
	return IntegrateResult{Value: sign * total, Error: errSum, Evals: evals, Segments: segs.Len(), Status: status, NonFinite: nonFinite}
}
