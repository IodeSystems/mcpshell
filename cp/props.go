package cp

import "sort"

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func ceilDiv(a, b int64) int64 { return -floorDiv(-a, b) }

// --- linear ---

// linLE is sum(a[i]*x[i]) <= c, bounds consistent.
type linLE struct {
	x []int
	a []int64
	c int64
}

func (p *linLE) vars() []int { return p.x }

func termMin(s *store, a int64, v int) int64 {
	if a > 0 {
		return a * s.lo(v)
	}
	return a * s.hi(v)
}

func (p *linLE) propagate(s *store) bool {
	var minSum int64
	for i, v := range p.x {
		minSum += termMin(s, p.a[i], v)
	}
	if minSum > p.c {
		return false
	}
	for i, v := range p.x {
		a := p.a[i]
		if a == 0 {
			continue
		}
		slack := p.c - (minSum - termMin(s, a, v)) // a*x <= slack
		if a > 0 {
			if !s.setMax(v, floorDiv(slack, a)) {
				return false
			}
		} else if !s.setMin(v, ceilDiv(slack, a)) {
			return false
		}
	}
	return true
}

// linNE is sum(a[i]*x[i]) != c: it acts once at most one variable is free.
type linNE struct {
	x []int
	a []int64
	c int64
}

func (p *linNE) vars() []int { return p.x }

func (p *linNE) propagate(s *store) bool {
	free := -1
	var sum int64
	for i, v := range p.x {
		if p.a[i] == 0 {
			continue
		}
		if !s.fixed(v) {
			if free >= 0 {
				return true
			}
			free = i
			continue
		}
		sum += p.a[i] * s.lo(v)
	}
	if free < 0 {
		return sum != p.c
	}
	rest := p.c - sum
	if rest%p.a[free] == 0 {
		return s.remove(p.x[free], rest/p.a[free])
	}
	return true
}

// --- allDifferent: value elimination plus Hall intervals on bounds ---

type allDiff struct{ x []int }

func (p *allDiff) vars() []int { return p.x }

func (p *allDiff) propagate(s *store) bool {
	for changed := true; changed; {
		changed = false
		for i, v := range p.x {
			if !s.fixed(v) {
				continue
			}
			val := s.lo(v)
			for j, w := range p.x {
				if j != i && s.has(w, val) {
					if !s.remove(w, val) {
						return false
					}
					changed = true
				}
			}
		}
	}
	return p.hall(s)
}

// hall: for every interval [a, b] of values, the variables whose domains lie
// inside it number at most b-a+1; when equal, the others leave [a, b]. Only
// interval ends that are domain bounds matter. O(n^3) on n variables, which
// is fine up to 60 variables; above that only value elimination runs.
func (p *allDiff) hall(s *store) bool {
	n := len(p.x)
	if n < 2 || n > 60 {
		return true
	}
	los := make([]int64, 0, n)
	his := make([]int64, 0, n)
	for _, v := range p.x {
		los = append(los, s.lo(v))
		his = append(his, s.hi(v))
	}
	for _, a := range los {
		for _, b := range his {
			if b < a {
				continue
			}
			inside := 0
			for _, v := range p.x {
				if s.lo(v) >= a && s.hi(v) <= b {
					inside++
				}
			}
			width := b - a + 1
			if int64(inside) > width {
				return false
			}
			if int64(inside) < width {
				continue
			}
			for _, v := range p.x {
				if s.lo(v) >= a && s.hi(v) <= b {
					continue
				}
				if s.lo(v) >= a && s.lo(v) <= b && !s.setMin(v, b+1) {
					return false
				}
				if s.hi(v) >= a && s.hi(v) <= b && !s.setMax(v, a-1) {
					return false
				}
			}
		}
	}
	return true
}

// --- element: val == arr[idx] ---

type element struct {
	idx, val int
	arr      []int64
}

func (p *element) vars() []int { return []int{p.idx, p.val} }

func (p *element) propagate(s *store) bool {
	if !s.setMin(p.idx, 0) || !s.setMax(p.idx, int64(len(p.arr)-1)) {
		return false
	}
	if !s.keepOnly(p.idx, func(i int64) bool { return s.has(p.val, p.arr[i]) }) {
		return false
	}
	ok := map[int64]bool{}
	s.doms[p.idx].each(func(i int64) { ok[p.arr[i]] = true })
	return s.keepOnly(p.val, func(v int64) bool { return ok[v] })
}

