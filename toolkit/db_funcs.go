package toolkit

import (
	"database/sql/driver"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"modernc.org/sqlite"
)

// Decimal helpers and SQL functions of the Db toolkit. Everything here is
// registered on the toolkit's private driver (sqlite.Driver.RegisterFunction
// reaches only connections that driver opens), so the --sql toolkit and any
// other SQLite handle in the process are not affected.

const (
	dbDecimalType = "DECIMAL_TEXT" // declared type of an exact column: TEXT affinity, parsed exactly on the way out
	dbCollation   = "DECIMAL"      // collation of an exact column: compares by numeric value
)

var (
	// decimalRE is a plain decimal: no hex, no "1/3", no inf/nan.
	decimalRE = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)
	// canonicalDecRE is the form dbCanonical writes: no sign "+", no leading or trailing zeros.
	canonicalDecRE = regexp.MustCompile(`^-?(0|[1-9]\d*)(\.\d*[1-9])?$`)
)

// parseDecimal parses a decimal string exactly.
func parseDecimal(s string) (*big.Rat, bool) {
	s = strings.TrimSpace(s)
	if !decimalRE.MatchString(s) {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}

// ratDecimal writes r as a plain decimal when r terminates (denominator has
// only the factors 2 and 5), else ok is false.
func ratDecimal(r *big.Rat) (string, bool) {
	den := new(big.Int).Set(r.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	var twos, fives int
	m := new(big.Int)
	for {
		q, rem := new(big.Int).QuoRem(den, two, m)
		if rem.Sign() != 0 {
			break
		}
		den, twos = q, twos+1
	}
	for {
		q, rem := new(big.Int).QuoRem(den, five, m)
		if rem.Sign() != 0 {
			break
		}
		den, fives = q, fives+1
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		return "", false
	}
	s := r.FloatString(max(twos, fives))
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s, true
}

// floatRat converts a float64 by its shortest decimal form, the same form the
// boundary writes it in, so a REAL 0.1 is the decimal 0.1.
func floatRat(f float64) (*big.Rat, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%v is not a finite number", f)
	}
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(f, 'g', -1, 64))
	return r, nil
}

// dbRat reads a SQL argument as an exact number. NULL is the caller's check.
func dbRat(fn string, v driver.Value) (*big.Rat, error) {
	switch x := v.(type) {
	case int64:
		return new(big.Rat).SetInt64(x), nil
	case float64:
		r, err := floatRat(x)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		return r, nil
	case string:
		if r, ok := parseDecimal(x); ok {
			return r, nil
		}
		return nil, fmt.Errorf("%s: %q is not a number", fn, x)
	case []byte:
		if r, ok := parseDecimal(string(x)); ok {
			return r, nil
		}
		return nil, fmt.Errorf("%s: a blob is not a number", fn)
	}
	return nil, fmt.Errorf("%s: %v is not a number", fn, v)
}

// decResult returns r in the form that reads back exactly: an integer, a REAL
// when its shortest form is r, else the decimal as a blob (the boundary reads a
// blob as an exact decimal). A non-terminating value (1/3) is a float64.
func decResult(r *big.Rat) driver.Value {
	if r.IsInt() {
		if r.Num().IsInt64() {
			return r.Num().Int64()
		}
		return []byte(r.Num().String())
	}
	f, _ := r.Float64()
	if back, err := floatRat(f); err == nil && back.Cmp(r) == 0 {
		return f
	}
	if s, ok := ratDecimal(r); ok {
		return []byte(s)
	}
	return f
}

// ratSqrt returns sqrt(r) exactly when r is a perfect square of a rational,
// else a float64 (r must be >= 0).
func ratSqrt(r *big.Rat) driver.Value {
	n := new(big.Int).Sqrt(r.Num())
	d := new(big.Int).Sqrt(r.Denom())
	if new(big.Int).Mul(n, n).Cmp(r.Num()) == 0 && new(big.Int).Mul(d, d).Cmp(r.Denom()) == 0 {
		return decResult(new(big.Rat).SetFrac(n, d))
	}
	f, _ := new(big.Float).SetPrec(200).SetRat(r).Sqrt(new(big.Float).SetPrec(200).SetRat(r)).Float64()
	return f
}

// dbAgg adapts closures to sqlite.AggregateFunction. inverse is the window
// removal of the oldest row.
type dbAgg struct {
	step, inverse func(args []driver.Value) error
	value         func() (driver.Value, error)
}

