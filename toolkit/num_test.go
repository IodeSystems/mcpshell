package toolkit_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func numShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := toolkit.AddNum(toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell())))
	okEval(t, sh, `load("num")`)
	return sh
}

// numRefs reads the scipy/statsmodels reference values of the numeric
// package (numeric/testdata/refs.py).
func numRefs(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("../numeric/testdata/scipy_refs.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func numFloat(t *testing.T, sh *runtime.Shell, src string) float64 {
	t.Helper()
	s := okEval(t, sh, src)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%s: not a number: %s", src, s)
	}
	return f
}

func relNear(got, want, rel float64) bool {
	return math.Abs(got-want) <= rel*math.Abs(want)+1e-300
}

func TestNumLoad(t *testing.T) {
	sh := toolkit.AddNum(toolkit.InstallCore(runtime.NewShell()))
	mustContain(t, "call before load", failEval(t, sh, `Num.det([[1]])`), `load("num")`)
	first := okEval(t, sh, `load("num")`)
	mustContain(t, "first load", first, "Num.regress(", "Num.fit(", "Num.minimize(", "Num.root(", "Num.integrate(",
		"Num.interp(", "Num.cdf(", "Num.ttest(", "Num.solve(", "Num.eig(", "Num.rng(", "TYPICAL: Spectrum peaks", "TRAPS")
	if strings.Contains(first, "…") {
		t.Errorf("a description is cut in the usage block:\n%s", first)
	}
	if n := strings.Count(okEval(t, sh, `help("num")`), "\n") + 1; n > 50 {
		t.Errorf("guide is %d lines", n)
	}
}

func TestNumRegressMatchesStatsmodels(t *testing.T) {
	var o struct {
		Rows                   json.RawMessage
		Coef, SE, T, P, Lo, Hi []float64
		R2, AdjR2, F, FP       float64
	}
	if err := json.Unmarshal(numRefs(t)["ols"], &o); err != nil {
		t.Fatal(err)
	}
	sh := numShell(t)
	okEval(t, sh, "export rows = "+string(o.Rows))
	okEval(t, sh, `export r = Num.regress(rows, {y: "sales", x: ["price", "ads"]})`)
	for j, term := range []string{"intercept", "price", "ads"} {
		for k, want := range map[string]float64{"estimate": o.Coef[j], "stdErr": o.SE[j], "t": o.T[j], "p": o.P[j], "lo": o.Lo[j], "hi": o.Hi[j]} {
			got := numFloat(t, sh, fmt.Sprintf("r.coefficients[%d].%s", j, k))
			if !relNear(got, want, 1e-9) {
				t.Errorf("%s %s = %v, statsmodels %v", term, k, got, want)
			}
		}
	}
	for k, want := range map[string]float64{"r2": o.R2, "adjR2": o.AdjR2, "f": o.F, "fP": o.FP} {
		if got := numFloat(t, sh, "r."+k); !relNear(got, want, 1e-9) {
			t.Errorf("%s = %v, statsmodels %v", k, got, want)
		}
	}
	if got := okEval(t, sh, `r.coef.price == r.coefficients[1].estimate && r.residuals.length == 40 && r.n == 40`); got != "true" {
		t.Errorf("coef/residuals: %s", got)
	}
	// A null row is skipped and listed.
	okEval(t, sh, `export r2 = Num.regress([...rows, {price: null, ads: 1, sales: 3}], {y: "sales", x: ["price", "ads"]})`)
	if got := okEval(t, sh, `[r2.n, r2.skipped]`); got != "[40, [40]]" {
		t.Errorf("skipped: %s", got)
	}
}