// --- table: generalized arc consistency by scanning the tuples ---

type table struct {
	x      []int
	tuples [][]int64
}

func (p *table) vars() []int { return p.x }

func (p *table) propagate(s *store) bool {
	support := make([]map[int64]bool, len(p.x))
	for i := range support {
		support[i] = map[int64]bool{}
	}
	found := false
tuples:
	for _, t := range p.tuples {
		for i, v := range p.x {
			if !s.has(v, t[i]) {
				continue tuples
			}
		}
		found = true
		for i := range p.x {
			support[i][t[i]] = true
		}
	}
	if !found {
		return false
	}
	for i, v := range p.x {
		sup := support[i]
		if !s.keepOnly(v, func(x int64) bool { return sup[x] }) {
			return false
		}
	}
	return true
}

// --- count: #{i : x[i] == value} op rhs ---

type count struct {
	x     []int
	value int64
	op    Op
	rhs   int64
}

func (p *count) vars() []int { return p.x }

func (p *count) propagate(s *store) bool {
	sure, maybe := 0, 0
	for _, v := range p.x {
		if s.has(v, p.value) {
			if s.fixed(v) {
				sure++
			} else {
				maybe++
			}
		}
	}
	lo, hi := int64(sure), int64(sure+maybe)
	wantLo, wantHi := int64(-1)<<62, int64(1)<<62
	switch p.op {
	case LE:
		wantHi = p.rhs
	case GE:
		wantLo = p.rhs
	case EQ:
		wantLo, wantHi = p.rhs, p.rhs
	}
	if lo > wantHi || hi < wantLo {
		return false
	}
	if lo == wantHi && maybe > 0 { // no more may take the value
		for _, v := range p.x {
			if !s.fixed(v) && !s.remove(v, p.value) {
				return false
			}
		}
	} else if hi == wantLo && maybe > 0 { // every candidate must take it
		for _, v := range p.x {
			if s.has(v, p.value) && !s.fix(v, p.value) {
				return false
			}
		}
	}
	return true
}

// --- max / min: t == max(x) (min is max of the negation, done directly) ---

type maxOf struct {
	t   int
	x   []int
	min bool
}

func (p *maxOf) vars() []int { return append([]int{p.t}, p.x...) }

func (p *maxOf) propagate(s *store) bool {
	if p.min {
		return p.propMin(s)
	}
	maxLo, maxHi := int64(-1)<<62, int64(-1)<<62
	for _, v := range p.x {
		maxLo, maxHi = max(maxLo, s.lo(v)), max(maxHi, s.hi(v))
	}
	if !s.setMin(p.t, maxLo) || !s.setMax(p.t, maxHi) {
		return false
	}
	for _, v := range p.x {
		if !s.setMax(v, s.hi(p.t)) {
			return false
		}
	}
	// t must be reached: if one variable alone can reach t's lower bound,
	// it is at least that.
	reach, who := 0, -1
	for _, v := range p.x {
		if s.hi(v) >= s.lo(p.t) {
			reach++
			who = v
		}
	}
	if reach == 0 {
		return false
	}
	if reach == 1 && !s.setMin(who, s.lo(p.t)) {
		return false
	}
	return true
}

func (p *maxOf) propMin(s *store) bool {
	minLo, minHi := int64(1)<<62, int64(1)<<62
	for _, v := range p.x {
		minLo, minHi = min(minLo, s.lo(v)), min(minHi, s.hi(v))
	}
	if !s.setMin(p.t, minLo) || !s.setMax(p.t, minHi) {
		return false
	}
	for _, v := range p.x {
		if !s.setMin(v, s.lo(p.t)) {
			return false
		}
	}
	reach, who := 0, -1
	for _, v := range p.x {
		if s.lo(v) <= s.hi(p.t) {
			reach++
			who = v
		}
	}
	if reach == 0 {
		return false
	}
	if reach == 1 && !s.setMax(who, s.hi(p.t)) {
		return false
	}
	return true
}

// --- cumulative: time-table propagation on compulsory parts ---

type cumulative struct {
	tasks []Task
	cap   int64
	xs    []int
}