func (a *dbAgg) Step(_ *sqlite.FunctionContext, args []driver.Value) error { return a.step(args) }
func (a *dbAgg) WindowInverse(_ *sqlite.FunctionContext, args []driver.Value) error {
	return a.inverse(args)
}
func (a *dbAgg) WindowValue(*sqlite.FunctionContext) (driver.Value, error) { return a.value() }
func (a *dbAgg) Final(*sqlite.FunctionContext)                             {}

func hasNull(args []driver.Value) bool {
	for _, a := range args {
		if a == nil {
			return true
		}
	}
	return false
}

// regAgg registers an aggregate; make builds the state of one evaluation.
func regAgg(d *sqlite.Driver, name string, nargs int32, make func() *dbAgg) error {
	return d.RegisterFunction(name, &sqlite.FunctionImpl{
		NArgs: nargs, Deterministic: true,
		MakeAggregate: func(sqlite.FunctionContext) (sqlite.AggregateFunction, error) { return make(), nil },
	})
}

// sumAgg: sum and avg. Exact, with no integer overflow.
func sumAgg(name string, avg bool) func() *dbAgg {
	return func() *dbAgg {
		sum, n := new(big.Rat), 0
		upd := func(sign int) func(args []driver.Value) error {
			return func(args []driver.Value) error {
				if args[0] == nil {
					return nil
				}
				r, err := dbRat(name, args[0])
				if err != nil {
					return err
				}
				if sign > 0 {
					sum.Add(sum, r)
				} else {
					sum.Sub(sum, r)
				}
				n += sign
				return nil
			}
		}
		return &dbAgg{step: upd(1), inverse: upd(-1), value: func() (driver.Value, error) {
			if n == 0 {
				return nil, nil
			}
			if avg {
				return decResult(new(big.Rat).Quo(sum, new(big.Rat).SetInt64(int64(n)))), nil
			}
			return decResult(sum), nil
		}}
	}
}

// momentsAgg: variance, stdev and their population forms.
func momentsAgg(name string, pop, root bool) func() *dbAgg {
	return func() *dbAgg {
		n := 0
		sx, sxx := new(big.Rat), new(big.Rat)
		upd := func(sign int64) func(args []driver.Value) error {
			return func(args []driver.Value) error {
				if args[0] == nil {
					return nil
				}
				r, err := dbRat(name, args[0])
				if err != nil {
					return err
				}
				k := new(big.Rat).SetInt64(sign)
				sx.Add(sx, new(big.Rat).Mul(k, r))
				sxx.Add(sxx, new(big.Rat).Mul(k, new(big.Rat).Mul(r, r)))
				n += int(sign)
				return nil
			}
		}
		return &dbAgg{step: upd(1), inverse: upd(-1), value: func() (driver.Value, error) {
			if n < 1 || (!pop && n < 2) {
				return nil, nil
			}
			// (sxx - sx^2/n) / (n or n-1)
			nn := new(big.Rat).SetInt64(int64(n))
			v := new(big.Rat).Sub(sxx, new(big.Rat).Quo(new(big.Rat).Mul(sx, sx), nn))
			div := nn
			if !pop {
				div = new(big.Rat).SetInt64(int64(n - 1))
			}
			v.Quo(v, div)
			if root {
				return ratSqrt(v), nil
			}
			return decResult(v), nil
		}}
	}
}

