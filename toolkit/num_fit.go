package toolkit

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/iodesystems/mcpshell/numeric"
	. "github.com/iodesystems/mcpshell/runtime"
)

// numStatus reports a run that did not converge as a note.
func numStatus(sh *Shell, c rfail, st numeric.Status, iters int, how string) {
	switch st {
	case numeric.MaxIterations:
		sh.Note(fmt.Sprintf("%s: not converged: stopped at the iteration limit (%d). The result is the last point, not an optimum. %s", string(c), iters, how))
	case numeric.Stopped:
		lm := sh.Limits()
		sh.Note(fmt.Sprintf("%s: not converged: the eval time limit stopped it after %d iterations. The result is the last point. Raise the limit (%s) or simplify", string(c), iters, lm.RaiseHint("timeout", lm.TimeoutMs*2)))
	case numeric.Stalled:
		sh.Note(fmt.Sprintf("%s: stalled: no step improved the result, although the convergence tests were not met. Check the model for flat regions or discontinuities, or try another start", string(c)))
	}
}

// numContext bounds a run by 90% of the eval's remaining time, so the
// script can still use a stopped result, and by an eval cancel.
func numContext(sh *Shell, c rfail) (context.Context, func()) {
	ctx, done := routeContext(sh, c, 0)
	return ctx, done
}

// callNum calls a user function and reads a number result (NaN allowed:
// the methods treat it as a bad point).
func callNum(c rfail, fn *FuncVal, args []Value, what func() string) float64 {
	v := Call(fn, args)
	n, ok := v.(*NumberVal)
	if !ok {
		c.fail("the function must return a number; it returned %s for %s", describe(v), what())
	}
	return n.V
}

// paramShape is a parameter vector as the script gives it: a number, an
// array or an object with named entries.
type paramShape struct {
	kind  string // "number", "array", "object"
	names []string
}

func (c rfail) params(what string, v Value, allowNumber bool) ([]float64, paramShape) {
	switch x := v.(type) {
	case *NumberVal:
		if allowNumber {
			return []float64{c.nfloat(what, x)}, paramShape{kind: "number"}
		}
	case *ArrayVal:
		return c.vector(what, x), paramShape{kind: "array"}
	case *ObjectVal:
		if x.Len() == 0 {
			c.fail("%s is empty", what)
		}
		ps := paramShape{kind: "object", names: append([]string(nil), x.Keys()...)}
		out := make([]float64, len(ps.names))
		for i, k := range ps.names {
			e, _ := x.Get(k)
			out[i] = c.nfloat(what+"."+k, e)
		}
		return out, ps
	}
	want := "an array of numbers or an object {name: number}"
	if allowNumber {
		want = "a number, " + want
	}
	c.fail("%s must be %s, got %s", what, want, describe(v))
	return nil, paramShape{}
}

func (s paramShape) value(p []float64) Value {
	switch s.kind {
	case "number":
		return Num(p[0])
	case "object":
		o := NewObject()
		for i, k := range s.names {
			o.Set(k, Num(p[i]))
		}
		return o
	}
	return vecVal(p)
}

// valueOrNull is value(p), with null entries where p is NaN.
func (s paramShape) valueOrNull(p []float64) Value {
	v := s.value(p)
	null := func(x float64, e Value) Value {
		if math.IsNaN(x) {
			return Null
		}
		return e
	}
	switch x := v.(type) {
	case *ArrayVal:
		for i := range x.Elements {
			x.Elements[i] = null(p[i], x.Elements[i])
		}
	case *ObjectVal:
		for i, k := range s.names {
			e, _ := x.Get(k)
			x.Set(k, null(p[i], e))
		}
	case *NumberVal:
		return null(p[0], v)
	}
	return v
}

func (s paramShape) name(i int) string {
	switch s.kind {
	case "object":
		return s.names[i]
	case "number":
		return "x"
	}
	return fmt.Sprintf("p[%d]", i)
}