func newCumulative(tasks []Task, capacity int64) *cumulative {
	p := &cumulative{cap: capacity}
	for _, t := range tasks {
		if t.Dur > 0 && t.Demand > 0 {
			p.tasks = append(p.tasks, t)
			p.xs = append(p.xs, t.Start)
		}
	}
	return p
}

func (p *cumulative) vars() []int { return p.xs }

type seg struct {
	a, b int64 // [a, b)
	h    int64
}

// profile is the demand of the compulsory parts [lst, ect), as segments of
// constant height > 0, ascending.
func (p *cumulative) profile(s *store) []seg {
	type ev struct {
		t int64
		d int64
	}
	var evs []ev
	for _, t := range p.tasks {
		lst, ect := s.hi(t.Start), s.lo(t.Start)+t.Dur
		if lst < ect {
			evs = append(evs, ev{lst, t.Demand}, ev{ect, -t.Demand})
		}
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].t < evs[j].t })
	var segs []seg
	var h int64
	for i := 0; i < len(evs); {
		t := evs[i].t
		for i < len(evs) && evs[i].t == t {
			h += evs[i].d
			i++
		}
		if i < len(evs) && h > 0 {
			segs = append(segs, seg{t, evs[i].t, h})
		}
	}
	return segs
}

func (p *cumulative) propagate(s *store) bool {
	segs := p.profile(s)
	for _, g := range segs {
		if g.h > p.cap {
			return false
		}
	}
	for _, t := range p.tasks {
		if t.Demand > p.cap {
			return false
		}
		// The task's own compulsory part, removed from the profile height.
		lst, ect := s.hi(t.Start), s.lo(t.Start)+t.Dur
		own := func(g seg) int64 {
			if lst < ect && g.a < ect && lst < g.b {
				return t.Demand
			}
			return 0
		}
		// Push the earliest start past segments it cannot overlap.
		for moved := true; moved; {
			moved = false
			est := s.lo(t.Start)
			for _, g := range segs {
				if g.h-own(g)+t.Demand > p.cap && est < g.b && est+t.Dur > g.a {
					if !s.setMin(t.Start, g.b) {
						return false
					}
					moved = true
					break
				}
			}
		}
		// Pull the latest start before segments it cannot overlap.
		for moved := true; moved; {
			moved = false
			lst := s.hi(t.Start)
			for i := len(segs) - 1; i >= 0; i-- {
				g := segs[i]
				if g.h-own(g)+t.Demand > p.cap && lst < g.b && lst+t.Dur > g.a {
					if !s.setMax(t.Start, g.a-t.Dur) {
						return false
					}
					moved = true
					break
				}
			}
		}
	}
	return true
}

// --- noOverlap: cumulative with capacity 1, plus one order variable per
// pair of tasks (0: a before b, 1: b before a). Search branches on the order
// variables first, which splits the space by sequence instead of by time. ---

func newNoOverlap(tasks []Task) (*cumulative, [][2]Task) {
	var unit []Task
	for _, t := range tasks {
		if t.Dur > 0 {
			unit = append(unit, Task{t.Start, t.Dur, 1})
		}
	}
	var pairs [][2]Task
	for i := range unit {
		for j := i + 1; j < len(unit); j++ {
			pairs = append(pairs, [2]Task{unit[i], unit[j]})
		}
	}
	return newCumulative(unit, 1), pairs
}

// order links a pair's order variable o to their start times.
type order struct {
	a, b Task
	o    int
}

func (p *order) vars() []int { return []int{p.a.Start, p.b.Start, p.o} }

func (p *order) propagate(s *store) bool {
	a, b := p.a, p.b
	if !s.fixed(p.o) {
		aFirst := s.lo(a.Start)+a.Dur <= s.hi(b.Start)
		bFirst := s.lo(b.Start)+b.Dur <= s.hi(a.Start)
		switch {
		case !aFirst && !bFirst:
			return false
		case !aFirst:
			return s.fix(p.o, 1)
		case !bFirst:
			return s.fix(p.o, 0)
		}
		return true
	}
	if s.lo(p.o) == 1 {
		a, b = b, a
	}
	// a.start + a.dur <= b.start
	return s.setMin(b.Start, s.lo(a.Start)+a.Dur) && s.setMax(a.Start, s.hi(b.Start)-a.Dur)
}

