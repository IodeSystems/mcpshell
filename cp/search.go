package cp

import (
	"context"
	"math"
)

// DefaultLimits returns the defaults.
func DefaultLimits() Limits { return Limits{MaxNodes: 1_000_000, MaxSolutions: 1000} }

func (l Limits) fill() Limits {
	d := DefaultLimits()
	if l.MaxNodes <= 0 {
		l.MaxNodes = d.MaxNodes
	}
	if l.MaxSolutions <= 0 {
		l.MaxSolutions = d.MaxSolutions
	}
	return l
}

type search struct {
	ctx   context.Context
	m     *Model
	lim   Limits
	s     *store
	obj   *linLE // objective in minimization form, <= best-1 once a solution exists
	objK  int
	hiFst []bool // try the largest value first (it lowers the objective)
	first []bool // in SearchFirst
	res   *Result
	have  bool
	best  []int64
	bestV int64 // minimization form
	stop  string
	done  bool
	// orders: the hidden order variables of noOverlap pairs, after the
	// model's variables in the store.
	orders []*order
}

// Solve searches the model. Errors are ErrInvalid only; limits and the
// context end the search with Complete false and Stop set.
func Solve(ctx context.Context, m *Model, lim Limits) (*Result, error) {
	lim = lim.fill()
	doms, err := m.validate()
	if err != nil {
		return nil, err
	}
	if lim.All && m.Objective != nil {
		return nil, invalid("listing all solutions and an objective do not mix; drop one")
	}
	se := &search{ctx: ctx, m: m, lim: lim, s: newStore(doms), res: &Result{}, objK: -1}
	if err := se.post(); err != nil {
		return nil, err
	}
	se.hiFst = make([]bool, len(m.Vars))
	se.first = make([]bool, len(m.Vars))
	for _, j := range m.SearchFirst {
		se.first[j] = true
	}
	if se.obj != nil {
		for i, v := range se.obj.x {
			if se.obj.a[i] < 0 {
				se.hiFst[v] = true
			}
		}
	}

	res := se.res
	se.s.newStamp()
	se.s.enqueueAll()
	if !se.s.fixpoint() {
		res.Status, res.Complete = Infeasible, true
		return res, nil
	}
	var rootLB int64
	if se.obj != nil {
		for i, v := range se.obj.x {
			rootLB += termMin(se.s, se.obj.a[i], v)
		}
	}
	se.dfs()
	if se.stop != "" && se.obj != nil {
		rootLB = se.destructiveBound(rootLB)
		if se.have && rootLB >= se.bestV {
			se.stop = "" // the root alone refutes anything better: proven
		}
	}

	res.Complete = se.stop == ""
	res.Stop = se.stop
	user := func(v int64) int64 {
		if m.Sense == Maximize {
			return -v
		}
		return v
	}
	switch {
	case se.have && res.Complete && se.obj != nil:
		res.Status = Optimal
		res.Objective, res.Bound = user(se.bestV), user(se.bestV)
	case se.have:
		res.Status = Feasible
		if se.obj != nil {
			res.Objective, res.Bound = user(se.bestV), user(rootLB)
		}
	case res.Complete:
		res.Status = Infeasible
	default:
		res.Status = Unknown
		if se.obj != nil {
			res.Bound = user(rootLB)
		}
	}
	res.Values = se.best
	return res, nil
}

// post turns the constraints into propagators.
func (se *search) post() error {
	s := se.s
	for _, c := range se.m.Cons {
		switch c.Kind {
		case Linear:
			neg := func() []int64 {
				a := make([]int64, len(c.Coefs))
				for i, x := range c.Coefs {
					a[i] = -x
				}
				return a
			}
			switch c.Op {
			case LE:
				s.add(&linLE{c.Vars, c.Coefs, c.RHS})
			case GE:
				s.add(&linLE{c.Vars, neg(), -c.RHS})
			case EQ:
				s.add(&linLE{c.Vars, c.Coefs, c.RHS})
				s.add(&linLE{c.Vars, neg(), -c.RHS})
			case NE:
				s.add(&linNE{c.Vars, c.Coefs, c.RHS})
			default:
				return invalid("linear: unknown operator %d", c.Op)
			}
		case AllDifferent:
			s.add(&allDiff{c.Vars})
		case Element:
			s.add(&element{c.Vars[0], c.Vars[1], c.Array})
		case Table:
			s.add(&table{c.Vars, c.Tuples})
		case Count:
			s.add(&count{c.Vars, c.Value, c.Op, c.RHS})
		case Max, Min:
			s.add(&maxOf{c.Vars[0], c.Vars[1:], c.Kind == Min})
		case Cumulative:
			s.add(newCumulative(c.Tasks, c.Capacity))
		case NoOverlap:
			cum, pairs := newNoOverlap(c.Tasks)
			s.add(cum)
			s.add(newEdgeFind(c.Tasks))
			for _, pr := range pairs {
				o := s.addVar(newDom(0, 1))
				se.orders = append(se.orders, &order{pr[0], pr[1], o})
				s.add(se.orders[len(se.orders)-1])
			}
		case Circuit:
			s.add(&circuit{c.Vars, &allDiff{c.Vars}})
		}
	}
	if len(se.m.Objective) > 0 {
		o := &linLE{c: math.MaxInt64 / 4}
		for _, t := range se.m.Objective {
			a := t.Coef
			if se.m.Sense == Maximize {
				a = -a
			}
			o.x = append(o.x, t.Var)
			o.a = append(o.a, a)
		}
		se.obj, se.objK = o, len(s.props)
		s.add(o)
	} else if se.m.Objective != nil {
		se.m.Objective = nil // an empty objective is a satisfaction problem
	}
	return nil
}