// bounds reads bounds in the shape of the parameters: [lo, hi] for a
// number, [[lo, hi], ...] for an array, {name: [lo, hi]} for an object.
// null in a pair = no limit.
func (c rfail) bounds(v Value, s paramShape, n int) (lo, hi []float64) {
	lo, hi = make([]float64, n), make([]float64, n)
	for i := range n {
		lo[i], hi[i] = math.Inf(-1), math.Inf(1)
	}
	pair := func(what string, v Value, i int) {
		a, ok := v.(*ArrayVal)
		if !ok || len(a.Elements) != 2 {
			c.fail("%s must be [lo, hi] (null = no limit), got %s", what, describe(v))
		}
		if a.Elements[0] != Null {
			lo[i] = c.nfloatInf(what+" lo", a.Elements[0])
		}
		if a.Elements[1] != Null {
			hi[i] = c.nfloatInf(what+" hi", a.Elements[1])
		}
		if lo[i] > hi[i] {
			c.fail("%s: lo %v is above hi %v", what, lo[i], hi[i])
		}
	}
	switch s.kind {
	case "number":
		pair("bounds", v, 0)
	case "array":
		a, ok := v.(*ArrayVal)
		if !ok || len(a.Elements) != n {
			c.fail("bounds must be an array of %d pairs [lo, hi], one per parameter, got %s", n, describe(v))
		}
		for i, e := range a.Elements {
			if e != Null {
				pair(fmt.Sprintf("bounds[%d]", i), e, i)
			}
		}
	case "object":
		o, ok := v.(*ObjectVal)
		if !ok {
			c.fail("bounds must be an object {name: [lo, hi]} like the parameters, got %s", describe(v))
		}
		for _, k := range o.Keys() {
			i := -1
			for j, nm := range s.names {
				if nm == k {
					i = j
				}
			}
			if i < 0 {
				c.fail("bounds: %q is not a parameter. Parameters: %s", k, strings.Join(s.names, ", "))
			}
			e, _ := o.Get(k)
			pair("bounds."+k, e, i)
		}
	}
	return lo, hi
}

// --- regression ------------------------------------------------------------------

func numRegress(args []Value) Value {
	const c = rfail("Num.regress")
	c.args(args, 2, 2, `(rows, {y: "col", x: ["col", ...]})`)
	rows, ok := args[0].(*ArrayVal)
	if !ok {
		c.fail("rows must be an array of objects (query rows), got %s", describe(args[0]))
	}
	o := c.options(args[1], "spec", []string{"y", "x", "intercept", "level"})
	yv, ok := o.get("y")
	ys, isStr := yv.(*StringVal)
	if !ok || !isStr {
		c.fail(`spec.y must be the response column name, e.g. {y: "sales", x: ["price"]}`)
	}
	xv, _ := o.get("x")
	var xcols []string
	switch x := xv.(type) {
	case *StringVal:
		xcols = []string{x.V}
	case *ArrayVal:
		for i, e := range x.Elements {
			s, ok := e.(*StringVal)
			if !ok {
				c.fail("spec.x[%d] must be a column name, got %s", i, describe(e))
			}
			xcols = append(xcols, s.V)
		}
	default:
		c.fail(`spec.x must be the predictor column names, e.g. {y: "sales", x: ["price", "ads"]}`)
	}
	intercept := o.boolean("intercept", true)
	level := numLevel(c, o)
	var terms []string
	if intercept {
		if contains(xcols, "intercept") {
			c.fail(`a column is named "intercept", which is the name of the constant term; rename it`)
		}
		terms = append(terms, "intercept")
	}
	terms = append(terms, xcols...)
	if len(terms) == 0 {
		c.fail("no terms: give x columns, or intercept: true")
	}
	var X [][]float64
	var y []float64
	var skipped []Value
	for i, r := range rows.Elements {
		ro, ok := r.(*ObjectVal)
		if !ok {
			c.fail("rows[%d] must be an object, got %s", i, describe(r))
		}
		read := func(col string) (float64, bool) {
			v, has := ro.Get(col)
			if !has {
				keys := ro.Keys()
				hint := ""
				if m := nearKey(col, keys); m != "" {
					hint = fmt.Sprintf(" (did you mean %q?)", m)
				}
				c.fail("rows[%d] has no column %q%s. Columns: %s", i, col, hint, strings.Join(keys, ", "))
			}
			if v == Null {
				return 0, false
			}
			n, isNum := v.(*NumberVal)
			if !isNum {
				c.fail("rows[%d].%s is %s; regress takes numbers. For a category make 0/1 columns: rows.map(r => ({...r, %s_b: r.%s == \"b\" ? 1 : 0}))", i, col, describe(v), col, col)
			}
			if !finiteF(n.V) {
				c.fail("rows[%d].%s is %s; it must be finite", i, col, n.Display())
			}
			return n.V, true
		}
		yi, ok := read(ys.V)
		row := make([]float64, 0, len(terms))
		if intercept {
			row = append(row, 1)
		}
		for _, col := range xcols {
			x, okx := read(col)
			ok = ok && okx
			row = append(row, x)
		}
		if !ok {
			skipped = append(skipped, Num(float64(i)))
			continue
		}
		X = append(X, row)
		y = append(y, yi)
	}
	if len(X) <= len(terms) {
		c.fail("%d usable rows for %d terms: need more rows than terms (%d rows skipped for null values)", len(X), len(terms), len(skipped))
	}
	reg, dep, err := numeric.OLS(X, y, intercept, level)
	if err != nil {
		if dep >= 0 {
			c.fail("%q is a linear combination of the terms before it (%s), so the coefficients are not unique. Drop it, or a column it duplicates", terms[dep], strings.Join(terms[:dep], ", "))
		}
		c.fail("%v", err)
	}
	coefs := make([]Value, len(terms))
	coef := NewObject()
	for j, t := range terms {
		r := NewObject()
		r.Set("term", Str(t))
		r.Set("estimate", Num(reg.Coef[j]))
		r.Set("stdErr", Num(reg.StdErr[j]))
		r.Set("t", Num(reg.T[j]))
		r.Set("p", Num(reg.P[j]))
		r.Set("lo", Num(reg.CI[j][0]))
		r.Set("hi", Num(reg.CI[j][1]))
		coefs[j] = r
		coef.Set(t, Num(reg.Coef[j]))
	}
	out := NewObject()
	out.Set("coefficients", &ArrayVal{Elements: coefs})
	out.Set("coef", coef)
	out.Set("r2", Num(reg.R2))
	out.Set("adjR2", Num(reg.AdjR2))
	out.Set("sigma", Num(reg.Sigma))
	out.Set("f", Num(reg.F))
	out.Set("fP", Num(reg.FP))
	out.Set("df", Num(float64(reg.DF)))
	out.Set("n", Num(float64(len(X))))
	out.Set("level", Num(level))
	out.Set("residuals", vecVal(reg.Residuals))
	out.Set("fitted", vecVal(reg.Fitted))
	out.Set("skipped", &ArrayVal{Elements: skipped})
	return out
}

