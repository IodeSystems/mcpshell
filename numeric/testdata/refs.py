import json, sys
import numpy as np
from scipy import stats, optimize, integrate, interpolate
import statsmodels.api as sm

out = {}
r = lambda v: float(repr(float(v))) if np.isfinite(v) else (None if np.isnan(v) else ("inf" if v > 0 else "-inf"))

# Distributions: name, scipy object, params dict, xs, ps
D = [
    ("normal", stats.norm(loc=1.5, scale=2), {"mu": 1.5, "sigma": 2}, [-7, -1, 0, 1.5, 3, 12], [1e-10, 0.025, 0.3, 0.5, 0.975, 1 - 1e-10]),
    ("t", stats.t(5), {"df": 5}, [-30, -2.5, -0.3, 0, 1, 2.015, 40], [1e-8, 0.025, 0.4, 0.5, 0.95, 0.999999]),
    ("t", stats.t(1.5), {"df": 1.5}, [-5, 0.7, 3], [0.01, 0.9]),
    ("t", stats.t(200), {"df": 200}, [-3, 1.96], [0.025, 0.995]),
    ("chi2", stats.chi2(3), {"df": 3}, [0.01, 1, 3, 7.81, 40], [1e-6, 0.05, 0.5, 0.95, 0.9999]),
    ("chi2", stats.chi2(150), {"df": 150}, [100, 150, 220], [0.01, 0.99]),
    ("f", stats.f(4, 17), {"df1": 4, "df2": 17}, [0.1, 1, 2.96, 15], [0.05, 0.5, 0.95, 0.9999]),
    ("binomial", stats.binom(20, 0.3), {"n": 20, "p": 0.3}, [0, 3, 6, 10, 20], [0.001, 0.25, 0.5, 0.9, 0.999]),
    ("poisson", stats.poisson(4.5), {"lambda": 4.5}, [0, 2, 4, 9, 20], [0.01, 0.5, 0.95, 0.99999]),
    ("poisson", stats.poisson(1000), {"lambda": 1000}, [900, 1000, 1100], [0.05, 0.95]),
    ("exponential", stats.expon(scale=1 / 0.5), {"rate": 0.5}, [0.1, 2, 20], [0.1, 0.5, 0.99]),
    ("gamma", stats.gamma(2.5, scale=3), {"shape": 2.5, "scale": 3}, [0.5, 7, 30], [0.05, 0.5, 0.95]),
    ("beta", stats.beta(2, 5), {"a": 2, "b": 5}, [0.05, 0.3, 0.9], [0.01, 0.5, 0.99]),
    ("uniform", stats.uniform(2, 3), {"lo": 2, "hi": 5}, [1, 2.5, 4.9], [0.1, 0.5]),
    ("lognormal", stats.lognorm(0.5, scale=np.exp(1)), {"mu": 1, "sigma": 0.5}, [0.5, 2.7, 9], [0.05, 0.5, 0.95]),
]
dist = []
for name, d, params, xs, ps in D:
    disc = name in ("binomial", "poisson")
    for x in xs:
        dist.append({"dist": name, "params": params, "fn": "pdf", "at": x, "want": r(d.pmf(x) if disc else d.pdf(x))})
        dist.append({"dist": name, "params": params, "fn": "cdf", "at": x, "want": r(d.cdf(x))})
        dist.append({"dist": name, "params": params, "fn": "sf", "at": x, "want": r(d.sf(x))})
    for p in ps:
        dist.append({"dist": name, "params": params, "fn": "ppf", "at": p, "want": r(d.ppf(p))})