// TestNumFitPeaksRaman fits two Lorentzians and a Gaussian on a linear
// baseline through the language, and compares with scipy curve_fit.
func TestNumFitPeaksRaman(t *testing.T) {
	var ra struct {
		X, Y  json.RawMessage
		P, SE []float64
		SSR   float64
	}
	if err := json.Unmarshal(numRefs(t)["raman"], &ra); err != nil {
		t.Fatal(err)
	}
	sh := numShell(t)
	okEval(t, sh, "export xs = "+string(ra.X)+"; export ys = "+string(ra.Y))
	start := time.Now()
	okEval(t, sh, `export f = Num.fit({x: xs, y: ys, baseline: "linear", peaks: [
	  {shape: "lorentzian", center: 475, height: 100, width: 10},
	  {shape: "lorentzian", center: 522, height: 250, width: 10},
	  {shape: "gaussian", center: 600, height: 60, width: 20}]})`)
	t.Logf("peak fit: %v", time.Since(start))
	if got := okEval(t, sh, `f.status`); got != "converged" {
		t.Errorf("status %s", got)
	}
	keys := []string{"center", "height", "width"}
	for k := range 3 {
		for j, key := range keys {
			got := numFloat(t, sh, fmt.Sprintf("f.peaks[%d].%s", k, key))
			se := numFloat(t, sh, fmt.Sprintf("f.peaks[%d].stdErr.%s", k, key))
			if !relNear(got, ra.P[3*k+j], 1e-6) || !relNear(se, ra.SE[3*k+j], 1e-3) {
				t.Errorf("peak %d %s = %v ± %v, scipy %v ± %v", k, key, got, se, ra.P[3*k+j], ra.SE[3*k+j])
			}
		}
	}
	for k, want := range map[string]float64{"baseline.c0": ra.P[9], "baseline.c1": ra.P[10], "ssr": ra.SSR} {
		if got := numFloat(t, sh, "f."+k); !relNear(got, want, 1e-6) {
			t.Errorf("%s = %v, scipy %v", k, got, want)
		}
	}
	// Lorentzian area = pi/2 * height * width.
	area := numFloat(t, sh, "f.peaks[1].area")
	if want := math.Pi / 2 * ra.P[4] * ra.P[5]; !relNear(area, want, 1e-6) {
		t.Errorf("area %v, want %v", area, want)
	}
	if se := numFloat(t, sh, "f.peaks[1].stdErr.area"); !(se > 0 && se < area/10) {
		t.Errorf("area stdErr %v", se)
	}
}

func TestNumFitModel(t *testing.T) {
	var e struct {
		X, Y  json.RawMessage
		P, SE []float64
	}
	if err := json.Unmarshal(numRefs(t)["expfit"], &e); err != nil {
		t.Fatal(err)
	}
	sh := numShell(t)
	okEval(t, sh, "export xs = "+string(e.X)+"; export ys = "+string(e.Y))
	okEval(t, sh, `export f = Num.fit({x: xs, y: ys, model: (x, p) => p.a * Math.exp(-p.k * x) + p.c, p0: {a: 1, k: 1, c: 0}})`)
	for j, k := range []string{"a", "k", "c"} {
		got, se := numFloat(t, sh, "f.params."+k), numFloat(t, sh, "f.stdErr."+k)
		if !relNear(got, e.P[j], 1e-6) || !relNear(se, e.SE[j], 1e-3) {
			t.Errorf("%s = %v ± %v, scipy %v ± %v", k, got, se, e.P[j], e.SE[j])
		}
	}
	// Array p0 gives arrays; the covariance diagonal is stdErr squared.
	okEval(t, sh, `export g = Num.fit({x: xs, y: ys, model: (x, p) => p[0] * Math.exp(-p[1] * x) + p[2], p0: [1, 1, 0]})`)
	if got := okEval(t, sh, `Math.abs(g.covariance[1][1] - g.stdErr[1] ** 2) < 1e-15 && g.params.length == 3`); got != "true" {
		t.Errorf("array form: %s", got)
	}
	// Not converged: status and note say so.
	sh.Limits().ClearWarnings()
	if got := okEval(t, sh, `Num.fit({x: xs, y: ys, model: (x, p) => p[0] * Math.exp(-p[1] * x) + p[2], p0: [1, 1, 0], maxIter: 1}).status`); got != "max_iterations" {
		t.Errorf("maxIter 1: %s", got)
	}
	mustContain(t, "note", strings.Join(sh.Limits().Notes(), "\n"), "Num.fit: not converged")
	// Bounds in the shape of p0.
	if got := okEval(t, sh, `Num.fit({x: xs, y: ys, model: (x, p) => p.a * Math.exp(-p.k * x) + p.c, p0: {a: 1, k: 1, c: 0}, bounds: {c: [0, 0.2]}}).params.c`); got != "0.2" {
		t.Errorf("bound: %s", got)
	}
	// x entries may be rows for a model with several inputs.
	got := okEval(t, sh, `Math.round(Num.fit({x: [{u: 1, v: 0}, {u: 0, v: 1}, {u: 1, v: 1}, {u: 2, v: 1}], y: [2, 3, 5, 7], model: (r, p) => p[0] * r.u + p[1] * r.v, p0: [1, 1]}).params[0] * 1e6) / 1e6`)
	if got != "2" {
		t.Errorf("row x: %s", got)
	}
}