// --- curve fitting -----------------------------------------------------------------

var (
	numFitKeys  = []string{"x", "y", "model", "p0", "peaks", "baseline", "bounds", "sigma", "maxIter", "tol"}
	numPeakKeys = []string{"shape", "center", "height", "width", "eta", "bounds"}
)

func numFit(sh *Shell, args []Value) Value {
	const c = rfail("Num.fit")
	c.args(args, 1, 1, "(spec)")
	o := c.options(args[0], "spec", numFitKeys)
	xv, okx := o.get("x")
	yv, oky := o.get("y")
	if !okx || !oky {
		c.fail("needs x and y: {x: [...], y: [...], model, p0} or {x, y, peaks}")
	}
	y := c.vector("y", yv)
	xa, ok := xv.(*ArrayVal)
	if !ok || len(xa.Elements) != len(y) {
		c.fail("x must be an array with one entry per y value (%d), got %s", len(y), describe(xv))
	}
	var w []float64
	if sv, ok := o.get("sigma"); ok {
		s := c.vector("sigma", sv)
		if len(s) != len(y) {
			c.fail("sigma must have one value per point (%d), got %d", len(y), len(s))
		}
		w = make([]float64, len(s))
		for i, v := range s {
			if !(v > 0) {
				c.fail("sigma[%d] must be > 0, got %v", i, v)
			}
			w[i] = 1 / v
		}
	}
	pr := numeric.FitProblem{Y: y, W: w}
	pr.MaxIter = o.integer("maxIter", 200, 1, 100000)
	if tv, ok := o.get("tol"); ok {
		t := c.nfloat("tol", tv)
		if !(t > 0 && t < 1) {
			c.fail("tol must be between 0 and 1, got %v", t)
		}
		pr.FTol, pr.XTol = t, t
	}
	_, hasModel := o.get("model")
	_, hasPeaks := o.get("peaks")
	switch {
	case hasModel && hasPeaks:
		c.fail("give model (with p0) or peaks, not both")
	case hasPeaks:
		return numFitPeaks(sh, c, o, xa, pr)
	case !hasModel:
		c.fail("needs model: (x, p) => y with p0, or peaks: [{shape, center, ...}]")
	}
	for _, k := range []string{"baseline"} {
		if _, ok := o.get(k); ok {
			c.fail("%s applies to peaks; with a model, put it in the model", k)
		}
	}
	mv, _ := o.get("model")
	fn, ok := mv.(*FuncVal)
	if !ok {
		c.fail("model must be a function (x, p) => y, got %s", describe(mv))
	}
	p0v, ok := o.get("p0")
	if !ok {
		c.fail("model needs p0: the starting parameters, [1, 0.5] or {a: 1, k: 0.5}")
	}
	p0, shape := c.params("p0", p0v, false)
	pr.P0 = p0
	if bv, ok := o.get("bounds"); ok {
		pr.Lo, pr.Hi = c.bounds(bv, shape, len(p0))
		for j := range p0 {
			if p0[j] < pr.Lo[j] || p0[j] > pr.Hi[j] {
				c.fail("p0 %s = %v is outside its bounds [%v, %v]", shape.name(j), p0[j], pr.Lo[j], pr.Hi[j])
			}
		}
	}
	xs := xa.Elements
	pr.Model = func(p, out []float64) {
		pv := shape.value(p)
		for i, x := range xs {
			out[i] = callNum(c, fn, []Value{x, pv}, func() string { return fmt.Sprintf("x[%d] = %s", i, x.Inspect()) })
		}
	}
	res := numRunFit(sh, c, pr, shape)
	out := numFitResult(res, y, shape)
	return out
}

