package toolkit

import (
	"math"
	"math/big"

	. "github.com/iodesystems/mcpshell/runtime"
)

// dim is one unit dimension of a routing problem (distance, time or load).
// The algorithms run on float64. When every value of a dimension is exact
// (integers, decimals, fractions), finish rescales the values to integers by
// the common denominator, so float64 sums are exact and the reported numbers
// are exact rationals. A float value (haversine, sqrt) makes the dimension
// float64.
type dim struct {
	spans   [][]float64
	rats    map[*float64]*big.Rat // exact values that are not integers
	inexact bool
	scale   *big.Int // nil: float64 values
}

const maxExact = 1 << 53

func isInt(f float64) bool { return f == math.Trunc(f) && math.Abs(f) < maxExact }

// add registers a span; its values are set with set, setRat or setFloat.
func (d *dim) add(span []float64) { d.spans = append(d.spans, span) }

func (d *dim) set(span []float64, i int, v *NumberVal) {
	span[i] = v.V
	if isInt(v.V) {
		return
	}
	if r, ok := v.Exact(); ok {
		d.setRat(span, i, r)
		return
	}
	d.inexact = true
}

func (d *dim) setRat(span []float64, i int, r *big.Rat) {
	f, _ := r.Float64()
	span[i] = f
	if r.IsInt() && isInt(f) {
		return
	}
	if d.rats == nil {
		d.rats = map[*float64]*big.Rat{}
	}
	d.rats[&span[i]] = r
}

func (d *dim) setFloat(span []float64, i int, f float64) {
	span[i] = f
	if !isInt(f) {
		d.inexact = true
	}
}

// exact reports whether finish found an exact integer scale.
func (d *dim) exact() bool { return d.scale != nil }

// finish rescales to integers when every value is exact and terms sums of
// the largest value stay below 2^53. Inf values (no limit) are left alone.
func (d *dim) finish(terms float64) {
	if d.inexact {
		return
	}
	lcm := big.NewInt(1)
	for _, r := range d.rats {
		den := r.Denom()
		g := new(big.Int).GCD(nil, nil, lcm, den)
		lcm.Mul(lcm, new(big.Int).Quo(den, g))
		if lcm.BitLen() > 40 {
			d.inexact = true
			return
		}
	}
	scaled := func(p *float64) float64 {
		if r, ok := d.rats[p]; ok {
			x := new(big.Rat).Mul(r, new(big.Rat).SetInt(lcm))
			f, _ := x.Float64()
			return f
		}
		return *p * float64(lcm.Int64())
	}
	maxAbs := 0.0
	for _, s := range d.spans {
		for i := range s {
			if !math.IsInf(s[i], 0) {
				maxAbs = math.Max(maxAbs, math.Abs(scaled(&s[i])))
			}
		}
	}
	if maxAbs*math.Max(terms, 1) >= maxExact {
		d.inexact = true
		return
	}
	for _, s := range d.spans {
		for i := range s {
			if !math.IsInf(s[i], 0) {
				s[i] = scaled(&s[i])
			}
		}
	}
	d.scale = lcm
	d.rats = nil
}

// out turns an internal value back into a language number: exact when the
// dimension is exact, else float64.
func (d *dim) out(f float64) Value {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return Null
	}
	if d.scale == nil {
		return Num(f)
	}
	r := new(big.Rat)
	if isInt(f) {
		r.SetInt64(int64(f))
	} else {
		r.SetFloat64(f)
	}
	return NumRat(r.Quo(r, new(big.Rat).SetInt(d.scale)))
}