# A seeded grid over parameters and tails, for coverage beyond the hand-picked points.
g = np.random.default_rng(11)
for _ in range(60):
    df = float(np.round(10 ** g.uniform(-0.5, 3), 3))
    x = float(np.round(g.normal(0, 4), 4))
    p = float(10 ** g.uniform(-12, -0.31))
    d = stats.t(df)
    dist += [{"dist": "t", "params": {"df": df}, "fn": fn, "at": x, "want": r(getattr(d, fn)(x))} for fn in ("pdf", "cdf", "sf")]
    dist += [{"dist": "t", "params": {"df": df}, "fn": "ppf", "at": q, "want": r(d.ppf(q))} for q in (p, 1 - p)]
    k = float(np.round(10 ** g.uniform(-1, 3), 3))
    d = stats.chi2(k)
    x = float(np.round(k * 10 ** g.uniform(-1, 0.6), 4))
    dist += [{"dist": "chi2", "params": {"df": k}, "fn": fn, "at": x, "want": r(getattr(d, fn)(x))} for fn in ("pdf", "cdf", "sf")]
    dist += [{"dist": "chi2", "params": {"df": k}, "fn": "ppf", "at": q, "want": r(d.ppf(q))} for q in (p, 1 - p)]
    d1, d2 = float(np.round(10 ** g.uniform(0, 2.5))), float(np.round(10 ** g.uniform(0, 2.5)))
    d = stats.f(d1, d2)
    x = float(np.round(10 ** g.uniform(-1, 1), 4))
    dist += [{"dist": "f", "params": {"df1": d1, "df2": d2}, "fn": fn, "at": x, "want": r(getattr(d, fn)(x))} for fn in ("pdf", "cdf", "sf")]
    dist += [{"dist": "f", "params": {"df1": d1, "df2": d2}, "fn": "ppf", "at": q, "want": r(d.ppf(q))} for q in (p, 1 - p)]
    n = float(g.integers(1, 5000)); pp = float(np.round(g.uniform(0.001, 0.999), 4))
    d = stats.binom(n, pp)
    x = float(np.round(n * pp + g.normal(0, 3) * np.sqrt(n * pp * (1 - pp))))
    dist += [{"dist": "binomial", "params": {"n": n, "p": pp}, "fn": "pdf", "at": x, "want": r(d.pmf(x))}]
    dist += [{"dist": "binomial", "params": {"n": n, "p": pp}, "fn": fn, "at": x, "want": r(getattr(d, fn)(x))} for fn in ("cdf", "sf")]
    dist += [{"dist": "binomial", "params": {"n": n, "p": pp}, "fn": "ppf", "at": q, "want": r(d.ppf(q))} for q in (p, 1 - p)]
    lam = float(np.round(10 ** g.uniform(-2, 4), 3))
    d = stats.poisson(lam)
    x = float(max(0, np.round(lam + g.normal(0, 3) * np.sqrt(lam))))
    dist += [{"dist": "poisson", "params": {"lambda": lam}, "fn": "pdf", "at": x, "want": r(d.pmf(x))}]
    dist += [{"dist": "poisson", "params": {"lambda": lam}, "fn": fn, "at": x, "want": r(getattr(d, fn)(x))} for fn in ("cdf", "sf")]
    dist += [{"dist": "poisson", "params": {"lambda": lam}, "fn": "ppf", "at": q, "want": r(d.ppf(q))} for q in (p, 1 - p)]
    a, b = float(np.round(10 ** g.uniform(-1, 2), 3)), float(np.round(10 ** g.uniform(-1, 2), 3))
    d = stats.beta(a, b)
    x = float(np.round(g.uniform(0.001, 0.999), 4))
    dist += [{"dist": "beta", "params": {"a": a, "b": b}, "fn": fn, "at": x, "want": r(getattr(d, fn)(x))} for fn in ("pdf", "cdf", "sf")]
    dist += [{"dist": "beta", "params": {"a": a, "b": b}, "fn": "ppf", "at": q, "want": r(d.ppf(q))} for q in (p, 1 - p)]
out["dist"] = dist

# Linear algebra
A = [[4, -2, 1], [3, 6, -4], [2, 1, 8]]
b = [12, -25, 32]
out["solve"] = {"A": A, "b": b, "x": [r(v) for v in np.linalg.solve(A, b)]}
out["inv"] = {"A": A, "inv": [[r(v) for v in row] for row in np.linalg.inv(A)], "det": r(np.linalg.det(A))}
S = [[4, 1, 2], [1, 3, 0.5], [2, 0.5, 5]]
w, v = np.linalg.eigh(S)
out["eigh"] = {"A": S, "values": [r(x) for x in w], "vectors": [[r(x) for x in v[:, i]] for i in range(3)]}
X = [[1, 1], [1, 2], [1, 3], [1, 4], [1, 5]]
y = [2.1, 3.9, 6.2, 7.8, 10.1]
sol, res, rank, sv = np.linalg.lstsq(X, y, rcond=None)
out["lstsq"] = {"A": X, "b": y, "x": [r(v) for v in sol], "ssr": r(res[0])}

# OLS (statsmodels) on a fixed dataset
rng = np.random.default_rng(7)
n = 40
price = np.round(rng.uniform(5, 15, n), 2)
ads = np.round(rng.uniform(0, 10, n), 2)
sales = np.round(50 - 2.5 * price + 1.8 * ads + rng.normal(0, 3, n), 2)
Xm = sm.add_constant(np.column_stack([price, ads]))
m = sm.OLS(sales, Xm).fit()
ci = m.conf_int(0.05)
out["ols"] = {
    "rows": [{"price": float(a), "ads": float(b_), "sales": float(c)} for a, b_, c in zip(price, ads, sales)],
    "coef": [r(v) for v in m.params], "se": [r(v) for v in m.bse], "t": [r(v) for v in m.tvalues],
    "p": [r(v) for v in m.pvalues], "lo": [r(v) for v in ci[:, 0]], "hi": [r(v) for v in ci[:, 1]],
    "r2": r(m.rsquared), "adjR2": r(m.rsquared_adj), "f": r(m.fvalue), "fP": r(m.f_pvalue),
    "sigma": r(np.sqrt(m.scale)), "df": int(m.df_resid),
}
m0 = sm.OLS(sales, np.column_stack([price, ads])).fit()
out["ols_noint"] = {"coef": [r(v) for v in m0.params], "se": [r(v) for v in m0.bse], "r2": r(m0.rsquared),
                    "adjR2": r(m0.rsquared_adj), "f": r(m0.fvalue), "fP": r(m0.f_pvalue)}

