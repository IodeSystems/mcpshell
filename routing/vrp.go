package routing

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
)

// DefaultVRPIterations is the default ruin-and-recreate count for s stops.
// Measured: 100 stops, 10 vehicles ~0.3 s.
func DefaultVRPIterations(s int) int {
	switch {
	case s < 4:
		return 0
	case s <= 30:
		return 200
	case s <= 150:
		return 50
	case s <= 500:
		return 20
	}
	return 5
}

type evalRes struct {
	why             Violation
	load, dist, dur float64
	depart, back    float64
	cost            float64 // objective part: dist or dur
	lateStop        int     // stop index of a LateWindow
}

type vrpState struct {
	p      *VRP
	tol    float64
	routes [][]int
	cost   []float64 // per vehicle
	pool   []int     // stops not in a route that could fit alone
	nb     [][]int   // nb[i]: the nearest stops to stop i
}

// neighbours lists, for each stop, the k nearest other stops by D(i,j)+D(j,i).
func neighbours(p *VRP, k int) [][]int {
	nb := make([][]int, len(p.Stops))
	D := p.Dist
	for i := range p.Stops {
		cand := make([]int, 0, len(p.Stops)-1)
		for j := range p.Stops {
			if j != i {
				cand = append(cand, j)
			}
		}
		ni := p.Stops[i].Node
		d := func(j int) float64 { nj := p.Stops[j].Node; return D.At(ni, nj) + D.At(nj, ni) }
		sort.SliceStable(cand, func(a, b int) bool { return d(cand[a]) < d(cand[b]) })
		nb[i] = cand[:min(k, len(cand))]
	}
	return nb
}

// eval checks route r on vehicle v. arrive, if not nil, gets the service
// start times.
func (s *vrpState) eval(v int, r []int, arrive []float64) evalRes {
	p := s.p
	veh := &p.Vehicles[v]
	var e evalRes
	if len(r) == 0 {
		return e
	}
	for _, i := range r {
		e.load += p.Stops[i].Demand
	}
	if e.load > veh.Capacity+s.tol {
		e.why = OverCapacity
		return e
	}
	D, T := p.Dist, p.Time
	n := D.N
	prev := veh.Start
	first := &p.Stops[r[0]]
	t := math.Max(veh.Open, first.Open-T.C[prev*n+first.Node])
	e.depart = t
	for k, i := range r {
		st := &p.Stops[i]
		t += T.C[prev*n+st.Node]
		e.dist += D.C[prev*n+st.Node]
		if t < st.Open {
			t = st.Open
		}
		if t > st.Close+s.tol {
			e.why, e.lateStop = LateWindow, i
			return e
		}
		if arrive != nil {
			arrive[k] = t
		}
		t += st.Service
		prev = st.Node
	}
	t += T.C[prev*n+veh.End]
	e.dist += D.C[prev*n+veh.End]
	e.back = t
	e.dur = t - e.depart
	switch {
	case t > veh.Close+s.tol:
		e.why = OverShift
	case e.dur > veh.MaxTime+s.tol:
		e.why = OverTime
	case e.dist > veh.MaxDist+s.tol:
		e.why = OverDist
	}
	e.cost = e.dist
	if p.Objective == MinTime {
		e.cost = e.dur
	}
	return e
}

// score is the plan objective, compared in order: unassigned stops,
// vehicles used (MinVehicles only), cost.
type score struct {
	un, veh int
	cost    float64
}

func (s *vrpState) better(a, b score) bool {
	if a.un != b.un {
		return a.un < b.un
	}
	if a.veh != b.veh {
		return a.veh < b.veh
	}
	return a.cost < b.cost-s.eps()
}

func (s *vrpState) eps() float64 {
	if s.p.Integer {
		return 0.5
	}
	return math.Max(s.tol, 1e-12)
}

func (s *vrpState) score() score {
	var sc score
	sc.un = len(s.pool)
	for v, r := range s.routes {
		if len(r) > 0 {
			sc.cost += s.cost[v]
			if s.p.Objective == MinVehicles {
				sc.veh++
			}
		}
	}
	return sc
}

func (s *vrpState) clone() *vrpState {
	c := &vrpState{p: s.p, tol: s.tol, nb: s.nb, cost: slices.Clone(s.cost), pool: slices.Clone(s.pool)}
	c.routes = make([][]int, len(s.routes))
	for i, r := range s.routes {
		c.routes[i] = slices.Clone(r)
	}
	return c
}