// pairAgg: covar, covar_pop, corr. A row with a NULL in either column is skipped.
func pairAgg(name string, kind string) func() *dbAgg {
	return func() *dbAgg {
		n := 0
		sx, sy, sxx, syy, sxy := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
		upd := func(sign int64) func(args []driver.Value) error {
			return func(args []driver.Value) error {
				if hasNull(args) {
					return nil
				}
				x, err := dbRat(name, args[0])
				if err != nil {
					return err
				}
				y, err := dbRat(name, args[1])
				if err != nil {
					return err
				}
				k := new(big.Rat).SetInt64(sign)
				add := func(acc, v *big.Rat) { acc.Add(acc, new(big.Rat).Mul(k, v)) }
				add(sx, x)
				add(sy, y)
				add(sxx, new(big.Rat).Mul(x, x))
				add(syy, new(big.Rat).Mul(y, y))
				add(sxy, new(big.Rat).Mul(x, y))
				n += int(sign)
				return nil
			}
		}
		return &dbAgg{step: upd(1), inverse: upd(-1), value: func() (driver.Value, error) {
			if n < 2 && !(kind == "covar_pop" && n == 1) {
				return nil, nil
			}
			nn := new(big.Rat).SetInt64(int64(n))
			// n*sxy - sx*sy, scaled by n^2 relative to the covariance numerator
			cxy := new(big.Rat).Sub(new(big.Rat).Mul(nn, sxy), new(big.Rat).Mul(sx, sy))
			switch kind {
			case "covar_pop":
				return decResult(cxy.Quo(cxy, new(big.Rat).Mul(nn, nn))), nil
			case "covar":
				return decResult(cxy.Quo(cxy, new(big.Rat).Mul(nn, new(big.Rat).SetInt64(int64(n-1))))), nil
			}
			cxx := new(big.Rat).Sub(new(big.Rat).Mul(nn, sxx), new(big.Rat).Mul(sx, sx))
			cyy := new(big.Rat).Sub(new(big.Rat).Mul(nn, syy), new(big.Rat).Mul(sy, sy))
			den := new(big.Rat).Mul(cxx, cyy)
			if den.Sign() <= 0 {
				return nil, nil // a constant column has no correlation
			}
			root := ratSqrt(den)
			if rv, ok := dbValueRat(root); ok {
				return decResult(new(big.Rat).Quo(cxy, rv)), nil
			}
			fd := new(big.Float).SetPrec(200).SetRat(den)
			q := new(big.Float).SetPrec(200).Quo(new(big.Float).SetPrec(200).SetRat(cxy), fd.Sqrt(fd))
			f, _ := q.Float64()
			return math.Max(-1, math.Min(1, f)), nil
		}}
	}
}

// dbValueRat reads back an exact driver value made by decResult / ratSqrt.
func dbValueRat(v driver.Value) (*big.Rat, bool) {
	switch x := v.(type) {
	case int64:
		return new(big.Rat).SetInt64(x), true
	case []byte:
		return parseDecimal(string(x))
	case float64:
		if r, err := floatRat(x); err == nil {
			return r, true
		}
	}
	return nil, false
}

// quantileAgg: median and quantile(x, p). Linear interpolation between the two
// nearest ranks (R type 7, the spreadsheet PERCENTILE). A sorted list, with
// binary search for the window removal.
func quantileAgg(name string, fixedP *big.Rat) func() *dbAgg {
	return func() *dbAgg {
		var xs []*big.Rat
		var p = fixedP
		find := func(r *big.Rat) int { return sort.Search(len(xs), func(i int) bool { return xs[i].Cmp(r) >= 0 }) }
		return &dbAgg{
			step: func(args []driver.Value) error {
				if fixedP == nil && len(args) > 1 && args[1] != nil && p == nil {
					pr, err := dbRat(name, args[1])
					if err != nil {
						return err
					}
					if pr.Sign() < 0 || pr.Cmp(big.NewRat(1, 1)) > 0 {
						return fmt.Errorf("%s: p must be between 0 and 1, got %s", name, pr.RatString())
					}
					p = pr
				}
				if args[0] == nil {
					return nil
				}
				r, err := dbRat(name, args[0])
				if err != nil {
					return err
				}
				i := find(r)
				xs = append(xs, nil)
				copy(xs[i+1:], xs[i:])
				xs[i] = r
				return nil
			},
			inverse: func(args []driver.Value) error {
				if args[0] == nil {
					return nil
				}
				r, err := dbRat(name, args[0])
				if err != nil {
					return err
				}
				if i := find(r); i < len(xs) && xs[i].Cmp(r) == 0 {
					xs = append(xs[:i], xs[i+1:]...)
				}
				return nil
			},
			value: func() (driver.Value, error) {
				if len(xs) == 0 || p == nil {
					return nil, nil
				}
				// h = (n-1)p, value = x[floor h] + (h - floor h)(x[floor h + 1] - x[floor h])
				h := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(len(xs)-1)), p)
				lo := new(big.Int).Quo(h.Num(), h.Denom()).Int64()
				frac := new(big.Rat).Sub(h, new(big.Rat).SetInt64(lo))
				v := new(big.Rat).Set(xs[lo])
				if frac.Sign() > 0 {
					d := new(big.Rat).Sub(xs[lo+1], xs[lo])
					v.Add(v, d.Mul(d, frac))
				}
				return decResult(v), nil
			},
		}
	}
}