// --- circuit: one Hamiltonian cycle over successor variables ---

type circuit struct {
	x  []int
	ad *allDiff
}

func (p *circuit) vars() []int { return p.x }

func (p *circuit) propagate(s *store) bool {
	n := len(p.x)
	if n == 1 {
		return s.fix(p.x[0], 0)
	}
	for i, v := range p.x {
		if !s.setMin(v, 0) || !s.setMax(v, int64(n-1)) || !s.remove(v, int64(i)) {
			return false
		}
	}
	if !p.ad.propagate(s) {
		return false
	}
	// Follow fixed links from every chain start; the chain's end may not
	// close back to the start unless the chain covers all nodes.
	hasPred := make([]bool, n)
	for _, v := range p.x {
		if s.fixed(v) {
			hasPred[s.lo(v)] = true
		}
	}
	visited := 0
	for start := 0; start < n; start++ {
		if hasPred[start] {
			continue
		}
		end, length := start, 1
		for s.fixed(p.x[end]) {
			end = int(s.lo(p.x[end]))
			length++
			if length > n {
				return false
			}
		}
		visited += length
		if length < n && !s.remove(p.x[end], int64(start)) {
			return false
		}
	}
	// Every node has a fixed predecessor in a set of closed cycles: they
	// must form one cycle of length n.
	if visited == 0 {
		cur, length := 0, 0
		for {
			cur = int(s.lo(p.x[cur]))
			length++
			if cur == 0 {
				break
			}
		}
		return length == n
	}
	// Nodes not on any chain from a start lie on closed fixed cycles.
	return visited >= n
}

// --- edge finding for a disjunctive resource ---

// edgeFind: for every window [e, L] given by an earliest start e and a latest
// completion L, the tasks inside it (est >= e, lct <= L) need p(set) time:
// e + p > L is an overload. A task i outside the set with
// min(e, est_i) + p + p_i > L cannot end before the set does, so it starts
// after the set: est_i >= e + p. The mirror rule bounds latest starts.
// O(n^3) per call; skipped above 60 tasks (time-tabling still runs).
type edgeFind struct {
	tasks []Task
	xs    []int
}

func newEdgeFind(tasks []Task) *edgeFind {
	p := &edgeFind{}
	for _, t := range tasks {
		if t.Dur > 0 {
			p.tasks = append(p.tasks, t)
			p.xs = append(p.xs, t.Start)
		}
	}
	return p
}

func (p *edgeFind) vars() []int { return p.xs }

func (p *edgeFind) propagate(s *store) bool {
	n := len(p.tasks)
	if n < 2 || n > 60 {
		return true
	}
	est := func(t Task) int64 { return s.lo(t.Start) }
	lct := func(t Task) int64 { return s.hi(t.Start) + t.Dur }
	for _, tk := range p.tasks {
		L := lct(tk)
		for _, te := range p.tasks {
			e := est(te)
			var sum int64
			in := make([]bool, n)
			for i, t := range p.tasks {
				if est(t) >= e && lct(t) <= L {
					in[i] = true
					sum += t.Dur
				}
			}
			if sum == 0 {
				continue
			}
			if e+sum > L {
				return false
			}
			for i, t := range p.tasks {
				if in[i] {
					continue
				}
				if min(e, est(t))+sum+t.Dur > L && !s.setMin(t.Start, e+sum) {
					return false
				}
			}
		}
	}
	// Mirror: windows [E, l] by latest completion l and earliest start E; a
	// task that cannot start after the set must end before it.
	for _, te := range p.tasks {
		E := est(te)
		for _, tk := range p.tasks {
			l := lct(tk)
			var sum int64
			in := make([]bool, n)
			for i, t := range p.tasks {
				if est(t) >= E && lct(t) <= l {
					in[i] = true
					sum += t.Dur
				}
			}
			if sum == 0 {
				continue
			}
			if E+sum > l {
				return false
			}
			for i, t := range p.tasks {
				if in[i] {
					continue
				}
				if max(l, lct(t))-sum-t.Dur < E && !s.setMax(t.Start, l-sum-t.Dur) {
					return false
				}
			}
		}
	}
	return true
}