// numRunFit runs the fit and raises the errors that leave no result.
func numRunFit(sh *Shell, c rfail, pr numeric.FitProblem, shape paramShape) *numeric.FitResult {
	ctx, done := numContext(sh, c)
	defer done()
	res, err := numeric.Fit(ctx, pr)
	if err != nil {
		f := make([]float64, len(pr.Y))
		pr.Model(pr.P0, f)
		for i, v := range f {
			if !finiteF(v) {
				c.fail("the model is %v at point %d with the starting parameters %s; give p0 where the model is finite", v, i, shape.value(pr.P0).Inspect())
			}
		}
		c.fail("%v", err)
	}
	if sh.Limits().Cancelled() {
		c.fail("Execution cancelled")
	}
	numStatus(sh, c, res.Status, res.Iterations, "Raise maxIter, or start closer (p0)")
	np := len(res.P)
	switch {
	case res.DOF <= 0:
		sh.Note(fmt.Sprintf("%s: %d points for %d parameters: no residual degrees of freedom, so stdErr and covariance are null", string(c), len(pr.Y), np))
	case res.Cov == nil:
		sh.Note(fmt.Sprintf("%s: %s is not identifiable (the model barely changes with it, or another parameter does the same job), so stdErr and covariance are null. Fix or remove it", string(c), shape.name(res.Dependent)))
	}
	var at []string
	for j, b := range res.AtBound {
		if b {
			at = append(at, shape.name(j))
		}
	}
	if len(at) > 0 && res.Cov != nil {
		sh.Note(fmt.Sprintf("%s: at a bound: %s. stdErr treats each parameter as free, so it is not reliable for these", string(c), strings.Join(at, ", ")))
	}
	return res
}

// numFitResult builds the common result keys.
func numFitResult(res *numeric.FitResult, y []float64, shape paramShape) *ObjectVal {
	np := len(res.P)
	out := NewObject()
	out.Set("status", Str(res.Status.String()))
	if res.Reason != "" {
		out.Set("reason", Str(res.Reason))
	}
	out.Set("params", shape.value(res.P))
	se := make([]float64, np)
	for j := range se {
		se[j] = math.NaN()
		if res.Cov != nil {
			se[j] = math.Sqrt(res.Cov[j*np+j])
		}
	}
	out.Set("stdErr", shape.valueOrNull(se))
	if res.Cov != nil {
		cov := make([][]float64, np)
		for a := range np {
			cov[a] = res.Cov[a*np : (a+1)*np]
		}
		out.Set("covariance", matVal(cov))
	} else {
		out.Set("covariance", Null)
	}
	numFitStats(out, res, y)
	return out
}