# curve_fit: exponential decay
xd = np.linspace(0, 4, 25)
yd = np.round(3.2 * np.exp(-1.3 * xd) + 0.4 + rng.normal(0, 0.05, xd.size), 4)
f = lambda x, a, k, c: a * np.exp(-k * x) + c
popt, pcov = optimize.curve_fit(f, xd, yd, p0=[1, 1, 0])
out["expfit"] = {"x": xd.tolist(), "y": yd.tolist(), "p": [r(v) for v in popt], "se": [r(v) for v in np.sqrt(np.diag(pcov))],
                 "ssr": r(np.sum((yd - f(xd, *popt)) ** 2))}

# Raman-style: 3 peaks (2 Lorentzian, 1 Gaussian) on a linear baseline, FWHM widths.
def lor(x, c, h, w): return h / (1 + 4 * ((x - c) / w) ** 2)
def gau(x, c, h, w): return h * np.exp(-4 * np.log(2) * ((x - c) / w) ** 2)
def raman(x, c1, h1, w1, c2, h2, w2, c3, h3, w3, b0, b1):
    return lor(x, c1, h1, w1) + lor(x, c2, h2, w2) + gau(x, c3, h3, w3) + b0 + b1 * x
xr = np.linspace(400, 700, 601)
true = [480, 120, 12, 520, 300, 8, 610, 80, 25, 20, 0.02]
yr = np.round(raman(xr, *true) + rng.normal(0, 2, xr.size), 3)
p0 = [475, 100, 10, 522, 250, 10, 600, 60, 20, 0, 0]
popt, pcov = optimize.curve_fit(raman, xr, yr, p0=p0, maxfev=20000)
out["raman"] = {"x": xr.tolist(), "y": yr.tolist(), "p0": p0, "p": [r(v) for v in popt], "se": [r(v) for v in np.sqrt(np.diag(pcov))],
                "ssr": r(np.sum((yr - raman(xr, *popt)) ** 2))}

# Root, minimize, quad, spline
out["brentq"] = r(optimize.brentq(lambda x: x ** 3 - 2 * x - 5, 2, 3, xtol=1e-15))
out["quad"] = {
    "exp_sq_0_1": r(integrate.quad(lambda x: np.exp(-x * x), 0, 1, epsabs=1e-13, epsrel=1e-13)[0]),
    "exp_sq_inf": r(integrate.quad(lambda x: np.exp(-x * x), -np.inf, np.inf, epsabs=1e-13, epsrel=1e-13)[0]),
    "lorentz_0_inf": r(integrate.quad(lambda x: 1 / (1 + x * x), 0, np.inf, epsabs=1e-13, epsrel=1e-13)[0]),
    "sin_0_pi": r(integrate.quad(np.sin, 0, np.pi)[0]),
}
xs = [0, 1, 2.5, 3, 4.5, 6]
ys = [1, 3, 2, 4, 3.5, 0]
cs = interpolate.CubicSpline(xs, ys, bc_type="natural")
at = [0.5, 1, 2.2, 3.7, 5.9, -0.5, 7]
out["spline"] = {"xs": xs, "ys": ys, "at": at, "want": [r(v) for v in cs(at)], "lin": [r(v) for v in np.interp(at[:5], xs, ys)]}
out["trapz"] = r(np.trapezoid(ys, xs))

# t tests
a = [5.1, 4.9, 5.6, 5.8, 6.0, 5.3, 5.7, 6.1, 4.8, 5.5]
bb = [4.2, 4.8, 5.0, 4.4, 5.1, 4.9, 4.0, 4.6]
c = [5.0, 5.2, 5.1, 5.9, 5.7, 5.0, 5.9, 6.0, 4.5, 5.6]
def tt(res, ci):
    return {"t": r(res.statistic), "p": r(res.pvalue), "df": r(res.df), "lo": r(ci.low), "hi": r(ci.high)}
t1 = stats.ttest_1samp(a, 5.0)
t1g = stats.ttest_1samp(a, 5.0, alternative="greater")
tw = stats.ttest_ind(a, bb, equal_var=False)
ts = stats.ttest_ind(a, bb, equal_var=True)
tp = stats.ttest_rel(a, c)
tl = stats.ttest_rel(a, c, alternative="less")
out["ttest"] = {"a": a, "b": bb, "c": c,
    "one": tt(t1, t1.confidence_interval(0.95)), "oneGreater": tt(t1g, t1g.confidence_interval(0.95)),
    "welch": tt(tw, tw.confidence_interval(0.95)), "student": tt(ts, ts.confidence_interval(0.90)),
    "paired": tt(tp, tp.confidence_interval(0.95)), "pairedLess": tt(tl, tl.confidence_interval(0.95))}
from statsmodels.stats.proportion import proportion_confint
lo, hi = proportion_confint(17, 60, alpha=0.05, method="wilson")
out["wilson"] = {"k": 17, "n": 60, "lo": r(lo), "hi": r(hi)}

json.dump(out, sys.stdout, indent=1)