// choose picks the next variable and whether to try its largest value
// first. Order variables come first: the unordered pair whose earlier task
// can start soonest, that task first. Then SearchFirst variables, earliest
// lower bound first; then the variable with the fewest values.
func (se *search) choose() (int, bool) {
	s := se.s
	var bo *order
	var bkey int64
	for _, o := range se.orders {
		if s.fixed(o.o) {
			continue
		}
		key := min(s.lo(o.a.Start), s.lo(o.b.Start))
		if bo == nil || key < bkey {
			bo, bkey = o, key
		}
	}
	if bo != nil {
		ea, eb := s.lo(bo.a.Start), s.lo(bo.b.Start)
		bFirst := eb < ea || eb == ea && s.hi(bo.b.Start) < s.hi(bo.a.Start)
		return bo.o, bFirst // o = 1 is b first
	}
	best := -1
	for v := range se.m.Vars {
		if s.fixed(v) {
			continue
		}
		if best < 0 {
			best = v
			continue
		}
		bf, vf := se.first[best], se.first[v]
		switch {
		case vf && !bf:
			best = v
		case bf && !vf:
		case vf: // both start times: earliest first
			if s.lo(v) < s.lo(best) || s.lo(v) == s.lo(best) && s.hi(v) < s.hi(best) {
				best = v
			}
		default: // fewest values first
			if s.doms[v].size < s.doms[best].size {
				best = v
			}
		}
	}
	if best < 0 {
		return -1, false
	}
	return best, se.hiFst[best]
}

func (se *search) solution() {
	s := se.s
	vals := make([]int64, len(se.m.Vars))
	for i := range vals {
		vals[i] = s.doms[i].lo
	}
	if se.lim.All {
		se.res.Solutions = append(se.res.Solutions, vals)
		if !se.have {
			se.best = vals
		}
		se.have = true
		if len(se.res.Solutions) >= se.lim.MaxSolutions {
			se.stop = "solutions"
		}
		return
	}
	se.have, se.best = true, vals
	if se.obj == nil {
		se.done = true
		return
	}
	var v int64
	for i, x := range se.obj.x {
		v += se.obj.a[i] * vals[x]
	}
	se.bestV = v
	se.obj.c = v - 1
}

func (se *search) dfs() {
	s := se.s
	se.res.Nodes++
	if se.res.Nodes >= se.lim.MaxNodes {
		se.stop = "nodes"
		return
	}
	if se.res.Nodes%256 == 0 && se.ctx.Err() != nil {
		se.stop = "time"
		return
	}
	v, hi := se.choose()
	if v < 0 {
		se.solution()
		return
	}
	frame := s.newStamp()
	m := s.mark()
	for {
		val := s.lo(v)
		if hi {
			val = s.hi(v)
		}
		s.newStamp()
		m2 := s.mark()
		if s.fix(v, val) && se.propagate() {
			se.dfs()
		} else {
			se.res.Fails++
		}
		s.undo(m2)
		if se.stop != "" || se.done {
			break
		}
		s.stamp = frame
		var ok bool
		if hi {
			ok = s.setMax(v, val-1)
		} else {
			ok = s.setMin(v, val+1)
		}
		if !ok || !se.propagate() {
			s.clearQueue()
			break
		}
	}
	s.undo(m)
}

// propagate runs to a fixpoint, with the objective bound re-checked.
func (se *search) propagate() bool {
	if se.objK >= 0 {
		se.s.enqueue(se.objK)
	}
	return se.s.fixpoint()
}

// destructiveBound finds the smallest objective value t >= lb that root
// propagation does not refute when the objective is held <= t. Every value
// below it fails, so it is a valid lower bound. Binary search, about 60
// fixpoints at most. Called at the root state (after the search unwinds).
func (se *search) destructiveBound(lb int64) int64 {
	s := se.s
	saved := se.obj.c
	defer func() { se.obj.c = saved }()
	hi := saved // the incumbent bound (best-1), or no limit
	if !se.have {
		var maxSum int64
		for i, v := range se.obj.x {
			maxSum += -termMin(s, -se.obj.a[i], v)
		}
		hi = maxSum
	}
	refuted := func(t int64) bool {
		se.obj.c = t
		s.newStamp()
		m := s.mark()
		ok := se.propagate()
		s.clearQueue()
		s.undo(m)
		return !ok
	}
	if hi < lb {
		return lb
	}
	if refuted(hi) {
		return hi + 1
	}
	lo := lb
	for lo < hi {
		mid := lo + (hi-lo)/2
		if refuted(mid) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