func numFitStats(out *ObjectVal, res *numeric.FitResult, y []float64) {
	n := len(y)
	resid := make([]float64, n)
	mean, sst, ss := 0.0, 0.0, 0.0
	for _, v := range y {
		mean += v
	}
	mean /= float64(n)
	for i, v := range y {
		resid[i] = v - res.Fitted[i]
		ss += resid[i] * resid[i]
		sst += (v - mean) * (v - mean)
	}
	out.Set("ssr", Num(res.SSR))
	out.Set("rmse", Num(math.Sqrt(ss/float64(n))))
	out.Set("r2", Num(1-ss/sst))
	out.Set("n", Num(float64(n)))
	out.Set("dof", Num(float64(res.DOF)))
	out.Set("iterations", Num(float64(res.Iterations)))
	out.Set("evaluations", Num(float64(res.Evals)))
	out.Set("fitted", vecVal(res.Fitted))
	out.Set("residuals", vecVal(resid))
}

var numBaselines = map[string]int{"none": 0, "constant": 1, "linear": 2, "quadratic": 3}

func numFitPeaks(sh *Shell, c rfail, o *ropts, xa *ArrayVal, pr numeric.FitProblem) Value {
	for _, k := range []string{"p0", "bounds"} {
		if _, ok := o.get(k); ok {
			c.fail("%s applies to model; with peaks, give guesses and bounds in each peak row", k)
		}
	}
	x := make([]float64, len(xa.Elements))
	for i, e := range xa.Elements {
		x[i] = c.nfloat(fmt.Sprintf("x[%d]", i), e)
	}
	y := pr.Y
	nb := numBaselines[o.str("baseline", "none", "none", "constant", "linear", "quadratic")]
	pv, _ := o.get("peaks")
	pa, ok := pv.(*ArrayVal)
	if !ok || len(pa.Elements) == 0 {
		c.fail("peaks must be a non-empty array of {shape, center, height?, width?}, got %s", describe(pv))
	}
	xmin, xmax := x[0], x[0]
	for _, v := range x {
		xmin, xmax = math.Min(xmin, v), math.Max(xmax, v)
	}
	ymin := y[0]
	for _, v := range y {
		ymin = math.Min(ymin, v)
	}
	// nearestY is y at the x nearest to t.
	nearestY := func(t float64) float64 {
		best := 0
		for i, v := range x {
			if math.Abs(v-t) < math.Abs(x[best]-t) {
				best = i
			}
		}
		return y[best]
	}
	var shapes []numeric.PeakShape
	var offs []int
	var p0, lo, hi []float64
	var names []string
	var shapeNames []string
	for _, s := range numeric.PeakShapes {
		shapeNames = append(shapeNames, s.Name)
	}
	for i, e := range pa.Elements {
		where := fmt.Sprintf("peaks[%d]", i)
		po := c.options(e, where, numPeakKeys)
		sv, ok := po.get("shape")
		ss, isStr := sv.(*StringVal)
		if !ok || !isStr {
			c.fail("%s needs shape: %s", where, quoteList(shapeNames))
		}
		shape, ok := numeric.FindPeakShape(strings.ToLower(ss.V))
		if !ok {
			c.fail("%s: unknown shape %q. Shapes: %s", where, ss.V, quoteList(shapeNames))
		}
		cv, ok := po.get("center")
		if !ok {
			c.fail("%s needs center: the peak position guess, in x units", where)
		}
		center := c.nfloat(where+" center", cv)
		height := nearestY(center)
		if nb > 0 {
			height -= ymin
		}
		if v, ok := po.get("height"); ok {
			height = c.nfloat(where+" height", v)
		}
		width := (xmax - xmin) / 50
		if v, ok := po.get("width"); ok {
			width = c.nfloat(where+" width", v)
		}
		if !(width > 0) {
			c.fail("%s width must be > 0 (full width at half maximum), got %v", where, width)
		}
		guess := []float64{center, height, width}
		plo := []float64{math.Inf(-1), math.Inf(-1), 1e-12 * math.Max(xmax-xmin, 1)}
		phi := []float64{math.Inf(1), math.Inf(1), math.Inf(1)}
		pn := []string{"center", "height", "width"}
		if shape.NParams == 4 {
			eta := 0.5
			if v, ok := po.get("eta"); ok {
				eta = c.nfloat(where+" eta", v)
				if eta < 0 || eta > 1 {
					c.fail("%s eta (Lorentzian fraction) must be from 0 to 1, got %v", where, eta)
				}
			}
			guess, plo, phi, pn = append(guess, eta), append(plo, 0), append(phi, 1), append(pn, "eta")
		} else if _, ok := po.get("eta"); ok {
			c.fail("%s: eta applies to shape \"pseudovoigt\"", where)
		}
		if bv, ok := po.get("bounds"); ok {
			bo := c.options(bv, where+" bounds", pn)
			for j, k := range pn {
				if v, ok := bo.get(k); ok {
					l, h := c.bounds(v, paramShape{kind: "number"}, 1)
					plo[j], phi[j] = math.Max(plo[j], l[0]), math.Min(phi[j], h[0])
					if guess[j] < plo[j] || guess[j] > phi[j] {
						c.fail("%s %s guess %v is outside its bounds [%v, %v]", where, k, guess[j], plo[j], phi[j])
					}
				}
			}
		}
		shapes = append(shapes, shape)
		offs = append(offs, len(p0))
		p0, lo, hi = append(p0, guess...), append(lo, plo...), append(hi, phi...)
		for _, k := range pn {
			names = append(names, fmt.Sprintf("peaks[%d].%s", i, k))
		}
	}
	// Baseline c0 + c1 x + c2 x^2: start at the lowest y, flat.
	boff := len(p0)
	for k := range nb {
		v := 0.0
		if k == 0 {
			v = ymin
		}
		p0, lo, hi = append(p0, v), append(lo, math.Inf(-1)), append(hi, math.Inf(1))
		names = append(names, fmt.Sprintf("baseline.c%d", k))
	}
	pr.P0, pr.Lo, pr.Hi = p0, lo, hi
	pr.Model = func(p, out []float64) {
		for i, xi := range x {
			s := 0.0
			for k, sh := range shapes {
				s += sh.At(xi, p[offs[k]:offs[k]+sh.NParams])
			}
			b, xp := 0.0, 1.0
			for k := range nb {
				b += p[boff+k] * xp
				xp *= xi
			}
			out[i] = s + b
		}
	}
	shape := paramShape{kind: "object", names: names}
	res := numRunFit(sh, c, pr, shape)
	np := len(res.P)
	cov := func(a, b int) float64 {
		if res.Cov == nil {
			return math.NaN()
		}
		return res.Cov[a*np+b]
	}
	peaks := make([]Value, len(shapes))
	for k, s := range shapes {
		off := offs[k]
		pk := res.P[off : off+s.NParams]
		row := NewObject()
		row.Set("shape", Str(s.Name))
		se := NewObject()
		for j, nm := range []string{"center", "height", "width", "eta"}[:s.NParams] {
			row.Set(nm, Num(pk[j]))
			se.Set(nm, numOrNull(math.Sqrt(cov(off+j, off+j))))
		}
		area := s.Area(pk)
		row.Set("area", Num(area))
		// Delta method: var(area) = g' C g, g = d area / d params.
		g := make([]float64, s.NParams)
		for j := range g {
			h := 1e-6 * math.Max(math.Abs(pk[j]), 1e-3)
			q := append([]float64(nil), pk...)
			q[j] += h
			g[j] = (s.Area(q) - area) / h
		}
		va := 0.0
		for a := range g {
			for b := range g {
				va += g[a] * g[b] * cov(off+a, off+b)
			}
		}
		se.Set("area", numOrNull(math.Sqrt(va)))
		row.Set("stdErr", se)
		peaks[k] = row
	}
	out := NewObject()
	out.Set("status", Str(res.Status.String()))
	if res.Reason != "" {
		out.Set("reason", Str(res.Reason))
	}
	out.Set("peaks", &ArrayVal{Elements: peaks})
	if nb > 0 {
		b, bse := NewObject(), NewObject()
		for k := range nb {
			b.Set(fmt.Sprintf("c%d", k), Num(res.P[boff+k]))
			bse.Set(fmt.Sprintf("c%d", k), numOrNull(math.Sqrt(cov(boff+k, boff+k))))
		}
		b.Set("stdErr", bse)
		out.Set("baseline", b)
	}
	numFitStats(out, res, y)
	return out
}

