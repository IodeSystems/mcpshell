package cp

// propagator narrows domains. propagate returns false on a contradiction.
type propagator interface {
	vars() []int
	propagate(s *store) bool
}

type trailEntry struct {
	v     int
	d     *dom
	stamp int
}

// store holds the current domains. A change copies the domain once per
// stamp and records the old one on the trail, so backtracking restores it.
type store struct {
	doms    []*dom
	owner   []int // stamp at which doms[i] was copied
	stamp   int
	nstamp  int // last stamp handed out
	trail   []trailEntry
	watch   [][]int
	props   []propagator
	queue   []int
	inQueue []bool
	current int // propagator running, or -1
}

func newStore(doms []*dom) *store {
	return &store{doms: doms, owner: make([]int, len(doms)), watch: make([][]int, len(doms)), current: -1}
}

func (s *store) add(p propagator) {
	k := len(s.props)
	s.props = append(s.props, p)
	s.inQueue = append(s.inQueue, false)
	seen := map[int]bool{}
	for _, v := range p.vars() {
		if !seen[v] {
			seen[v] = true
			s.watch[v] = append(s.watch[v], k)
		}
	}
}

// newStamp starts a new copy-on-write scope.
func (s *store) newStamp() int {
	s.nstamp++
	s.stamp = s.nstamp
	return s.stamp
}

func (s *store) mark() int { return len(s.trail) }

// undo restores the domains to trail position m.
func (s *store) undo(m int) {
	for len(s.trail) > m {
		e := s.trail[len(s.trail)-1]
		s.trail = s.trail[:len(s.trail)-1]
		s.doms[e.v], s.owner[e.v] = e.d, e.stamp
	}
}

func (s *store) mut(v int) *dom {
	if s.owner[v] != s.stamp {
		s.trail = append(s.trail, trailEntry{v, s.doms[v], s.owner[v]})
		s.doms[v] = s.doms[v].clone()
		s.owner[v] = s.stamp
	}
	return s.doms[v]
}

func (s *store) changed(v int) bool {
	if s.doms[v].size == 0 {
		return false
	}
	for _, k := range s.watch[v] {
		if !s.inQueue[k] { // the running propagator too: one pass need not reach its own fixpoint
			s.inQueue[k] = true
			s.queue = append(s.queue, k)
		}
	}
	return true
}

func (s *store) lo(v int) int64          { return s.doms[v].lo }
func (s *store) hi(v int) int64          { return s.doms[v].hi }
func (s *store) fixed(v int) bool        { return s.doms[v].size == 1 }
func (s *store) has(v int, x int64) bool { return s.doms[v].has(x) }

// The narrowing operations return false on a wipeout.

func (s *store) setMin(v int, x int64) bool {
	if x <= s.doms[v].lo {
		return true
	}
	s.mut(v).setMin(x)
	return s.changed(v)
}

func (s *store) setMax(v int, x int64) bool {
	if x >= s.doms[v].hi {
		return true
	}
	s.mut(v).setMax(x)
	return s.changed(v)
}

func (s *store) remove(v int, x int64) bool {
	if !s.doms[v].has(x) {
		return true
	}
	s.mut(v).remove(x)
	return s.changed(v)
}

func (s *store) fix(v int, x int64) bool {
	if !s.doms[v].has(x) {
		return false
	}
	if s.doms[v].size == 1 {
		return true
	}
	s.mut(v).keepOnly(func(y int64) bool { return y == x })
	return s.changed(v)
}

func (s *store) keepOnly(v int, keep func(int64) bool) bool {
	d := s.doms[v]
	any := false
	d.each(func(x int64) {
		if !keep(x) {
			any = true
		}
	})
	if !any {
		return true
	}
	s.mut(v).keepOnly(keep)
	return s.changed(v)
}

// fixpoint runs queued propagators until none changes a domain.
func (s *store) fixpoint() bool {
	for len(s.queue) > 0 {
		k := s.queue[0]
		s.queue = s.queue[1:]
		s.inQueue[k] = false
		s.current = k
		ok := s.props[k].propagate(s)
		s.current = -1
		if !ok {
			for _, q := range s.queue {
				s.inQueue[q] = false
			}
			s.queue = s.queue[:0]
			return false
		}
	}
	return true
}

func (s *store) enqueue(k int) {
	if !s.inQueue[k] {
		s.inQueue[k] = true
		s.queue = append(s.queue, k)
	}
}

func (s *store) enqueueAll() {
	for k := range s.props {
		s.enqueue(k)
	}
}

func (s *store) clearQueue() {
	for _, q := range s.queue {
		s.inQueue[q] = false
	}
	s.queue = s.queue[:0]
}

// addVar adds a hidden variable (before search starts).
func (s *store) addVar(d *dom) int {
	s.doms = append(s.doms, d)
	s.owner = append(s.owner, 0)
	s.watch = append(s.watch, nil)
	return len(s.doms) - 1
}
