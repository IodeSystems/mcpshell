package toolkit

import (
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/iodesystems/mcpshell/numeric"
	. "github.com/iodesystems/mcpshell/runtime"
)

// AddNum registers the numeric toolkit as the loadable "num" (namespace
// Num). It has no outside effect, so it needs no flag.
func AddNum(sh *Shell) *Shell {
	return sh.AddLoadable(Loadable{
		Name:    "num",
		Summary: "float numerics: regression, curve/peak fit, minimize, root, integrate, linear algebra, distributions, t tests, seeded RNG (Num.*)",
		Guide:   numGuide,
		Install: func(sh *Shell) (io.Closer, error) {
			for _, c := range numCommands(sh) {
				sh.Register(c)
			}
			return nil, nil
		},
	})
}

const numGuide = `Num — floating-point numerics. load("num") first.
Results are float64 (about 16 significant digits), not exact numbers: compare with a
tolerance, round for reports. Matrices are arrays of rows: [[1, 2], [3, 4]].
status: "converged" = a stopping test was met (for fit and minimize: a local optimum, not
proven global). "max_iterations", "stopped" (time limit), "stalled" = not converged; a note says so.

TYPICAL: Regression from query rows
  let r = Num.regress(rows, {y: "sales", x: ["price", "ads"]})
  r.coefficients   // [{term: "intercept", estimate, stdErr, t, p, lo, hi}, {term: "price", ...}]
  r.coef.price; r.r2; r.adjR2; r.f; r.fP; r.residuals     // rows with null in y or x are skipped
TYPICAL: Curve fit, your model (Levenberg-Marquardt)
  let f = Num.fit({x: xs, y: ys, model: (x, p) => p.a * Math.exp(-p.k * x) + p.c, p0: {a: 1, k: 1, c: 0}})
  f.status; f.params; f.stdErr; f.covariance; f.r2; f.residuals    // p0: [..] gives arrays
TYPICAL: Spectrum peaks (built-in shapes, fast for thousands of points)
  let f = Num.fit({x: shift, y: counts, baseline: "linear", peaks: [
    {shape: "lorentzian", center: 520, height: 300, width: 8}, {shape: "gaussian", center: 610}]})
  f.peaks     // [{shape, center, height, width (FWHM), area, stdErr: {center, height, width, area}}]
TYPICAL: Distributions, tests, intervals
  Num.cdf("normal", 1.96); Num.sf("t", 2.3, {df: 9}); Num.ppf("chi2", 0.95, {df: 3})
  Num.ttest(a, b)       // Welch two-sample; {paired: true}, {equalVar: true}; Num.ttest(a, {mu: 5})
  Num.ci(xs)            // {mean, lo, hi, ...} t interval;  Num.ci({k: 17, n: 60}) Wilson interval
TYPICAL: Minimize, root, integral, interpolation
  Num.minimize(p => (p[0] - 1) ** 2 + (p[1] - 2) ** 2, [0, 0])   // {status, x, fun}; bounds: [[lo, hi], ...]
  Num.root(x => x ** 3 - 2 * x - 5, [2, 3]).x                     // f(a), f(b) must differ in sign
  Num.integrate(x => Math.exp(-x * x), -Infinity, Infinity).value // Num.integrate(xs, ys): trapezoid
  Num.interp(xs, ys, [2.5, 3], {method: "cubic"})                 // natural spline; default linear
TYPICAL: Linear algebra
  Num.solve([[2, 1], [1, 3]], [3, 5]); Num.inv(A); Num.det(A); Num.matmul(A, B); Num.lstsq(A, b)
  Num.eig(S)            // symmetric only: {values (ascending), vectors}; vectors[i] goes with values[i]
TYPICAL: Monte Carlo (Math.random has no seed)
  let g = Num.rng(42)   // same seed, same numbers
  g.normal(0, 1, 1000); g.uniform(); g.int(1, 6); g.choice(xs); g.sample(xs, 3); g.shuffle(xs)
  g.draw("poisson", {lambda: 3}, 100)

DISTRIBUTIONS: normal {mu: 0, sigma: 1}, t {df}, chi2 {df}, f {df1, df2}, binomial {n, p},
  poisson {lambda}, exponential {rate: 1}, gamma {shape, scale: 1}, beta {a, b},
  uniform {lo: 0, hi: 1}, lognormal {mu: 0, sigma: 1}. pdf of binomial and poisson is the pmf.
  sf(x) = 1 - cdf(x), with the digits of a small upper tail: use it for p-values.
TRAPS
  A model function runs once per point, p + 1 times per step: 601 points, 11 params took 13 s
  as a function and 25 ms as peaks. Use peaks for spectra; keep other models cheap.
  fit stdErr: scipy curve_fit convention (scaled by the residual variance; sigma: weights).
  interp outside the data range is an error unless extrapolate: true.`