func numOrNull(x float64) Value {
	if math.IsNaN(x) {
		return Null
	}
	return Num(x)
}

// --- minimize, root, integrate, interp ------------------------------------------------

func numMinimize(sh *Shell, args []Value) Value {
	const c = rfail("Num.minimize")
	c.args(args, 2, 3, "(f, x0, opts?)")
	fn, ok := args[0].(*FuncVal)
	if !ok {
		c.fail("f must be a function of the parameters returning a number, got %s", describe(args[0]))
	}
	x0, shape := c.params("x0", args[1], true)
	o := &ropts{c, NewObject()}
	if v := argOpt(args, 2); v != nil && v != Null {
		o = c.options(v, "opts", []string{"bounds", "tol", "maxIter"})
	}
	var opt numeric.MinimizeOptions
	if bv, ok := o.get("bounds"); ok {
		opt.Lo, opt.Hi = c.bounds(bv, shape, len(x0))
	}
	if tv, ok := o.get("tol"); ok {
		t := c.nfloat("tol", tv)
		if !(t > 0 && t < 1) {
			c.fail("tol must be between 0 and 1, got %v", t)
		}
		opt.XTol, opt.FTol = t, t
	}
	opt.MaxIter = o.integer("maxIter", 200*len(x0), 1, 1000000)
	opt.MaxEvals = 2 * opt.MaxIter
	f := func(p []float64) float64 {
		return callNum(c, fn, []Value{shape.value(p)}, func() string { return shape.value(p).Inspect() })
	}
	if v := f(x0); math.IsNaN(v) || math.IsInf(v, 0) {
		c.fail("f(x0) is %v; start where f is finite", v)
	}
	ctx, done := numContext(sh, c)
	defer done()
	r := numeric.NelderMead(ctx, f, x0, opt)
	if sh.Limits().Cancelled() {
		c.fail("Execution cancelled")
	}
	numStatus(sh, c, r.Status, r.Iterations, "Raise maxIter, or start closer")
	out := NewObject()
	out.Set("status", Str(r.Status.String()))
	out.Set("x", shape.value(r.X))
	out.Set("fun", Num(r.F))
	out.Set("iterations", Num(float64(r.Iterations)))
	out.Set("evaluations", Num(float64(r.Evals)))
	return out
}

