// Package cp is a finite-domain constraint solver: integer variables with
// explicit domains, propagators run to a fixpoint, and depth-first search
// with branch and bound for an objective. It does not depend on the runtime.
package cp

import "math/bits"

// dom is a set of integers in [base, base+64*len(w)), as a bitset, with
// cached bounds and size. A dom is never changed after it is shared: the
// store copies it before a change (copy on write, undone by the trail).
type dom struct {
	base   int64
	w      []uint64
	lo, hi int64
	size   int
}

func newDom(lo, hi int64) *dom {
	n := (hi - lo + 64) / 64
	d := &dom{base: lo, w: make([]uint64, n), lo: lo, hi: hi, size: int(hi - lo + 1)}
	for i := range d.w {
		d.w[i] = ^uint64(0)
	}
	// Clear the bits past hi in the last word.
	if extra := int(int64(n)*64 - (hi - lo + 1)); extra > 0 {
		d.w[n-1] >>= uint(extra)
	}
	return d
}

func newDomValues(vals []int64) *dom {
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo, hi = min(lo, v), max(hi, v)
	}
	d := &dom{base: lo, w: make([]uint64, (hi-lo+64)/64)}
	for _, v := range vals {
		o := v - lo
		d.w[o/64] |= 1 << uint(o%64)
	}
	d.recount()
	return d
}

func (d *dom) clone() *dom {
	c := *d
	c.w = append([]uint64(nil), d.w...)
	return &c
}

func (d *dom) has(v int64) bool {
	if v < d.lo || v > d.hi {
		return false
	}
	o := v - d.base
	return d.w[o/64]&(1<<uint(o%64)) != 0
}

func (d *dom) fixed() bool { return d.size == 1 }

// recount recomputes lo, hi and size from the bits; size 0 is a wipeout.
func (d *dom) recount() {
	d.size = 0
	first, last := -1, -1
	for i, x := range d.w {
		if x != 0 {
			d.size += bits.OnesCount64(x)
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if d.size == 0 {
		return
	}
	d.lo = d.base + int64(first)*64 + int64(bits.TrailingZeros64(d.w[first]))
	d.hi = d.base + int64(last)*64 + 63 - int64(bits.LeadingZeros64(d.w[last]))
}

// The mutators below change d in place and report whether it changed.
// Callers own d (the store has copied it).

func (d *dom) remove(v int64) bool {
	if !d.has(v) {
		return false
	}
	o := v - d.base
	d.w[o/64] &^= 1 << uint(o%64)
	d.size--
	if d.size > 0 && (v == d.lo || v == d.hi) {
		d.recount()
	}
	return true
}

// setMin removes every value below v.
func (d *dom) setMin(v int64) bool {
	if v <= d.lo {
		return false
	}
	if v > d.hi {
		d.size = 0
		return true
	}
	o := v - d.base
	for i := int64(0); i < o/64; i++ {
		d.w[i] = 0
	}
	d.w[o/64] &^= (1 << uint(o%64)) - 1
	d.recount()
	return true
}

// setMax removes every value above v.
func (d *dom) setMax(v int64) bool {
	if v >= d.hi {
		return false
	}
	if v < d.lo {
		d.size = 0
		return true
	}
	o := v - d.base
	for i := o/64 + 1; i < int64(len(d.w)); i++ {
		d.w[i] = 0
	}
	if s := uint(o%64) + 1; s < 64 {
		d.w[o/64] &= (1 << s) - 1
	}
	d.recount()
	return true
}

// keepOnly intersects d with the values for which keep is true.
func (d *dom) keepOnly(keep func(int64) bool) bool {
	changed := false
	d.each(func(v int64) {
		if !keep(v) {
			o := v - d.base
			d.w[o/64] &^= 1 << uint(o%64)
			changed = true
		}
	})
	if changed {
		d.recount()
	}
	return changed
}

// each calls f for every value, ascending. f must not change d.
func (d *dom) each(f func(int64)) {
	if d.size == 0 {
		return
	}
	first := (d.lo - d.base) / 64
	last := (d.hi - d.base) / 64
	for i := first; i <= last; i++ {
		x := d.w[i]
		for x != 0 {
			t := bits.TrailingZeros64(x)
			f(d.base + i*64 + int64(t))
			x &= x - 1
		}
	}
}

// next returns the smallest value > v, or ok false.
func (d *dom) next(v int64) (int64, bool) {
	for u := max(v+1, d.lo); u <= d.hi; u++ {
		if d.has(u) {
			return u, true
		}
	}
	return 0, false
}