// change is a candidate replacement of up to two routes.
type change struct {
	v    [2]int
	r    [2][]int
	cost [2]float64
}

// try applies routes ra on va and rb on vb (vb < 0: one route) when both
// are feasible and the plan improves.
func (s *vrpState) try(va int, ra []int, vb int, rb []int) bool {
	ea := s.eval(va, ra, nil)
	if ea.why != OK {
		return false
	}
	old := s.score()
	nw := old
	nw.cost += ea.cost - s.cost[va]
	if s.p.Objective == MinVehicles {
		nw.veh += b2i(len(ra) > 0) - b2i(len(s.routes[va]) > 0)
	}
	var eb evalRes
	if vb >= 0 {
		eb = s.eval(vb, rb, nil)
		if eb.why != OK {
			return false
		}
		nw.cost += eb.cost - s.cost[vb]
		if s.p.Objective == MinVehicles {
			nw.veh += b2i(len(rb) > 0) - b2i(len(s.routes[vb]) > 0)
		}
	}
	if !s.better(nw, old) {
		return false
	}
	s.routes[va], s.cost[va] = ra, ea.cost
	if vb >= 0 {
		s.routes[vb], s.cost[vb] = rb, eb.cost
	}
	return true
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func splice(parts ...[]int) []int {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]int, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// openPenalty makes MinVehicles insertion open a new vehicle last.
const openPenalty = 1e15

// insertBest puts stop i at its cheapest feasible position on any vehicle
// but skip (-1: none). It returns false when no vehicle can take it.
func (s *vrpState) insertBest(i, skip int) bool {
	bestV, bestPos, bestD := -1, -1, Inf
	for v, r := range s.routes {
		if v == skip {
			continue
		}
		for pos := 0; pos <= len(r); pos++ {
			e := s.eval(v, splice(r[:pos], []int{i}, r[pos:]), nil)
			if e.why != OK {
				continue
			}
			d := e.cost - s.cost[v]
			if s.p.Objective == MinVehicles && len(r) == 0 {
				d += openPenalty
			}
			if d < bestD {
				bestV, bestPos, bestD = v, pos, d
			}
		}
	}
	if bestV < 0 {
		return false
	}
	r := s.routes[bestV]
	s.routes[bestV] = splice(r[:bestPos], []int{i}, r[bestPos:])
	s.cost[bestV] = s.eval(bestV, s.routes[bestV], nil).cost
	return true
}

// fillPool inserts pool stops, cheapest first, while any fits.
func (s *vrpState) fillPool() {
	for len(s.pool) > 0 {
		progress := false
		rest := s.pool[:0:0]
		for _, i := range s.pool {
			if !s.insertBest(i, -1) {
				rest = append(rest, i)
			} else {
				progress = true
			}
		}
		s.pool = rest
		if !progress {
			return
		}
	}
}

// localSearch runs relocate (segments of 1-3), swap, 2-opt* and intra-route
// 2-opt to a local optimum. Moves between routes join a stop to one of its
// nearest neighbours (s.nb), plus empty vehicles for relocate.
func (s *vrpState) localSearch(ctx context.Context) {
	nv := len(s.routes)
	route := make([]int, len(s.p.Stops)) // stop -> vehicle, -1 when not routed
	pos := make([]int, len(s.p.Stops))
	index := func() {
		for i := range route {
			route[i] = -1
		}
		for v, r := range s.routes {
			for k, i := range r {
				route[i], pos[i] = v, k
			}
		}
	}
	for improved := true; improved && ctx.Err() == nil; {
		improved = false
		if len(s.pool) > 0 {
			before := len(s.pool)
			s.fillPool()
			improved = len(s.pool) < before
		}
		index()
		// Relocate the segment r[i..i+L-1] next to a neighbour, or onto an empty vehicle.
		for a := 0; a < nv; a++ {
			for L := 1; L <= 3; L++ {
				for i := 0; i+L <= len(s.routes[a]); i++ {
					if ctx.Err() != nil {
						return
					}
					ra := s.routes[a]
					seg := slices.Clone(ra[i : i+L])
					rest := splice(ra[:i], ra[i+L:])
					x, y := seg[0], seg[L-1]
					moved := false
					tryAt := func(b, p int) {
						if moved {
							return
						}
						if b == a {
							if p != i && p <= len(rest) && s.try(a, splice(rest[:p], seg, rest[p:]), -1, nil) {
								moved = true
							}
							return
						}
						rb := s.routes[b]
						if s.try(a, rest, b, splice(rb[:p], seg, rb[p:])) {
							moved = true
						}
					}
					// after a neighbour w of x, or before a neighbour w of y.
					for _, w := range s.nb[x] {
						if b := route[w]; b >= 0 && !(b == a && pos[w] >= i && pos[w] < i+L) {
							p := pos[w] + 1
							if b == a && pos[w] >= i+L {
								p -= L
							}
							tryAt(b, p)
						}
					}
					for _, w := range s.nb[y] {
						if b := route[w]; b >= 0 && !(b == a && pos[w] >= i && pos[w] < i+L) {
							p := pos[w]
							if b == a && pos[w] >= i+L {
								p -= L
							}
							tryAt(b, p)
						}
					}
					for b := 0; b < nv && !moved; b++ {
						if b != a && len(s.routes[b]) == 0 {
							tryAt(b, 0)
						}
					}
					if moved {
						improved = true
						index()
					}
				}
			}
		}
		// Swap a stop with a neighbour on another route; 2-opt*: join a stop
		// to a neighbour on another route and exchange the tails.
		for x := range s.p.Stops {
			if ctx.Err() != nil {
				return
			}
			for _, w := range s.nb[x] {
				a, b := route[x], route[w]
				if a < 0 || b < 0 || a == b {
					continue
				}
				i, j := pos[x], pos[w]
				ra, rb := slices.Clone(s.routes[a]), slices.Clone(s.routes[b])
				ra[i], rb[j] = rb[j], ra[i]
				if s.try(a, ra, b, rb) {
					improved = true
					index()
					continue
				}
				ra, rb = s.routes[a], s.routes[b]
				if s.try(a, splice(ra[:i+1], rb[j:]), b, splice(rb[:j], ra[i+1:])) {
					improved = true
					index()
				}
			}
		}
		// Intra-route 2-opt.
		for a := 0; a < nv; a++ {
			for i := 0; i < len(s.routes[a]); i++ {
				for j := i + 1; j < len(s.routes[a]); j++ {
					r := slices.Clone(s.routes[a])
					slices.Reverse(r[i : j+1])
					if s.try(a, r, -1, nil) {
						improved = true
					}
				}
			}
		}
		// Route elimination: move every stop of the smallest routes elsewhere.
		if s.eliminate() {
			improved = true
		}
	}
}

// eliminate tries to empty each route by inserting its stops into the others.
func (s *vrpState) eliminate() bool {
	order := make([]int, 0, len(s.routes))
	for v, r := range s.routes {
		if len(r) > 0 {
			order = append(order, v)
		}
	}
	if len(order) < 2 {
		return false
	}
	sort.Slice(order, func(i, j int) bool { return len(s.routes[order[i]]) < len(s.routes[order[j]]) })
	any := false
	for _, v := range order[:min(len(order), 3)] {
		if len(s.routes[v]) == 0 {
			continue
		}
		c := s.clone()
		stops := c.routes[v]
		c.routes[v], c.cost[v] = nil, 0
		ok := true
		for _, i := range stops {
			if !c.insertBest(i, v) {
				ok = false
				break
			}
		}
		if ok && s.better(c.score(), s.score()) {
			*s = *c
			any = true
		}
	}
	return any
}

// ruin removes the stop nearest to a random seed stop and its k-1 nearest
// routed neighbours, then reinserts them.
func (s *vrpState) ruinRecreate(rng *rand.Rand, k int) {
	var routed []int
	for _, r := range s.routes {
		routed = append(routed, r...)
	}
	if len(routed) == 0 {
		return
	}
	seed := routed[rng.Intn(len(routed))]
	D := s.p.Dist
	sn := s.p.Stops[seed].Node
	sort.Slice(routed, func(a, b int) bool {
		da := D.At(sn, s.p.Stops[routed[a]].Node) + D.At(s.p.Stops[routed[a]].Node, sn)
		db := D.At(sn, s.p.Stops[routed[b]].Node) + D.At(s.p.Stops[routed[b]].Node, sn)
		return da < db
	})
	remove := map[int]bool{}
	for _, i := range routed[:min(k, len(routed))] {
		remove[i] = true
	}
	for v, r := range s.routes {
		nr := r[:0:0]
		for _, i := range r {
			if !remove[i] {
				nr = append(nr, i)
			}
		}
		if len(nr) != len(r) {
			s.routes[v] = nr
			s.cost[v] = s.eval(v, nr, nil).cost
		}
	}
	removed := routed[:min(k, len(routed))]
	rng.Shuffle(len(removed), func(a, b int) { removed[a], removed[b] = removed[b], removed[a] })
	s.pool = append(s.pool, removed...)
	s.fillPool()
}

// SolveVRP builds a plan: Clarke-Wright savings, assignment to vehicles,
// cheapest insertion of the rest, local search, then ruin and recreate.
func SolveVRP(ctx context.Context, p *VRP) (*VRPResult, error) {
	if p.Dist == nil || p.Time == nil || p.Dist.N != p.Time.N {
		return nil, invalid("dist and time matrices must have the same size")
	}
	if len(p.Vehicles) == 0 {
		return nil, invalid("no vehicles")
	}
	n := p.Dist.N
	for _, m := range []*Matrix{p.Dist, p.Time} {
		for _, c := range m.C {
			if c < 0 || math.IsNaN(c) || math.IsInf(c, 1) {
				return nil, invalid("costs must be finite and >= 0")
			}
		}
	}
	maxT := 0.0
	for _, c := range p.Time.C {
		maxT = math.Max(maxT, c)
	}
	for i, st := range p.Stops {
		if st.Node < 0 || st.Node >= n {
			return nil, invalid("stop %d: node %d is not in the matrix", i, st.Node)
		}
		if st.Close < st.Open {
			return nil, invalid("stop %d: window closes before it opens", i)
		}
		maxT = math.Max(maxT, math.Max(math.Abs(st.Open), st.Service))
		if !math.IsInf(st.Close, 1) {
			maxT = math.Max(maxT, math.Abs(st.Close))
		}
	}
	for i, v := range p.Vehicles {
		if v.Start < 0 || v.Start >= n || v.End < 0 || v.End >= n {
			return nil, invalid("vehicle %d: start or end is not in the matrix", i)
		}
	}
	s := &vrpState{p: p, routes: make([][]int, len(p.Vehicles)), cost: make([]float64, len(p.Vehicles)), nb: neighbours(p, 12)}
	if !p.Integer {
		s.tol = 1e-9 * math.Max(maxT, 1)
	}
	res := &VRPResult{}

	// 1. Stops no vehicle can serve alone.
	var ok []int
	for i := range p.Stops {
		fits := false
		for v := range p.Vehicles {
			if s.eval(v, []int{i}, nil).why == OK {
				fits = true
				break
			}
		}
		if fits {
			ok = append(ok, i)
			continue
		}
		res.Unassigned = append(res.Unassigned, s.alone(i))
	}

	// 2. Savings on the first vehicle's depots, merges feasible on some vehicle.
	type rinfo struct{ stops []int }
	routeOf := make([]int, len(p.Stops))
	var routes []*rinfo
	for _, i := range ok {
		routeOf[i] = len(routes)
		routes = append(routes, &rinfo{stops: []int{i}})
	}
	profiles := distinctVehicles(p.Vehicles)
	feasibleAny := func(r []int) bool {
		for _, v := range profiles {
			if s.eval(v, r, nil).why == OK {
				return true
			}
		}
		return false
	}
	type saving struct {
		i, j int
		s    float64
	}
	var sv []saving
	S, E := p.Vehicles[0].Start, p.Vehicles[0].End
	D := p.Dist
	if len(ok) <= 3000 {
		for _, i := range ok {
			for _, j := range ok {
				if i == j {
					continue
				}
				ni, nj := p.Stops[i].Node, p.Stops[j].Node
				if x := D.At(ni, E) + D.At(S, nj) - D.At(ni, nj); x > 0 {
					sv = append(sv, saving{i, j, x})
				}
			}
		}
	}
	sort.SliceStable(sv, func(a, b int) bool { return sv[a].s > sv[b].s })
	for k, x := range sv {
		if k&1023 == 0 && ctx.Err() != nil {
			break
		}
		ri, rj := routeOf[x.i], routeOf[x.j]
		if ri == rj {
			continue
		}
		a, b := routes[ri].stops, routes[rj].stops
		if a[len(a)-1] != x.i || b[0] != x.j {
			continue
		}
		merged := splice(a, b)
		if !feasibleAny(merged) {
			continue
		}
		routes[ri].stops = merged
		routes[rj].stops = nil
		for _, st := range b {
			routeOf[st] = ri
		}
	}

	// 3. Assign the routes to vehicles, largest load first.
	var built [][]int
	for _, r := range routes {
		if len(r.stops) > 0 {
			built = append(built, r.stops)
		}
	}
	load := func(r []int) float64 {
		t := 0.0
		for _, i := range r {
			t += p.Stops[i].Demand
		}
		return t
	}
	sort.SliceStable(built, func(a, b int) bool { return load(built[a]) > load(built[b]) })
	for _, r := range built {
		bestV, bestC := -1, Inf
		for v := range p.Vehicles {
			if len(s.routes[v]) > 0 {
				continue
			}
			e := s.eval(v, r, nil)
			if e.why == OK && (e.cost < bestC || (e.cost == bestC && p.Vehicles[v].Capacity < p.Vehicles[bestV].Capacity)) {
				bestV, bestC = v, e.cost
			}
		}
		if bestV < 0 {
			s.pool = append(s.pool, r...)
			continue
		}
		s.routes[bestV], s.cost[bestV] = r, bestC
	}
	s.fillPool()

	// 4. Local search, then ruin and recreate from the best plan.
	s.localSearch(ctx)
	best := s.clone()
	iters := p.Iterations
	if iters == 0 {
		iters = DefaultVRPIterations(len(ok))
	}
	rng := rand.New(rand.NewSource(p.Seed))
	k := max(2, len(ok)/10)
	done := 0
	for ; done < iters && ctx.Err() == nil; done++ {
		c := best.clone()
		c.ruinRecreate(rng, k)
		c.localSearch(ctx)
		if c.better(c.score(), best.score()) {
			best = c
		}
	}
	res.Iterations = done

	// 5. Report.
	for v, r := range best.routes {
		if len(r) == 0 {
			continue
		}
		arr := make([]float64, len(r))
		e := best.eval(v, r, arr)
		res.Routes = append(res.Routes, Route{Vehicle: v, Stops: r, Load: e.load, Dist: e.dist, Time: e.dur, Depart: e.depart, Back: e.back, Arrive: arr})
		res.Dist += e.dist
		res.Time += e.dur
	}
	for _, i := range best.pool {
		res.Unassigned = append(res.Unassigned, Unassigned{Stop: i, Why: Crowded})
	}
	sort.Slice(res.Unassigned, func(a, b int) bool { return res.Unassigned[a].Stop < res.Unassigned[b].Stop })
	switch p.Objective {
	case MinVehicles:
		res.Cost = float64(len(res.Routes))
	case MinTime:
		res.Cost = res.Time
	default:
		res.Cost = res.Dist
	}
	res.Bound = vrpBound(p)
	switch {
	case len(res.Unassigned) > 0:
		res.Status = Infeasible
	case len(p.Stops) == 0:
		res.Status = Optimal
	case p.Integer && p.Objective != MinVehicles && res.Bound >= res.Cost:
		res.Status = Optimal
	default:
		res.Status = Feasible
	}
	if res.Status != Infeasible {
		res.Bound = math.Min(res.Bound, res.Cost)
	}
	res.Stopped = ctx.Err()
	return res, nil
}

// alone says why stop i fits no vehicle on its own.
func (s *vrpState) alone(i int) Unassigned {
	p := s.p
	st := p.Stops[i]
	maxCap := math.Inf(-1)
	for _, v := range p.Vehicles {
		maxCap = math.Max(maxCap, v.Capacity)
	}
	if st.Demand > maxCap+s.tol {
		return Unassigned{Stop: i, Why: OverCapacity, Value: st.Demand, Limit: maxCap}
	}
	counts := map[Violation]int{}
	earliest := Inf
	for v, veh := range p.Vehicles {
		e := s.eval(v, []int{i}, nil)
		counts[e.why]++
		if e.why == LateWindow {
			earliest = math.Min(earliest, veh.Open+p.Time.At(veh.Start, st.Node))
		}
	}
	u := Unassigned{Stop: i, Why: NoRoom}
	switch {
	case counts[LateWindow] == len(p.Vehicles):
		u.Why, u.Value, u.Limit = LateWindow, earliest, st.Close
	case counts[OverShift] > 0:
		u.Why = OverShift
	case counts[OverTime] > 0:
		u.Why = OverTime
	case counts[OverDist] > 0:
		u.Why = OverDist
	}
	return u
}

// Reason is the text of u, with numbers formatted by num.
func (u Unassigned) Reason(num func(float64) string) string {
	switch u.Why {
	case OverCapacity:
		return fmt.Sprintf("demand %s is over the largest vehicle capacity %s", num(u.Value), num(u.Limit))
	case LateWindow:
		return fmt.Sprintf("window closes at %s; the earliest arrival is %s", num(u.Limit), num(u.Value))
	case OverShift:
		return "no vehicle can serve it alone and be back by the end of the vehicle window"
	case OverTime:
		return "a route to it alone is over maxTime"
	case OverDist:
		return "a route to it alone is over maxDist"
	case Crowded:
		return "no vehicle has room left (capacity, windows or limits are used by other stops)"
	}
	return "no vehicle can serve it (capacity, window or limits)"
}

// Fnum formats a float64 as an integer when it is one.
func Fnum(f float64) string { return fnum(f) }

func fnum(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

func distinctVehicles(vs []Vehicle) []int {
	var out []int
	seen := map[Vehicle]bool{}
	for i, v := range vs {
		if !seen[v] {
			seen[v] = true
			out = append(out, i)
		}
	}
	return out
}

// vrpBound is a lower bound on the objective when every stop is served.
//
// Vehicles: kmin, the fewest largest capacities that hold the total demand.
//
// Distance (time adds service), the largest of:
//   - in: each stop is entered once, by the cheapest arc at best, and each of
//     the at least kmin routes ends with an arc into an end depot;
//   - out: the same for arcs out of stops and out of start depots;
//   - one stop: the route that serves stop i costs at least
//     r_i = min over vehicles of sp(start, i) + sp(i, end) (sp: shortest path);
//   - radial: a route costs at least the demand-weighted mean of r_i over its
//     stops, so the total is >= sum_i demand_i * r_i / max capacity.
func vrpBound(p *VRP) float64 {
	if len(p.Stops) == 0 {
		return 0
	}
	total, maxCap := 0.0, 0.0
	for _, st := range p.Stops {
		total += st.Demand
	}
	caps := make([]float64, len(p.Vehicles))
	for i, v := range p.Vehicles {
		caps[i] = v.Capacity
		maxCap = math.Max(maxCap, v.Capacity)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(caps)))
	kmin, sum := len(caps), 0.0
	for k, c := range caps {
		sum += c
		if sum >= total {
			kmin = k + 1
			break
		}
	}
	if p.Objective == MinVehicles {
		return float64(kmin)
	}
	m := p.Dist
	if p.Objective == MinTime {
		m = p.Time
	}
	in, out, service := 0.0, 0.0, 0.0
	toEnd, fromStart := Inf, Inf
	for i, st := range p.Stops {
		bi, bo := Inf, Inf
		for j, o := range p.Stops {
			if j != i {
				bi = math.Min(bi, m.At(o.Node, st.Node))
				bo = math.Min(bo, m.At(st.Node, o.Node))
			}
		}
		for _, v := range p.Vehicles {
			bi = math.Min(bi, m.At(v.Start, st.Node))
			bo = math.Min(bo, m.At(st.Node, v.End))
			toEnd = math.Min(toEnd, m.At(st.Node, v.End))
			fromStart = math.Min(fromStart, m.At(v.Start, st.Node))
		}
		in += bi
		out += bo
		service += st.Service
	}
	b := math.Max(in+float64(kmin)*toEnd, out+float64(kmin)*fromStart)
	from := map[int][]float64{}
	to := map[int][]float64{}
	for _, v := range p.Vehicles {
		if from[v.Start] == nil {
			from[v.Start] = DenseShortest(m, v.Start, false)
		}
		if to[v.End] == nil {
			to[v.End] = DenseShortest(m, v.End, true)
		}
	}
	radial, one := 0.0, 0.0
	for _, st := range p.Stops {
		r := Inf
		for _, v := range p.Vehicles {
			r = math.Min(r, from[v.Start][st.Node]+to[v.End][st.Node])
		}
		one = math.Max(one, r)
		radial += st.Demand * r
	}
	b = math.Max(b, one)
	if total > 0 && maxCap > 0 && !math.IsInf(maxCap, 1) {
		b = math.Max(b, radial/maxCap)
	}
	if p.Objective == MinTime {
		b += service
	}
	if p.Integer {
		b = math.Ceil(b - 1e-9*b)
	}
	return b
}