func numRoot(args []Value) Value {
	const c = rfail("Num.root")
	c.args(args, 2, 3, "(f, [a, b], opts?)")
	fn, ok := args[0].(*FuncVal)
	if !ok {
		c.fail("f must be a function x => number, got %s", describe(args[0]))
	}
	br, ok := args[1].(*ArrayVal)
	if !ok || len(br.Elements) != 2 {
		c.fail("bracket must be [a, b] with f(a) and f(b) of opposite sign, got %s", describe(args[1]))
	}
	a, b := c.nfloat("bracket a", br.Elements[0]), c.nfloat("bracket b", br.Elements[1])
	o := &ropts{c, NewObject()}
	if v := argOpt(args, 2); v != nil && v != Null {
		o = c.options(v, "opts", []string{"tol", "maxIter"})
	}
	xtol := 2e-12
	if tv, ok := o.get("tol"); ok {
		xtol = c.nfloat("tol", tv)
		if !(xtol > 0) {
			c.fail("tol must be > 0, got %v", xtol)
		}
	}
	maxIter := o.integer("maxIter", 100, 1, 100000)
	f := func(x float64) float64 {
		v := callNum(c, fn, []Value{Num(x)}, func() string { return fmt.Sprint("x = ", x) })
		if math.IsNaN(v) {
			c.fail("f(%v) is NaN; the bracket must be where f is defined", x)
		}
		return v
	}
	fa, fb := f(a), f(b)
	if fa != 0 && fb != 0 && math.Signbit(fa) == math.Signbit(fb) {
		c.fail("f(a) and f(b) have the same sign (f(%v) = %v, f(%v) = %v), so a root is not bracketed. Scan for a sign change first, e.g. range(0, 11).map(i => [i, f(i)])", a, fa, b, fb)
	}
	r := numeric.Brent(f, a, b, fa, fb, xtol, 4*0x1p-52, maxIter)
	out := NewObject()
	out.Set("status", Str(r.Status.String()))
	out.Set("x", Num(r.X))
	out.Set("fx", Num(r.FX))
	out.Set("iterations", Num(float64(r.Iterations)))
	return out
}