// numCommands lists the Num.* commands.
func numCommands(sh *Shell) []*CommandDef {
	cmd := func(name, sig, desc, ex string, fn func(args []Value) Value) *CommandDef {
		return &CommandDef{Namespace: "Num", Name: name, Signature: sig, Description: desc, Examples: []string{ex}, Fn: fn}
	}
	return []*CommandDef{
		cmd("regress", "rows: [{...}], spec: {y, x: [cols], intercept?, level?}",
			"OLS: {coefficients: [{term, estimate, stdErr, t, p, lo, hi}], r2, adjR2, ...}",
			`Num.regress([{x: 1, y: 2.1}, {x: 2, y: 3.9}, {x: 3, y: 6.2}, {x: 4, y: 7.8}], {y: "y", x: ["x"]})`,
			func(a []Value) Value { return numRegress(a) }),
		cmd("fit", "spec: {x, y, model: (x, p) => y, p0, bounds?, sigma?, maxIter?} | {x, y, peaks: [{shape, center, height?, width?}], baseline?}",
			"least-squares fit (Levenberg-Marquardt) of a model or of peaks: {status, ...}",
			`Num.fit({x: [0, 1, 2, 3], y: [3, 1.9, 1.2, 0.8], model: (x, p) => p[0] * Math.exp(-p[1] * x), p0: [3, 0.5]})`,
			func(a []Value) Value { return numFit(sh, a) }),
		cmd("minimize", "f: (x) => number, x0: number | [numbers] | {name: number}, opts?: {bounds?, tol?, maxIter?}",
			"local minimum of f from x0 (Nelder-Mead): {status, x, fun, iterations}",
			`Num.minimize(p => (p[0] - 1) ** 2 + (p[1] + 2) ** 2, [0, 0])`,
			func(a []Value) Value { return numMinimize(sh, a) }),
		cmd("root", "f: (x) => number, bracket: [a, b], opts?: {tol?, maxIter?}",
			"root of f in [a, b] where f(a), f(b) differ in sign (Brent): {status, x, fx}",
			`Num.root(x => x * x - 2, [0, 2])`,
			func(a []Value) Value { return numRoot(a) }),
		cmd("integrate", "f: (x) => number, a: number, b: number, opts?: {tol?, limit?}",
			"integral of f on [a, b] (ends may be ±Infinity); (xs, ys): trapezoid rule",
			`Num.integrate(x => Math.sin(x), 0, Math.PI)`,
			func(a []Value) Value { return numIntegrate(sh, a) }),
		cmd("interp", "xs: [numbers], ys: [numbers], x: number | [numbers], opts?: {method?: \"linear\" | \"cubic\", extrapolate?}",
			"value at x of the linear or natural cubic interpolant through (xs, ys)",
			`Num.interp([0, 1, 2], [0, 10, 40], 1.5)`,
			func(a []Value) Value { return numInterp(a) }),
		cmd("pdf", "dist: string, x: number | [numbers], params?: {...}",
			"probability density (pmf for binomial, poisson)",
			`Num.pdf("normal", 0)`,
			func(a []Value) Value { return numDistFn("pdf", a) }),
		cmd("cdf", "dist: string, x: number | [numbers], params?: {...}",
			"cumulative probability P(X <= x)",
			`Num.cdf("t", 2.1, {df: 9})`,
			func(a []Value) Value { return numDistFn("cdf", a) }),
		cmd("sf", "dist: string, x: number | [numbers], params?: {...}",
			"upper tail P(X > x) = 1 - cdf, accurate when small (p-values)",
			`Num.sf("chi2", 11.3, {df: 3})`,
			func(a []Value) Value { return numDistFn("sf", a) }),
		cmd("ppf", "dist: string, p: number | [numbers], params?: {...}",
			"quantile: x with cdf(x) = p (discrete: smallest k with cdf(k) >= p)",
			`Num.ppf("normal", 0.975)`,
			func(a []Value) Value { return numDistFn("ppf", a) }),
		cmd("ttest", "a: [numbers], b?: [numbers], opts?: {paired?, equalVar?, mu?, alternative?, level?}",
			"t test: one-sample, Welch (default), pooled or paired: {t, df, p, lo, hi, ...}",
			`Num.ttest([5.1, 4.9, 5.6, 5.8, 6.0], [4.2, 4.8, 5.0, 4.4])`,
			func(a []Value) Value { return numTTest(a) }),
		cmd("ci", "data: [numbers] | {k, n}, opts?: {level?}",
			"interval for the mean (t) of numbers, or for a proportion {k, n} (Wilson)",
			`Num.ci([5.1, 4.9, 5.6, 5.8, 6.0])`,
			func(a []Value) Value { return numCI(a) }),
		cmd("solve", "A: [[numbers]], b: [numbers] | [[numbers]]",
			"x with A x = b for square A (LU, partial pivoting); singular A is an error",
			`Num.solve([[2, 1], [1, 3]], [3, 5])`,
			func(a []Value) Value { return numSolve(sh, a) }),
		cmd("lstsq", "A: [[numbers]], b: [numbers]",
			"least-squares x minimizing |A x - b| (QR): {x, residuals, ssr}",
			`Num.lstsq([[1, 1], [1, 2], [1, 3]], [1, 2, 2])`,
			func(a []Value) Value { return numLstsq(a) }),
		cmd("inv", "A: [[numbers]]", "inverse of a square matrix", `Num.inv([[2, 1], [1, 3]])`,
			func(a []Value) Value { return numInv(sh, a) }),
		cmd("det", "A: [[numbers]]", "determinant of a square matrix", `Num.det([[2, 1], [1, 3]])`,
			func(a []Value) Value { return numDet(a) }),
		cmd("eig", "A: [[numbers]]", "symmetric A: {values (ascending), vectors}; vectors[i] goes with values[i]",
			`Num.eig([[2, 1], [1, 2]])`,
			func(a []Value) Value { return numEig(a) }),
		cmd("matmul", "A: [[numbers]] | [numbers], B: [[numbers]] | [numbers]",
			"matrix product; a vector is a column on the right, a row on the left",
			`Num.matmul([[1, 2], [3, 4]], [1, 1])`,
			func(a []Value) Value { return numMatmul(a) }),
		cmd("transpose", "A: [[numbers]]", "transpose", `Num.transpose([[1, 2, 3], [4, 5, 6]])`,
			func(a []Value) Value { return numTranspose(a) }),
		cmd("rng", "seed?: integer",
			"seeded generator: .uniform .normal .int .choice .sample .shuffle .draw",
			`Num.rng(42).normal(0, 1, 3)`,
			func(a []Value) Value { return numRNG(a) }),
	}
}