// modeAgg: the most frequent value; the smallest on a tie. Numbers (including
// decimal text) that are equal in value count as one.
func modeAgg() *dbAgg {
	type entry struct {
		count int
		v     driver.Value
		r     *big.Rat // nil for non-numeric text
	}
	m := map[string]*entry{}
	key := func(v driver.Value) (string, *big.Rat) {
		switch x := v.(type) {
		case int64, float64:
			r, err := dbRat("mode", x)
			if err == nil {
				return "n:" + r.RatString(), r
			}
		case string:
			if canonicalDecRE.MatchString(x) {
				r, _ := parseDecimal(x)
				return "n:" + r.RatString(), r
			}
			return "s:" + x, nil
		case []byte:
			if r, ok := parseDecimal(string(x)); ok {
				return "n:" + r.RatString(), r
			}
		}
		return fmt.Sprintf("o:%v", v), nil
	}
	return &dbAgg{
		step: func(args []driver.Value) error {
			if args[0] == nil {
				return nil
			}
			k, r := key(args[0])
			if e := m[k]; e != nil {
				e.count++
			} else {
				m[k] = &entry{1, args[0], r}
			}
			return nil
		},
		inverse: func(args []driver.Value) error {
			if args[0] == nil {
				return nil
			}
			k, _ := key(args[0])
			if e := m[k]; e != nil {
				if e.count--; e.count == 0 {
					delete(m, k)
				}
			}
			return nil
		},
		value: func() (driver.Value, error) {
			var best *entry
			for _, e := range m {
				switch {
				case best == nil, e.count > best.count:
					best = e
				case e.count == best.count:
					// tie: numbers before text, then ascending
					if (e.r != nil && best.r == nil) ||
						(e.r != nil && best.r != nil && e.r.Cmp(best.r) < 0) ||
						(e.r == nil && best.r == nil && fmt.Sprint(e.v) < fmt.Sprint(best.v)) {
						best = e
					}
				}
			}
			if best == nil {
				return nil, nil
			}
			return best.v, nil
		},
	}
}

// registerDbFunctions installs the exact aggregates, window functions, the
// DECIMAL collation and dmul on the private driver d.
//
// sum and avg override SQLite's own for this driver: the built-ins add in
// float64 (0.1 ten times gives 0.9999999999999999) and sum on TEXT decimals
// would convert them to float64 first.
func registerDbFunctions(d *sqlite.Driver) error {
	for _, a := range []struct {
		name  string
		nargs int32
		make  func() *dbAgg
	}{
		{"sum", 1, sumAgg("sum", false)},
		{"avg", 1, sumAgg("avg", true)},
		{"median", 1, quantileAgg("median", big.NewRat(1, 2))},
		{"quantile", 2, quantileAgg("quantile", nil)},
		{"variance", 1, momentsAgg("variance", false, false)},
		{"var_pop", 1, momentsAgg("var_pop", true, false)},
		{"stdev", 1, momentsAgg("stdev", false, true)},
		{"stdev_pop", 1, momentsAgg("stdev_pop", true, true)},
		{"stddev", 1, momentsAgg("stddev", false, true)},
		{"stddev_pop", 1, momentsAgg("stddev_pop", true, true)},
		{"covar", 2, pairAgg("covar", "covar")},
		{"covar_pop", 2, pairAgg("covar_pop", "covar_pop")},
		{"corr", 2, pairAgg("corr", "corr")},
		{"mode", 1, modeAgg},
	} {
		if err := regAgg(d, a.name, a.nargs, a.make); err != nil {
			return err
		}
	}
	if err := d.RegisterDeterministicScalarFunction("dmul", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if hasNull(args) {
			return nil, nil
		}
		x, err := dbRat("dmul", args[0])
		if err != nil {
			return nil, err
		}
		y, err := dbRat("dmul", args[1])
		if err != nil {
			return nil, err
		}
		return decResult(x.Mul(x, y)), nil
	}); err != nil {
		return err
	}
	return d.RegisterCollationUtf8(dbCollation, func(a, b string) int {
		x, okx := parseDecimal(a)
		y, oky := parseDecimal(b)
		switch {
		case okx && oky:
			return x.Cmp(y)
		case okx: // numbers before non-numbers
			return -1
		case oky:
			return 1
		}
		return strings.Compare(a, b)
	})
}