func TestNumDistributionsAndTests(t *testing.T) {
	sh := numShell(t)
	for src, want := range map[string]float64{
		`Num.cdf("normal", 1.96)`:                   0.9750021048517795,
		`Num.sf("t", 2.015, {df: 5})`:               0.05000308616340318,
		`Num.ppf("chi2", 0.95, {df: 3})`:            7.814727903251178,
		`Num.ppf("norm", 0.975)`:                    1.959963984540054,
		`Num.pdf("poisson", 2, {lambda: 4.5})`:      0.11247858994970336,
		`Num.ppf("binomial", 0.5, {n: 20, p: 0.3})`: 6,
	} {
		if got := numFloat(t, sh, src); !relNear(got, want, 1e-7) {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	if got := okEval(t, sh, `Num.cdf("normal", [0, 1e9])`); got != "[0.5, 1]" {
		t.Errorf("array x: %s", got)
	}
	// scipy: ttest_ind(a, b, equal_var=False) -> t 3.5223, p 0.002949...
	got := okEval(t, sh, `let r = Num.ttest([5.1, 4.9, 5.6, 5.8, 6.0, 5.3, 5.7, 6.1, 4.8, 5.5], [4.2, 4.8, 5.0, 4.4, 5.1, 4.9, 4.0, 4.6]); [r.method, Math.round(r.t * 1e4) / 1e4, r.p < 0.01, r.lo < r.estimate && r.estimate < r.hi]`)
	if got != `["welch", 4.3015, true, true]` {
		t.Errorf("welch: %s", got)
	}
	if got := okEval(t, sh, `Num.ttest([1, 2, 3, 4], {mu: 1}).method`); got != "one-sample" {
		t.Errorf("one-sample: %s", got)
	}
	if got := okEval(t, sh, `Num.ttest([1, 2, 3], [1.5, 2.5, 3.1], {paired: true}).method`); got != "paired" {
		t.Errorf("paired: %s", got)
	}
	if got := okEval(t, sh, `let c = Num.ci({k: 17, n: 60}); [c.method, Math.round(c.lo * 1e6) / 1e6]`); got != `["wilson", 0.185068]` {
		t.Errorf("wilson: %s", got)
	}
}

func TestNumLinalgAndMethods(t *testing.T) {
	sh := numShell(t)
	for src, want := range map[string]string{
		`Num.solve([[2, 1], [1, 3]], [3, 5])`:   `[0.8, 1.4]`,
		`Num.matmul([[1, 2], [3, 4]], [1, 1])`:  `[3, 7]`,
		`Num.matmul([1, 2], [3, 4])`:            `11`,
		`Num.transpose([[1, 2, 3], [4, 5, 6]])`: `[[1, 4], [2, 5], [3, 6]]`,
		`Num.inv([[2, 0], [0, 4]])`:             `[[0.5, 0], [0, 0.25]]`,
		`Num.lstsq([[1, 1], [1, 2], [1, 3]], [1, 2, 3]).x.map(v => Math.round(v * 1e9) / 1e9)`:                                                      `[0, 1]`,
		`Math.abs(Num.root(x => x * x - 2, [0, 2]).x - Math.SQRT2) < 1e-12`:                                                                         `true`,
		`Math.round(Num.integrate(x => Math.exp(-x * x), -Infinity, Infinity).value * 1e12) / 1e12 == Math.round(Math.sqrt(Math.PI) * 1e12) / 1e12`: `true`,
		`Num.integrate([0, 1, 2], [0, 1, 0])`:          `1`,
		`Num.interp([0, 1, 2], [0, 10, 40], 1.5)`:      `25`,
		`Num.interp([0, 1, 2], [0, 10, 40], [0.5, 2])`: `[5, 40]`,
		`let m = Num.minimize(p => (p[0] - 1) ** 2 + (p[1] + 2) ** 2, [0, 0]); [m.status, m.x.map(v => Math.round(v * 1e4) / 1e4)]`: `["converged", [1, -2]]`,
		`let m2 = Num.minimize(x => (x - 3) ** 2, 0, {bounds: [-1, 2]}); Math.round(m2.x * 1e6) / 1e6`:                              `2`,
		`Num.minimize(p => (p.a - 1) ** 2, {a: 5}).x.a > 0.999`:                                                                     `true`,
	} {
		if got := okEval(t, sh, src); got != want {
			t.Errorf("%s = %s, want %s", src, got, want)
		}
	}
}

func TestNumRNG(t *testing.T) {
	sh := numShell(t)
	a := okEval(t, sh, `let g = Num.rng(42); [g.normal(0, 1, 3), g.uniform(), g.int(1, 6, 5), g.choice(["a", "b", "c"]), g.sample([1, 2, 3, 4, 5], 2), g.shuffle([1, 2, 3]), g.draw("poisson", {lambda: 3}, 3)]`)
	b := okEval(t, sh, `let h = Num.rng(42); [h.normal(0, 1, 3), h.uniform(), h.int(1, 6, 5), h.choice(["a", "b", "c"]), h.sample([1, 2, 3, 4, 5], 2), h.shuffle([1, 2, 3]), h.draw("poisson", {lambda: 3}, 3)]`)
	if a != b {
		t.Errorf("same seed, different values:\n%s\n%s", a, b)
	}
	if got := okEval(t, sh, `let g2 = Num.rng(); typeof g2.seed == "number" && Num.rng(g2.seed).uniform() == g2.uniform()`); got != "true" {
		t.Errorf("seedless rng is not reproducible from its seed: %s", got)
	}
	if got := okEval(t, sh, `let g3 = Num.rng(1); let ys = g3.int(1, 6, 6000); [min(ys), max(ys)]`); got != "[1, 6]" {
		t.Errorf("int range: %s", got)
	}
	mustContain(t, "rng arg error", failEval(t, sh, `Num.rng(1).int(1)`), "rng.int", "(lo, hi, n?)")
}

func TestNumErrors(t *testing.T) {
	sh := numShell(t)
	for src, wants := range map[string][]string{
		`Num.cdf("weibull", 1)`:                              {"unknown distribution", "normal {mu, sigma}", "poisson {lambda}"},
		`Num.cdf("t", 1)`:                                    {"t needs df"},
		`Num.cdf("t", 1, {dof: 3})`:                          {`no parameter "dof"`, "Parameters: df"},
		`Num.ppf("normal", 1.5)`:                             {"probability from 0 to 1"},
		`Num.cdf("binomial", 1, {n: 2.5, p: 0.5})`:           {"n must be an integer"},
		`Num.solve([[1, 2], [2, 4]], [1, 2])`:                {"singular", "Num.lstsq"},
		`Num.solve([[1, 2], [3]], [1, 2])`:                   {"A[1] must be an array of 2 numbers"},
		`Num.solve([[1, 2], [3, 4]], [1, 2, 3])`:             {"b has 3 rows"},
		`Num.eig([[1, 2], [3, 4]])`:                          {"not symmetric", "only symmetric"},
		`Num.matmul([[1, 2]], [[1, 2]])`:                     {"shapes do not match", "1 x 2", "1 x 2"},
		`Num.det([[1, 2]])`:                                  {"must be square"},
		`Num.det([[1, NaN], [1, 1]])`:                        {"A[0][1] must be a finite number"},
		`Num.root(x => x * x + 1, [-1, 1])`:                  {"same sign", "f(-1) = 2"},
		`Num.interp([0, 1], [0, 1], 2)`:                      {"outside the data range", "extrapolate: true"},
		`Num.interp([0, 2, 1], [0, 1, 2], 1)`:                {"strictly increasing", "xs[2] = 1"},
		`Num.interp([0, 1], [0, 1], 0.5, {methd: "cubic"})`:  {`unknown key "methd"`, `did you mean "method"`},
		`Num.interp([0, 1], [0, 1], 0.5, {method: "akima"})`: {`method must be "linear" | "cubic"`},
		`Num.regress([{a: 1, b: 2}, {a: 2, b: 4}, {a: 3, b: 5}, {a: 4, b: 9}], {y: "b", x: ["a", "a2"]})`:                        {`no column "a2"`},
		`Num.regress([{a: 1, c: 2, b: 2}, {a: 2, c: 4, b: 4}, {a: 3, c: 6, b: 5}, {a: 4, c: 8, b: 9}], {y: "b", x: ["a", "c"]})`: {`"c" is a linear combination`},
		`Num.regress([{a: "x", b: 2}, {a: 2, b: 4}, {a: 3, b: 5}], {y: "b", x: ["a"]})`:                                          {"rows[0].a is string", "0/1 columns"},
		`Num.fit({x: [1, 2], y: [1, 2, 3], model: (x, p) => p[0], p0: [1]})`:                                                     {"one entry per y value (3)"},
		`Num.fit({x: [1, 2, 3], y: [1, 2, 3], model: (x, p) => "a", p0: [1]})`:                                                   {"must return a number", `"a"`, "x[0] = 1"},
		`Num.fit({x: [1, 2, 3], y: [1, 2, 3], model: (x, p) => Math.log(p[0]), p0: [-1]})`:                                       {"with the starting parameters"},
		`Num.fit({x: [1, 2, 3], y: [1, 2, 3], peaks: [{shape: "voigt", center: 2}]})`:                                            {`unknown shape "voigt"`, `"pseudovoigt"`},
		`Num.fit({x: [1, 2, 3], y: [1, 2, 3], model: (x, p) => p[0], p0: [1], peaks: []})`:                                       {"not both"},
		`Num.ttest([1, 2, 3], [4, 5], {paired: true})`:                                                                           {"same length"},
		`Num.ttest([1, 1, 1], {mu: 0})`: {"standard error is 0"},
	} {
		mustContain(t, src, failEval(t, sh, src), wants...)
	}
	// Errors end with the command's usage.
	mustContain(t, "usage", failEval(t, sh, `Num.det(5)`), "Usage: Num.det(")
	// A divergent integral is not reported as converged.
	sh.Limits().ClearWarnings()
	if got := okEval(t, sh, `Num.integrate(x => 1 / x, 0, 1).status`); got != "max_iterations" {
		t.Errorf("1/x on [0, 1]: %s", got)
	}
	mustContain(t, "note", strings.Join(sh.Limits().Notes(), "\n"), "Num.integrate: not converged")
}

func TestNumTimeLimit(t *testing.T) {
	sh := numShell(t)
	sh.Limits().SetDefaults(100_000_000, 256, 400)
	start := time.Now()
	got := okEval(t, sh, `Num.minimize(p => { let s = 0; for (let i = 0; i < 100; i++) s += Math.sin(p[0] + i); return s * s + Math.random() }, [0], {maxIter: 1000000, tol: 1e-15}).status`)
	if got != "stopped" {
		t.Errorf("status %s", got)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
	mustContain(t, "note", strings.Join(sh.Limits().Notes(), "\n"), "time limit stopped it")
}

func TestNumFitPeakBounds(t *testing.T) {
	sh := numShell(t)
	okEval(t, sh, `export xs = range(0, 201).map(i => 400 + i); export ys = xs.map(x => 100 / (1 + 4 * ((x - 520) / 8) ** 2) + 5)`)
	sh.Limits().ClearWarnings()
	okEval(t, sh, `export f = Num.fit({x: xs, y: ys, baseline: "constant", peaks: [{shape: "pseudovoigt", center: 503, width: 6, bounds: {center: [500, 505]}}]})`)
	mustContain(t, "note", strings.Join(sh.Limits().Notes(), "\n"), "at a bound: peaks[0].center")
	if got := okEval(t, sh, `[f.peaks[0].center, f.peaks[0].eta >= 0 && f.peaks[0].eta <= 1]`); got != "[505, true]" {
		t.Errorf("bounded center: %s", got)
	}
	// Unbounded, the pseudo-Voigt finds the Lorentzian: eta 1, center 520.
	got := okEval(t, sh, `let g = Num.fit({x: xs, y: ys, baseline: "constant", peaks: [{shape: "pseudovoigt", center: 515, width: 6}]}); [g.status, Math.round(g.peaks[0].center * 1e6) / 1e6, Math.round(g.peaks[0].eta * 1e6) / 1e6, Math.round(g.baseline.c0 * 1e6) / 1e6]`)
	if got != `["converged", 520, 1, 5]` {
		t.Errorf("pseudovoigt: %s", got)
	}
	mustContain(t, "loc", failEval(t, sh, `Num.cdf("normal", 0, {loc: 1})`), `no parameter "loc"`, "mu, sigma")
}