// --- readers -----------------------------------------------------------------

// nfloat reads a finite number by its float view.
func (c rfail) nfloat(what string, v Value) float64 {
	n, ok := v.(*NumberVal)
	if !ok {
		c.fail("%s must be a number, got %s", what, describe(v))
	}
	if !finiteF(n.V) {
		c.fail("%s must be a finite number, got %s", what, n.Display())
	}
	return n.V
}

// nfloatInf reads a number that may be ±Infinity (not NaN).
func (c rfail) nfloatInf(what string, v Value) float64 {
	n, ok := v.(*NumberVal)
	if !ok || math.IsNaN(n.V) {
		c.fail("%s must be a number, got %s", what, describe(v))
	}
	return n.V
}

func finiteF(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// maxNumEntries caps one input array or matrix (8 bytes each in Go, plus the
// values the result returns).
const maxNumEntries = 4_000_000

func (c rfail) vector(what string, v Value) []float64 {
	a, ok := v.(*ArrayVal)
	if !ok {
		c.fail("%s must be an array of numbers, got %s", what, describe(v))
	}
	if len(a.Elements) == 0 {
		c.fail("%s is empty", what)
	}
	if len(a.Elements) > maxNumEntries {
		c.fail("%s has %d values; the limit is %d", what, len(a.Elements), maxNumEntries)
	}
	out := make([]float64, len(a.Elements))
	for i, e := range a.Elements {
		out[i] = c.nfloat(fmt.Sprintf("%s[%d]", what, i), e)
	}
	return out
}

// isMatrix reports whether v is an array whose first element is an array.
func isMatrix(v Value) bool {
	a, ok := v.(*ArrayVal)
	if !ok || len(a.Elements) == 0 {
		return false
	}
	_, ok = a.Elements[0].(*ArrayVal)
	return ok
}

func (c rfail) matrix(what string, v Value) [][]float64 {
	a, ok := v.(*ArrayVal)
	if !ok || !isMatrix(v) {
		c.fail("%s must be a matrix: an array of rows like [[1, 2], [3, 4]], got %s", what, describe(v))
	}
	cols := len(a.Elements[0].(*ArrayVal).Elements)
	if cols == 0 {
		c.fail("%s[0] is empty", what)
	}
	if len(a.Elements)*cols > maxNumEntries {
		c.fail("%s is %d x %d; the limit is %d entries", what, len(a.Elements), cols, maxNumEntries)
	}
	out := make([][]float64, len(a.Elements))
	for i, r := range a.Elements {
		ra, ok := r.(*ArrayVal)
		if !ok || len(ra.Elements) != cols {
			c.fail("%s[%d] must be an array of %d numbers (as %s[0]); rows must have the same length, got %s", what, i, cols, what, describe(r))
		}
		out[i] = make([]float64, cols)
		for j, e := range ra.Elements {
			out[i][j] = c.nfloat(fmt.Sprintf("%s[%d][%d]", what, i, j), e)
		}
	}
	return out
}

// maxSquare bounds the O(n^3) operations (1000^3 is about a second).
const maxSquare = 1000

func (c rfail) square(what string, v Value) [][]float64 {
	m := c.matrix(what, v)
	if len(m) != len(m[0]) {
		c.fail("%s must be square, got %d x %d", what, len(m), len(m[0]))
	}
	if len(m) > maxSquare {
		c.fail("%s is %d x %d; the limit is %d x %d", what, len(m), len(m), maxSquare, maxSquare)
	}
	return m
}

func (c rfail) args(args []Value, lo, hi int, usage string) {
	if len(args) < lo || len(args) > hi {
		c.fail("takes %s, got %d argument(s)", usage, len(args))
	}
}

func vecVal(xs []float64) *ArrayVal {
	out := make([]Value, len(xs))
	for i, x := range xs {
		out[i] = Num(x)
	}
	return &ArrayVal{Elements: out}
}

func matVal(m [][]float64) *ArrayVal {
	out := make([]Value, len(m))
	for i, r := range m {
		out[i] = vecVal(r)
	}
	return &ArrayVal{Elements: out}
}

// --- linear algebra ------------------------------------------------------------

func numPivotNote(sh *Shell, c rfail, lu *numeric.LU) {
	if lu.PivotRatio < 1e-10 {
		sh.Note(fmt.Sprintf("%s: the matrix is ill-conditioned (smallest/largest pivot %.2g); the result may have few correct digits", string(c), lu.PivotRatio))
	}
}

func numSolve(sh *Shell, args []Value) Value {
	const c = rfail("Num.solve")
	c.args(args, 2, 2, "(A, b)")
	a := c.square("A", args[0])
	var b [][]float64
	vec := !isMatrix(args[1])
	if vec {
		bv := c.vector("b", args[1])
		b = make([][]float64, len(bv))
		for i, x := range bv {
			b[i] = []float64{x}
		}
	} else {
		b = c.matrix("b", args[1])
	}
	if len(b) != len(a) {
		c.fail("A is %d x %d but b has %d rows; they must match", len(a), len(a), len(b))
	}
	lu := numeric.FactorLU(a)
	x, err := lu.Solve(b)
	if err != nil {
		c.fail("A is singular to working precision (det %.3g): there is no unique solution. For a least-squares solution of a full-rank tall system use Num.lstsq(A, b)", lu.Det())
	}
	numPivotNote(sh, c, lu)
	if vec {
		col := make([]float64, len(x))
		for i := range x {
			col[i] = x[i][0]
		}
		return vecVal(col)
	}
	return matVal(x)
}

func numInv(sh *Shell, args []Value) Value {
	const c = rfail("Num.inv")
	c.args(args, 1, 1, "(A)")
	lu := numeric.FactorLU(c.square("A", args[0]))
	x, err := lu.Inverse()
	if err != nil {
		c.fail("A is singular to working precision (det %.3g): it has no inverse", lu.Det())
	}
	numPivotNote(sh, c, lu)
	return matVal(x)
}

func numDet(args []Value) Value {
	const c = rfail("Num.det")
	c.args(args, 1, 1, "(A)")
	return Num(numeric.FactorLU(c.square("A", args[0])).Det())
}

func numEig(args []Value) Value {
	const c = rfail("Num.eig")
	c.args(args, 1, 1, "(A)")
	vals, vecs, err := numeric.SymEig(c.square("A", args[0]))
	if err != nil {
		c.fail("A is not symmetric (A[i][j] != A[j][i]); only symmetric matrices are supported (covariance, correlation, Hessian). General eigenvalues are not implemented")
	}
	out := NewObject()
	out.Set("values", vecVal(vals))
	out.Set("vectors", matVal(vecs))
	return out
}

func numLstsq(args []Value) Value {
	const c = rfail("Num.lstsq")
	c.args(args, 2, 2, "(A, b)")
	a := c.matrix("A", args[0])
	b := c.vector("b", args[1])
	if len(b) != len(a) {
		c.fail("A has %d rows but b has %d values; they must match", len(a), len(b))
	}
	if len(a) < len(a[0]) {
		c.fail("A is %d x %d: fewer rows than columns, so the solution is not unique. Give at least %d rows", len(a), len(a[0]), len(a[0]))
	}
	x, dep, err := numeric.Lstsq(a, b)
	if err != nil {
		c.fail("column %d of A is a linear combination of the columns before it (rank-deficient); drop it", dep)
	}
	res := make([]float64, len(a))
	ssr := 0.0
	for i := range a {
		s := 0.0
		for j := range x {
			s += a[i][j] * x[j]
		}
		res[i] = b[i] - s
		ssr += res[i] * res[i]
	}
	out := NewObject()
	out.Set("x", vecVal(x))
	out.Set("residuals", vecVal(res))
	out.Set("ssr", Num(ssr))
	return out
}

func numMatmul(args []Value) Value {
	const c = rfail("Num.matmul")
	c.args(args, 2, 2, "(A, B)")
	am, bm := isMatrix(args[0]), isMatrix(args[1])
	var a, b [][]float64
	if am {
		a = c.matrix("A", args[0])
	} else {
		a = [][]float64{c.vector("A", args[0])} // row
	}
	if bm {
		b = c.matrix("B", args[1])
	} else {
		v := c.vector("B", args[1])
		b = make([][]float64, len(v)) // column
		for i, x := range v {
			b[i] = []float64{x}
		}
	}
	if len(a[0]) != len(b) {
		c.fail("shapes do not match: A is %d x %d, B is %d x %d (A's columns must equal B's rows)", len(a), len(a[0]), len(b), len(b[0]))
	}
	if float64(len(a))*float64(len(b[0])) > maxNumEntries {
		c.fail("the result would be %d x %d; the limit is %d entries", len(a), len(b[0]), maxNumEntries)
	}
	p := numeric.MatMul(a, b)
	switch {
	case !am && !bm:
		return Num(p[0][0])
	case !bm:
		col := make([]float64, len(p))
		for i := range p {
			col[i] = p[i][0]
		}
		return vecVal(col)
	case !am:
		return vecVal(p[0])
	}
	return matVal(p)
}

func numTranspose(args []Value) Value {
	const c = rfail("Num.transpose")
	c.args(args, 1, 1, "(A)")
	return matVal(numeric.Transpose(c.matrix("A", args[0])))
}

// --- distributions -------------------------------------------------------------

var distAliases = map[string]string{
	"norm": "normal", "gaussian": "normal", "student": "t", "studentt": "t",
	"chisq": "chi2", "chisquare": "chi2", "chi-square": "chi2", "binom": "binomial",
	"expon": "exponential", "exp": "exponential", "lognorm": "lognormal",
}

func distNames() string {
	var parts []string
	for _, d := range numeric.Distributions {
		var ps []string
		for _, p := range d.Params {
			ps = append(ps, p.Name)
		}
		parts = append(parts, d.Name+" {"+strings.Join(ps, ", ")+"}")
	}
	return strings.Join(parts, ", ")
}

// numDist reads a distribution name and its params object.
func numDist(c rfail, nameV, paramsV Value) numeric.Dist {
	s, ok := nameV.(*StringVal)
	if !ok {
		c.fail("the distribution must be a name string. Distributions: %s", distNames())
	}
	name := strings.ToLower(s.V)
	if a, ok := distAliases[name]; ok {
		name = a
	}
	spec, ok := numeric.FindDist(name)
	if !ok {
		c.fail("unknown distribution %q. Distributions: %s", s.V, distNames())
	}
	var keys []string
	for _, p := range spec.Params {
		keys = append(keys, p.Name)
	}
	var po *ObjectVal
	if paramsV != nil && paramsV != Null {
		po, ok = paramsV.(*ObjectVal)
		if !ok {
			c.fail("params must be an object like {%s}, got %s", strings.Join(keys, ", "), describe(paramsV))
		}
		for _, k := range po.Keys() {
			if !contains(keys, k) {
				c.fail("%s has no parameter %q. Parameters: %s", name, k, strings.Join(keys, ", "))
			}
		}
	}
	vals := make([]float64, len(spec.Params))
	for i, p := range spec.Params {
		var v Value
		if po != nil {
			v, _ = po.Get(p.Name)
		}
		if v == nil || v == Null {
			if math.IsNaN(p.Default) {
				c.fail("%s needs %s: {%s}", name, p.Name, strings.Join(keys, ", "))
			}
			vals[i] = p.Default
			continue
		}
		x := c.nfloat(name+" "+p.Name, v)
		if p.Integer && x != math.Trunc(x) {
			c.fail("%s %s must be an integer, got %v", name, p.Name, x)
		}
		if msg := p.Check(x); msg != "" {
			c.fail("%s %s must be %s, got %v", name, p.Name, msg, x)
		}
		vals[i] = x
	}
	d, err := spec.Make(vals)
	if err != nil {
		c.fail("%s: %v", name, err)
	}
	return d
}

func numDistFn(fn string, args []Value) Value {
	c := rfail("Num." + fn)
	c.args(args, 2, 3, "(dist, x, params?)")
	d := numDist(c, args[0], argOpt(args, 2))
	one := func(what string, v Value) Value {
		x := c.nfloatInf(what, v)
		switch fn {
		case "pdf":
			return Num(d.PDF(x))
		case "cdf":
			return Num(d.CDF(x))
		case "sf":
			return Num(d.SF(x))
		}
		if x < 0 || x > 1 {
			c.fail("%s must be a probability from 0 to 1, got %v", what, x)
		}
		return Num(d.PPF(x))
	}
	what := "x"
	if fn == "ppf" {
		what = "p"
	}
	if a, ok := args[1].(*ArrayVal); ok {
		out := make([]Value, len(a.Elements))
		for i, e := range a.Elements {
			out[i] = one(fmt.Sprintf("%s[%d]", what, i), e)
		}
		return &ArrayVal{Elements: out}
	}
	return one(what, args[1])
}

// --- tests and intervals ---------------------------------------------------------

func numLevel(c rfail, o *ropts) float64 {
	v, ok := o.get("level")
	if !ok {
		return 0.95
	}
	l := c.nfloat("level", v)
	if !(l > 0 && l < 1) {
		c.fail("level must be between 0 and 1 (0.95 for 95%%), got %v", l)
	}
	return l
}

func numTTest(args []Value) Value {
	const c = rfail("Num.ttest")
	c.args(args, 1, 3, "(a, b?, opts?)")
	a := c.vector("a", args[0])
	var b []float64
	var optV Value
	if len(args) >= 2 {
		if _, isArr := args[1].(*ArrayVal); isArr {
			b = c.vector("b", args[1])
			optV = argOpt(args, 2)
		} else if len(args) == 2 {
			optV = args[1]
		} else {
			c.fail("b must be an array of numbers, got %s", describe(args[1]))
		}
	}
	o := &ropts{c, NewObject()}
	if optV != nil && optV != Null {
		o = c.options(optV, "opts", []string{"paired", "equalVar", "mu", "alternative", "level"})
	}
	paired := o.boolean("paired", false)
	equalVar := o.boolean("equalVar", false)
	mu := 0.0
	if v, ok := o.get("mu"); ok {
		mu = c.nfloat("mu", v)
	}
	altS := o.str("alternative", "two-sided", "two-sided", "less", "greater")
	alt := map[string]numeric.Alternative{"two-sided": numeric.TwoSided, "less": numeric.Less, "greater": numeric.Greater}[altS]
	level := numLevel(c, o)
	var r numeric.TTestResult
	var method string
	switch {
	case b == nil:
		if paired || equalVar {
			c.fail("paired and equalVar need a second sample b: Num.ttest(a, b, {paired: true})")
		}
		if len(a) < 2 {
			c.fail("a needs at least 2 values")
		}
		r, method = numeric.TTest1(a, mu, alt, level), "one-sample"
	case paired:
		if len(a) != len(b) {
			c.fail("paired: a has %d values and b has %d; they must be the same length (pairs)", len(a), len(b))
		}
		if equalVar {
			c.fail("equalVar does not apply to a paired test")
		}
		d := make([]float64, len(a))
		for i := range a {
			d[i] = a[i] - b[i]
		}
		if len(d) < 2 {
			c.fail("a paired test needs at least 2 pairs")
		}
		r, method = numeric.TTest1(d, mu, alt, level), "paired"
	default:
		if len(a) < 2 || len(b) < 2 {
			c.fail("each sample needs at least 2 values (a has %d, b has %d)", len(a), len(b))
		}
		method = "welch"
		if equalVar {
			method = "pooled"
		}
		r = numeric.TTest2(a, b, mu, equalVar, alt, level)
	}
	if math.IsNaN(r.T) || math.IsInf(r.T, 0) {
		c.fail("the standard error is 0 (every value is the same), so t is not defined")
	}
	out := NewObject()
	out.Set("t", Num(r.T))
	out.Set("df", Num(r.DF))
	out.Set("p", Num(r.P))
	out.Set("estimate", Num(r.Estimate))
	out.Set("stdErr", Num(r.StdErr))
	out.Set("lo", Num(r.CI[0]))
	out.Set("hi", Num(r.CI[1]))
	out.Set("level", Num(level))
	out.Set("method", Str(method))
	out.Set("alternative", Str(altS))
	return out
}

func numCI(args []Value) Value {
	const c = rfail("Num.ci")
	c.args(args, 1, 2, "(data, opts?)")
	o := &ropts{c, NewObject()}
	if v := argOpt(args, 1); v != nil && v != Null {
		o = c.options(v, "opts", []string{"level"})
	}
	level := numLevel(c, o)
	out := NewObject()
	if po, ok := args[0].(*ObjectVal); ok {
		pv := c.options(po, "data", []string{"k", "n"})
		kv, ok1 := pv.get("k")
		nv, ok2 := pv.get("n")
		if !ok1 || !ok2 {
			c.fail("a proportion needs {k: successes, n: trials}")
		}
		k, n := c.nfloat("k", kv), c.nfloat("n", nv)
		if n < 1 || k < 0 || k > n || k != math.Trunc(k) || n != math.Trunc(n) {
			c.fail("k and n must be integers with 0 <= k <= n and n >= 1, got k %v, n %v", k, n)
		}
		lo, hi := numeric.WilsonCI(k, n, level)
		out.Set("estimate", Num(k/n))
		out.Set("lo", Num(lo))
		out.Set("hi", Num(hi))
		out.Set("level", Num(level))
		out.Set("method", Str("wilson"))
		return out
	}
	xs := c.vector("data", args[0])
	if len(xs) < 2 {
		c.fail("data needs at least 2 values")
	}
	r := numeric.TTest1(xs, 0, numeric.TwoSided, level)
	out.Set("estimate", Num(r.Estimate))
	out.Set("lo", Num(r.CI[0]))
	out.Set("hi", Num(r.CI[1]))
	out.Set("stdErr", Num(r.StdErr))
	out.Set("n", Num(float64(len(xs))))
	out.Set("df", Num(r.DF))
	out.Set("level", Num(level))
	out.Set("method", Str("t"))
	return out
}

// --- random numbers ---------------------------------------------------------------

func numRNG(args []Value) Value {
	const c = rfail("Num.rng")
	c.args(args, 0, 1, "(seed?)")
	var seed uint64
	if v := argOpt(args, 0); v != nil && v != Null {
		s := c.nfloat("seed", v)
		if s != math.Trunc(s) || s < 0 || s > 1<<53 {
			c.fail("seed must be an integer from 0 to 2^53, got %v", s)
		}
		seed = uint64(s)
	} else {
		seed = uint64(time.Now().UnixNano()) & (1<<53 - 1)
	}
	g := numeric.NewRNG(seed)
	var mu sync.Mutex // parallel branches may share the generator
	out := NewObject()
	out.Set("seed", Num(float64(seed)))
	method := func(name, usage string, lo, hi int, fn func(c rfail, args []Value) Value) {
		mc := rfail("rng." + name)
		out.Set(name, &FuncVal{Name: "rng." + name, Body: &NativeBody{Fn: func(args []Value) Value {
			mc.args(args, lo, hi, usage)
			mu.Lock()
			defer mu.Unlock()
			return fn(mc, args)
		}}})
	}
	// count reads the optional count argument at i: -1 = one value.
	count := func(c rfail, args []Value, i int) int {
		v := argOpt(args, i)
		if v == nil || v == Null {
			return -1
		}
		n := c.nfloat("n", v)
		if n != math.Trunc(n) || n < 0 || n > maxNumEntries {
			c.fail("n must be an integer from 0 to %d, got %v", maxNumEntries, n)
		}
		return int(n)
	}
	many := func(n int, f func() Value) Value {
		if n < 0 {
			return f()
		}
		out := make([]Value, n)
		for i := range out {
			out[i] = f()
		}
		return &ArrayVal{Elements: out}
	}
	optF := func(c rfail, args []Value, i int, what string, def float64) float64 {
		if v := argOpt(args, i); v != nil && v != Null {
			return c.nfloat(what, v)
		}
		return def
	}
	elems := func(c rfail, v Value) []Value {
		a, ok := v.(*ArrayVal)
		if !ok {
			c.fail("needs an array, got %s", describe(v))
		}
		return a.Elements
	}
	method("uniform", "(lo?, hi?, n?): uniform in [lo, hi), default [0, 1)", 0, 3, func(c rfail, args []Value) Value {
		lo, hi := optF(c, args, 0, "lo", 0), optF(c, args, 1, "hi", 1)
		if !(hi > lo) {
			c.fail("hi must be greater than lo, got lo %v, hi %v", lo, hi)
		}
		return many(count(c, args, 2), func() Value { return Num(lo + (hi-lo)*g.Float64()) })
	})
	method("normal", "(mu?, sigma?, n?): normal draws, default mu 0, sigma 1", 0, 3, func(c rfail, args []Value) Value {
		m, s := optF(c, args, 0, "mu", 0), optF(c, args, 1, "sigma", 1)
		if !(s > 0) {
			c.fail("sigma must be > 0, got %v", s)
		}
		return many(count(c, args, 2), func() Value { return Num(m + s*g.Normal()) })
	})
	method("int", "(lo, hi, n?): integer from lo to hi, both included", 2, 3, func(c rfail, args []Value) Value {
		lo, hi := c.nfloat("lo", args[0]), c.nfloat("hi", args[1])
		if lo != math.Trunc(lo) || hi != math.Trunc(hi) || hi < lo || hi-lo >= 1<<53 {
			c.fail("lo and hi must be integers with lo <= hi, got %v, %v", lo, hi)
		}
		span := uint64(hi-lo) + 1
		return many(count(c, args, 2), func() Value { return Num(lo + float64(g.Below(span))) })
	})
	method("choice", "(xs, n?): a random element, or n drawn with replacement", 1, 2, func(c rfail, args []Value) Value {
		xs := elems(c, args[0])
		if len(xs) == 0 {
			c.fail("the array is empty")
		}
		return many(count(c, args, 1), func() Value { return xs[g.Below(uint64(len(xs)))] })
	})
	method("sample", "(xs, k): k distinct elements, without replacement, in random order", 2, 2, func(c rfail, args []Value) Value {
		xs := elems(c, args[0])
		k := c.nfloat("k", args[1])
		if k != math.Trunc(k) || k < 0 || int(k) > len(xs) {
			c.fail("k must be an integer from 0 to %d (the array length), got %v", len(xs), k)
		}
		idx := g.Sample(len(xs), int(k))
		out := make([]Value, len(idx))
		for i, j := range idx {
			out[i] = xs[j]
		}
		return &ArrayVal{Elements: out}
	})
	method("shuffle", "(xs): a shuffled copy (xs is not changed)", 1, 1, func(c rfail, args []Value) Value {
		xs := elems(c, args[0])
		p := g.Perm(len(xs))
		out := make([]Value, len(xs))
		for i, j := range p {
			out[i] = xs[j]
		}
		return &ArrayVal{Elements: out}
	})
	method("draw", "(dist, params?, n?): draws from a named distribution, e.g. draw(\"poisson\", {lambda: 3}, 100)", 1, 3, func(c rfail, args []Value) Value {
		d := numDist(c, args[0], argOpt(args, 1))
		return many(count(c, args, 2), func() Value { return Num(g.Draw(d)) })
	})
	return out
}