func numIntegrate(sh *Shell, args []Value) Value {
	const c = rfail("Num.integrate")
	if len(args) >= 1 {
		if _, isArr := args[0].(*ArrayVal); isArr {
			c.args(args, 2, 2, "(xs, ys) for sampled points")
			xs, ys := c.vector("xs", args[0]), c.vector("ys", args[1])
			if len(xs) != len(ys) || len(xs) < 2 {
				c.fail("xs and ys must have the same length, at least 2 (got %d and %d)", len(xs), len(ys))
			}
			for i := 1; i < len(xs); i++ {
				if xs[i] < xs[i-1] {
					c.fail("xs must be sorted ascending: xs[%d] = %v after xs[%d] = %v. Sort the points by x first", i, xs[i], i-1, xs[i-1])
				}
			}
			return Num(numeric.Trapezoid(xs, ys))
		}
	}
	c.args(args, 3, 4, "(f, a, b, opts?) or (xs, ys)")
	fn, ok := args[0].(*FuncVal)
	if !ok {
		c.fail("f must be a function x => number (or give sampled points: Num.integrate(xs, ys)), got %s", describe(args[0]))
	}
	a, b := c.nfloatInf("a", args[1]), c.nfloatInf("b", args[2])
	o := &ropts{c, NewObject()}
	if v := argOpt(args, 3); v != nil && v != Null {
		o = c.options(v, "opts", []string{"tol", "limit"})
	}
	tol := 1.49e-8
	if tv, ok := o.get("tol"); ok {
		tol = c.nfloat("tol", tv)
		if !(tol > 0) {
			c.fail("tol must be > 0, got %v", tol)
		}
	}
	limit := o.integer("limit", 200, 1, 100000)
	var bad float64
	f := func(x float64) float64 {
		v := callNum(c, fn, []Value{Num(x)}, func() string { return fmt.Sprint("x = ", x) })
		if !finiteF(v) {
			bad = x
		}
		return v
	}
	ctx, done := numContext(sh, c)
	defer done()
	r := numeric.Integrate(ctx, f, a, b, tol, tol, limit)
	if sh.Limits().Cancelled() {
		c.fail("Execution cancelled")
	}
	if r.NonFinite {
		c.fail("f(%v) is not finite; the integral has a singularity there. Split the range at it, or integrate a transformed function", bad)
	}
	switch r.Status {
	case numeric.MaxIterations:
		sh.Note(fmt.Sprintf("%s: not converged: %d segments (limit) and the error estimate %.3g is above the tolerance. The value may be wrong: an oscillating or singular integrand needs a split range or opts.limit", string(c), r.Segments, r.Error))
	case numeric.Stopped:
		numStatus(sh, c, r.Status, r.Segments, "")
	}
	out := NewObject()
	out.Set("status", Str(r.Status.String()))
	out.Set("value", Num(r.Value))
	out.Set("error", Num(r.Error))
	out.Set("evaluations", Num(float64(r.Evals)))
	return out
}

func numInterp(args []Value) Value {
	const c = rfail("Num.interp")
	c.args(args, 3, 4, "(xs, ys, x, opts?)")
	xs, ys := c.vector("xs", args[0]), c.vector("ys", args[1])
	if len(xs) != len(ys) {
		c.fail("xs has %d values and ys has %d; they must match", len(xs), len(ys))
	}
	if len(xs) < 2 {
		c.fail("needs at least 2 points")
	}
	for i := 1; i < len(xs); i++ {
		if !(xs[i] > xs[i-1]) {
			c.fail("xs must be strictly increasing: xs[%d] = %v after xs[%d] = %v. Sort the points by x and merge equal x first", i, xs[i], i-1, xs[i-1])
		}
	}
	o := &ropts{c, NewObject()}
	if v := argOpt(args, 3); v != nil && v != Null {
		o = c.options(v, "opts", []string{"method", "extrapolate"})
	}
	var p *numeric.Interp
	if o.str("method", "linear", "linear", "cubic") == "cubic" {
		p = numeric.NewSpline(xs, ys)
	} else {
		p = numeric.NewLinear(xs, ys)
	}
	extra := o.boolean("extrapolate", false)
	at := func(what string, v Value) Value {
		x := c.nfloat(what, v)
		if !extra && (x < xs[0] || x > xs[len(xs)-1]) {
			c.fail("%s = %v is outside the data range [%v, %v]. Pass {extrapolate: true} to extend the end segments", what, x, xs[0], xs[len(xs)-1])
		}
		return Num(p.At(x))
	}
	if a, ok := args[2].(*ArrayVal); ok {
		out := make([]Value, len(a.Elements))
		for i, e := range a.Elements {
			out[i] = at(fmt.Sprintf("x[%d]", i), e)
		}
		return &ArrayVal{Elements: out}
	}
	return at("x", args[2])
}
