package toolkit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/iodesystems/mcpshell/routing"
	. "github.com/iodesystems/mcpshell/runtime"
)

// costs is one cost column of a source, registered in a dim. After the dim
// finishes, dense gives the matrix over chosen nodes.
type costs struct {
	s   *source
	col string
	w   []float64       // edges: arc weights
	pm  *routing.Matrix // points: the matrix over R (built at register time)
	R   []int           // points: the nodes pm covers
}

// register puts the column into d. For points it computes the distances
// over R now (they are floats, the dim must see them before finish).
func (s *source) register(d *dim, col string, R []int, sh *Shell) *costs {
	k := &costs{s: s, col: col}
	switch s.kind {
	case "points":
		checkMatrixAlloc(sh, s.c, len(R))
		m := &routing.Matrix{N: len(R), C: make([]float64, len(R)*len(R))}
		d.add(m.C)
		for i, a := range R {
			for j, b := range R {
				if i != j {
					d.setFloat(m.C, i*len(R)+j, s.pointDist(a, b))
				}
			}
		}
		k.pm, k.R = m, R
	case "matrix":
		fillMatrix(d, s.m, s.mrows)
	case "edges":
		k.w = s.edgeWeights(d, col)
	}
	return k
}

func checkMatrixAlloc(sh *Shell, c rfail, n int) {
	if n > routing.MaxNodes {
		c.fail("%d nodes is over the limit of %d (an n x n matrix)", n, routing.MaxNodes)
	}
	sh.Limits().CheckAlloc(int64(n)*int64(n)*8, fmt.Sprintf("%s: a %d x %d matrix", string(c), n, n))
}

// graph is the sparse graph of the column (edges or matrix source).
func (k *costs) graph() *routing.Graph {
	s := k.s
	g := routing.NewGraph(len(s.nodes.ids))
	switch s.kind {
	case "edges":
		for a := range s.from {
			g.AddArc(s.from[a], s.to[a], k.w[a])
		}
	case "matrix":
		n := s.m.N
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if w := s.m.C[i*n+j]; i != j && !math.IsInf(w, 1) {
					g.AddArc(i, j, w)
				}
			}
		}
	}
	return g
}

// dense is the cost matrix over R; a missing pair costs the shortest path.
func (k *costs) dense(ctx context.Context, sh *Shell, R []int) *routing.Matrix {
	s := k.s
	if s.kind == "points" {
		return k.pm
	}
	checkMatrixAlloc(sh, s.c, len(R))
	n := len(R)
	m := &routing.Matrix{N: n, C: make([]float64, n*n)}
	unreachable := func(i, j int) {
		s.c.fail("no path from %s to %s%s", s.nodes.ids[R[i]].Inspect(), s.nodes.ids[R[j]].Inspect(),
			map[bool]string{true: " (edges are two-way unless directed: true)", false: ""}[s.kind == "edges"])
	}
	var g *routing.Graph
	for i, a := range R {
		if i&15 == 0 && ctx.Err() != nil {
			routeTimeFail(sh, s.c, ctx)
		}
		var row []float64
		switch s.kind {
		case "matrix":
			N := s.m.N
			full := s.m.C[a*N : (a+1)*N]
			missing := false
			for _, b := range R {
				if math.IsInf(full[b], 1) {
					missing = true
					break
				}
			}
			row = full
			if missing {
				row = routing.DenseShortest(s.m, a, false)
			}
		case "edges":
			if g == nil {
				g = k.graph()
			}
			row, _, _ = routing.ShortestFrom(context.Background(), g, a)
		}
		for j, b := range R {
			if math.IsInf(row[b], 1) {
				unreachable(i, j)
			}
			m.C[i*n+j] = row[b]
		}
	}
	return m
}

// routeContext bounds a search by the eval's remaining time (90% of it, so
// the script can use the result) and timeLimit ms when given.
func routeContext(sh *Shell, c rfail, timeLimit int) (context.Context, func()) {
	lm := sh.Limits()
	remaining := lm.Remaining()
	if remaining <= 0 {
		c.fail("no time left in this eval's time limit (%dms). Raise it first: %s", lm.TimeoutMs, lm.RaiseHint("timeout", lm.TimeoutMs*2))
	}
	budget := remaining * 9 / 10
	if timeLimit > 0 && time.Duration(timeLimit)*time.Millisecond < budget {
		budget = time.Duration(timeLimit) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(25 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if lm.Cancelled() {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() { close(stop); cancel() }
}

// routeTimeFail raises the error for a context that ended before any
// result existed (while building the matrix).
func routeTimeFail(sh *Shell, c rfail, ctx context.Context) {
	lm := sh.Limits()
	if lm.Cancelled() {
		c.fail("Execution cancelled")
	}
	c.fail("stopped at the time limit while building the matrix. Raise the limit first (%s), or give fewer nodes", lm.RaiseHint("timeout", lm.TimeoutMs*2))
}

// stopped handles a search the context ended: cancelled is an error, a time
// limit is a note and stopped: "time" on the result.
func stopped(sh *Shell, c rfail, out *ObjectVal, err error, timeLimit int, iterations int, status string) {
	if err == nil {
		return
	}
	lm := sh.Limits()
	if lm.Cancelled() || errors.Is(err, context.Canceled) {
		c.fail("Execution cancelled")
	}
	out.Set("stopped", Str("time"))
	how := "raise the eval time limit (" + lm.RaiseHint("timeout", lm.TimeoutMs*2) + ")"
	if timeLimit > 0 {
		how = "raise timeLimit"
	}
	sh.Note(fmt.Sprintf("%s: the time limit stopped the search after %d iterations; the result is the best found, status %q. For a better plan, %s", string(c), iterations, status, how))
}

func gapVal(cost, bound float64) Value {
	return Num(math.Round(routing.Gap(cost, bound)*1e4) / 1e4)
}

func idList(ns *nodeSet, idx []int) *ArrayVal {
	out := make([]Value, len(idx))
	for i, v := range idx {
		out[i] = ns.ids[v]
	}
	return &ArrayVal{Elements: out}
}

// subset reads a stops/ids list into node indexes, or all nodes.
func subset(c rfail, ns *nodeSet, v Value, what string) []int {
	if v == nil {
		R := make([]int, len(ns.ids))
		for i := range R {
			R[i] = i
		}
		return R
	}
	a, ok := v.(*ArrayVal)
	if !ok || len(a.Elements) == 0 {
		c.fail("%s must be a non-empty array of ids, got %s", what, describe(v))
	}
	seen := map[int]bool{}
	var R []int
	for i, e := range a.Elements {
		if o, isObj := e.(*ObjectVal); isObj {
			if idv, has := o.Get("id"); has {
				e = idv
			}
		}
		x := ns.find(c, fmt.Sprintf("%s[%d]", what, i), e)
		if seen[x] {
			c.fail("%s[%d]: duplicate id %s", what, i, e.Inspect())
		}
		seen[x] = true
		R = append(R, x)
	}
	return R
}

func routeMatrix(sh *Shell, args []Value) Value {
	c := rfail("Route.matrix")
	if len(args) < 1 || len(args) > 2 {
		panic(WrongArguments(string(c), "rows, opts?", args, `Route.matrix([{id: "a", x: 0, y: 0}, {id: "b", x: 3, y: 4}])`))
	}
	spec := NewObject()
	rows := rowsOf(c, "rows", args[0])
	key := "points"
	if r0, ok := rows[0].(*ObjectVal); ok && (r0.Has("from") || r0.Has("to")) {
		key = "edges"
	}
	spec.Set(key, args[0])
	var stopsV Value
	format := "matrix"
	if len(args) == 2 && args[1] != Null {
		o := c.options(args[1], "opts", routeMatrixKeys)
		for _, k := range o.o.Keys() {
			v, _ := o.o.Get(k)
			switch k {
			case "stops":
				stopsV = v
			case "format":
				format = o.str("format", "matrix", "matrix", "rows")
			default:
				spec.Set(k, v)
			}
		}
	}
	o := c.options(spec, "opts", routeSourceKeys)
	s := parseSource(c, o, false)
	R := subset(c, s.nodes, stopsV, "stops")
	d := &dim{}
	col := s.col()
	k := s.register(d, col, R, sh)
	d.finish(float64(len(s.nodes.ids)))
	ctx, done := routeContext(sh, c, 0)
	defer done()
	m := k.dense(ctx, sh, R)
	n := len(R)
	if format == "rows" {
		sh.Limits().CheckAlloc(int64(n)*int64(n)*600, fmt.Sprintf("Route.matrix: %d rows", n*(n-1)))
		out := make([]Value, 0, n*(n-1))
		for i := range R {
			for j := range R {
				if i != j {
					row := NewObject()
					row.Set("from", s.nodes.ids[R[i]])
					row.Set("to", s.nodes.ids[R[j]])
					row.Set(col, d.out(m.C[i*n+j]))
					out = append(out, row)
				}
			}
		}
		return &ArrayVal{Elements: out}
	}
	sh.Limits().CheckAlloc(int64(n)*int64(n)*64, fmt.Sprintf("Route.matrix: a %d x %d matrix", n, n))
	rowsOut := make([]Value, n)
	for i := range R {
		row := make([]Value, n)
		for j := range R {
			row[j] = d.out(m.C[i*n+j])
		}
		rowsOut[i] = &ArrayVal{Elements: row}
	}
	out := NewObject()
	out.Set("ids", idList(s.nodes, R))
	out.Set("matrix", &ArrayVal{Elements: rowsOut})
	return out
}

func routePath(sh *Shell, args []Value) Value {
	c := rfail("Route.path")
	if len(args) != 1 {
		panic(WrongArguments(string(c), "spec: object", args, `Route.path({edges: [{from: "a", to: "b", dist: 4}], from: "a", to: "b"})`))
	}
	o := c.options(args[0], "spec", routePathKeys)
	if _, ok := o.get("k"); ok {
		c.fail("k is a Route.paths option: Route.paths({edges, from, to, k})")
	}
	s := parseSource(c, o, true)
	if s.kind == "points" {
		c.fail("needs edges or matrix: with points only, the shortest path is the straight line. points with edges turns on A*")
	}
	fv, ok1 := o.get("from")
	tv, ok2 := o.get("to")
	if !ok1 || !ok2 {
		c.fail("needs from and to")
	}
	src, dst := s.nodes.find(c, "from", fv), s.nodes.find(c, "to", tv)
	d := &dim{}
	col := s.col()
	k := s.register(d, col, nil, sh)
	d.finish(float64(len(s.nodes.ids)))
	g := k.graph()
	var h func(int) float64
	method := "dijkstra"
	if pv, ok := o.get("points"); ok && o.boolean("heuristic", true) {
		h = astarHeuristic(c, o, s, g, pv, dst)
		method = "astar"
	}
	ctx, done := routeContext(sh, c, 0)
	defer done()
	p, found, err := routing.ShortestPath(ctx, g, src, dst, h)
	if err != nil {
		routeTimeFail(sh, c, ctx)
	}
	out := NewObject()
	out.Set("found", Bln(found))
	if !found {
		out.Set(col, Null)
		out.Set("path", Arr())
		out.Set("legs", Arr())
	} else {
		out.Set(col, d.out(p.Dist))
		out.Set("path", idList(s.nodes, p.Nodes))
		out.Set("legs", legs(s.nodes, g, p.Nodes, col, d))
	}
	out.Set("method", Str(method))
	return out
}

// legs lists [{from, to, col}] along nodes, using the cheapest arc of each pair.
func legs(ns *nodeSet, g *routing.Graph, nodes []int, col string, d *dim) *ArrayVal {
	var out []Value
	for i := 0; i+1 < len(nodes); i++ {
		w := routing.Inf
		for _, a := range g.Adj[nodes[i]] {
			if a.To == nodes[i+1] && a.W < w {
				w = a.W
			}
		}
		row := NewObject()
		row.Set("from", ns.ids[nodes[i]])
		row.Set("to", ns.ids[nodes[i+1]])
		row.Set(col, d.out(w))
		out = append(out, row)
	}
	return &ArrayVal{Elements: out}
}

// astarHeuristic is the straight-line distance to dst scaled so that it never
// exceeds an arc weight (scale = min over arcs of weight / straight line),
// which makes it consistent in any units.
func astarHeuristic(c rfail, o *ropts, s *source, g *routing.Graph, pv Value, dst int) func(int) float64 {
	ps := &source{c: c, nodes: newNodeSet(), kind: "points"}
	ps.parsePoints(o, pv)
	at := make([]int, len(s.nodes.ids))
	for i, id := range s.nodes.ids {
		j, ok := ps.nodes.idx[idKey(c, "id", id)]
		if !ok {
			c.fail("points has no row for node %s; A* needs a point for every node (or heuristic: false)", id.Inspect())
		}
		at[i] = j
	}
	scale := math.Inf(1)
	for u, arcs := range g.Adj {
		for _, a := range arcs {
			if geo := ps.pointDist(at[u], at[a.To]); geo > 0 {
				scale = math.Min(scale, a.W/geo)
			}
		}
	}
	if math.IsInf(scale, 1) {
		scale = 0
	}
	scale *= 1 - 1e-9
	t := at[dst]
	return func(v int) float64 { return scale * ps.pointDist(at[v], t) }
}

func routePaths(sh *Shell, args []Value) Value {
	c := rfail("Route.paths")
	if len(args) != 1 {
		panic(WrongArguments(string(c), "spec: object", args, `Route.paths({edges: [{from: "a", to: "b", dist: 4}], from: "a"})`))
	}
	keys := []string{"from", "to", "k"}
	for _, k := range routeSourceKeys {
		if k != "points" && k != "metric" && k != "units" {
			keys = append(keys, k)
		}
	}
	o := c.options(args[0], "spec", keys)
	s := parseSource(c, o, false)
	fv, ok := o.get("from")
	if !ok {
		c.fail("needs from")
	}
	src := s.nodes.find(c, "from", fv)
	d := &dim{}
	col := s.col()
	k := s.register(d, col, nil, sh)
	d.finish(float64(len(s.nodes.ids)))
	g := k.graph()
	ctx, done := routeContext(sh, c, 0)
	defer done()
	row := func(to int, dist float64, nodes []int) *ObjectVal {
		r := NewObject()
		r.Set("to", s.nodes.ids[to])
		r.Set(col, d.out(dist))
		r.Set("path", idList(s.nodes, nodes))
		return r
	}
	tv, hasTo := o.get("to")
	if !hasTo {
		if _, hasK := o.get("k"); hasK {
			c.fail("k needs to: the k shortest paths go to one node")
		}
		dist, prev, err := routing.ShortestFrom(ctx, g, src)
		if err != nil {
			routeTimeFail(sh, c, ctx)
		}
		var order []int
		for v := range dist {
			if v != src && !math.IsInf(dist[v], 1) {
				order = append(order, v)
			}
		}
		sort.SliceStable(order, func(a, b int) bool { return dist[order[a]] < dist[order[b]] })
		out := make([]Value, len(order))
		for i, v := range order {
			out[i] = row(v, dist[v], routing.PathTo(prev, v))
		}
		return &ArrayVal{Elements: out}
	}
	dst := s.nodes.find(c, "to", tv)
	kk := o.integer("k", 1, 1, 100)
	ps, err := routing.KShortest(ctx, g, src, dst, kk)
	if err != nil && len(ps) == 0 {
		routeTimeFail(sh, c, ctx)
	}
	out := make([]Value, len(ps))
	for i, p := range ps {
		r := row(dst, p.Dist, p.Nodes)
		r.Set("rank", Num(float64(i+1)))
		out[i] = r
	}
	return &ArrayVal{Elements: out}
}

func routeTSP(sh *Shell, args []Value) Value {
	c := rfail("Route.tsp")
	if len(args) != 1 {
		panic(WrongArguments(string(c), "spec: object", args, `Route.tsp({points: [{id: "a", x: 0, y: 0}, {id: "b", x: 3, y: 4}], start: "a"})`))
	}
	o := c.options(args[0], "spec", routeTSPKeys)
	s := parseSource(c, o, false)
	stopsV, _ := o.get("stops")
	R := subset(c, s.nodes, stopsV, "stops")
	local := map[int]int{}
	for i, v := range R {
		local[v] = i
	}
	pos := func(k string) int {
		v, ok := o.get(k)
		if !ok {
			return -1
		}
		x := s.nodes.find(c, k, v)
		i, in := local[x]
		if !in {
			c.fail("%s %s is not in stops", k, v.Inspect())
		}
		return i
	}
	p := routing.TSP{Start: max(pos("start"), 0), End: pos("end")}
	p.RoundTrip = o.boolean("roundTrip", p.End < 0)
	if p.RoundTrip && p.End >= 0 && p.End != p.Start {
		c.fail("end is for a path: give roundTrip: false, or drop end for a round trip")
	}
	if v, ok := o.get("exact"); ok {
		if b, isB := v.(*BoolVal); !isB {
			c.fail("exact must be true or false, got %s", describe(v))
		} else if b.V {
			if len(R) > routing.HeldKarpMax {
				c.fail("exact: true works up to %d nodes (Held-Karp needs 2^n memory); got %d. Drop exact for the heuristic with a bound", routing.HeldKarpMax, len(R))
			}
			p.ExactMax = routing.HeldKarpMax
			if n := len(R); n > 1 {
				sh.Limits().CheckAlloc(int64(1)<<(n-1)*int64(n-1)*9, fmt.Sprintf("Route.tsp exact: true on %d nodes", n))
			}
		} else {
			p.ExactMax = -1
		}
	}
	if it := o.integer("iterations", -2, 0, 1_000_000); it == 0 {
		p.Iterations = -1
	} else if it > 0 {
		p.Iterations = it
	}
	p.Seed = int64(o.integer("seed", 0, math.MinInt32, math.MaxInt32))
	timeLimit := o.integer("timeLimit", 0, 1, math.MaxInt32)

	d := &dim{}
	col := s.col()
	k := s.register(d, col, R, sh)
	d.finish(float64(len(s.nodes.ids)) * float64(len(R)+2))
	ctx, done := routeContext(sh, c, timeLimit)
	defer done()
	p.M = k.dense(ctx, sh, R)
	p.Integer = d.exact()
	res, err := routing.SolveTSP(ctx, p)
	if err != nil {
		c.fail("%s", err.Error())
	}
	order := make([]int, len(res.Order))
	for i, v := range res.Order {
		order[i] = R[v]
	}
	out := NewObject()
	out.Set("status", Str(res.Status.String()))
	out.Set("cost", d.out(res.Cost))
	out.Set("order", idList(s.nodes, order))
	var lg []Value
	for i := 0; i+1 < len(res.Order); i++ {
		row := NewObject()
		row.Set("from", s.nodes.ids[order[i]])
		row.Set("to", s.nodes.ids[order[i+1]])
		row.Set(col, d.out(p.M.At(res.Order[i], res.Order[i+1])))
		lg = append(lg, row)
	}
	out.Set("legs", &ArrayVal{Elements: lg})
	out.Set("bound", d.out(res.Bound))
	out.Set("gap", gapVal(res.Cost, res.Bound))
	out.Set("method", Str(res.Method))
	out.Set("iterations", Num(float64(res.Iterations)))
	stopped(sh, c, out, res.Stopped, timeLimit, res.Iterations, res.Status.String())
	return out
}

// window reads [open, close] (close null = no end) into the time dim.
func window(c rfail, what string, v Value, d *dim, open, close []float64, i int) {
	a, ok := v.(*ArrayVal)
	if !ok || len(a.Elements) != 2 {
		c.fail("%s must be [open, close] (close null = no end), got %s", what, describe(v))
	}
	if a.Elements[0] == Null {
		c.fail("%s: open must be a number (0 for no start)", what)
	}
	d.set(open, i, c.number(what+" open", a.Elements[0]))
	if a.Elements[1] == Null {
		close[i] = routing.Inf
	} else {
		d.set(close, i, c.number(what+" close", a.Elements[1]))
		if close[i] < open[i] {
			c.fail("%s closes before it opens: %s", what, a.Display())
		}
	}
}

// rowKeys refuses a row key that is one edit from a known key (a typo that
// would be ignored); other extra keys (columns from a query) are ignored.
func rowKeys(c rfail, what string, o *ObjectVal, keys []string) {
	for _, k := range o.Keys() {
		if contains(keys, k) {
			continue
		}
		if m := nearKey(k, keys); m != "" {
			c.fail("%s: unknown key %q (did you mean %q?). Keys: %s", what, k, m, joinKeys(keys))
		}
	}
}

func joinKeys(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}

func routeVRP(sh *Shell, args []Value) Value {
	c := rfail("Route.vrp")
	if len(args) != 1 {
		panic(WrongArguments(string(c), "spec: object", args, `Route.vrp({points: pts, depot: "d", stops: ["a", "b"], vehicles: 2, capacity: 5})`))
	}
	o := c.options(args[0], "spec", routeVRPKeys)
	s := parseSource(c, o, false)
	ns := s.nodes
	dv, ok := o.get("depot")
	if !ok {
		c.fail("needs depot: the id where vehicles start and end")
	}
	depot := ns.find(c, "depot", dv)
	D, T, L := &dim{}, &dim{}, &dim{}

	// Vehicles.
	vv, ok := o.get("vehicles")
	if !ok {
		c.fail("needs vehicles: a count (with capacity) or rows [{id, capacity?, maxDist?, maxTime?, start?, end?, window?}]")
	}
	var vids []Value
	var vstart, vend []int
	var vcap, vdist, vtime, vopen, vclose []float64
	addVeh := func(id Value) {
		vids = append(vids, id)
		vstart, vend = append(vstart, depot), append(vend, depot)
		vcap, vdist, vtime = append(vcap, routing.Inf), append(vdist, routing.Inf), append(vtime, routing.Inf)
		vopen, vclose = append(vopen, 0), append(vclose, routing.Inf)
	}
	vehKeys := []string{"id", "capacity", "maxDist", "maxTime", "start", "end", "window"}
	switch x := vv.(type) {
	case *NumberVal:
		if x.V != math.Trunc(x.V) || x.V < 1 || x.V > 1000 {
			c.fail("vehicles must be a count from 1 to 1000 or rows, got %s", x.Display())
		}
		for i := 0; i < int(x.V); i++ {
			addVeh(Num(float64(i + 1)))
		}
		if cv, ok := o.get("capacity"); ok {
			n := c.number("capacity", cv)
			for i := range vcap {
				L.set(vcap, i, n)
			}
		}
	case *ArrayVal:
		if len(x.Elements) == 0 || len(x.Elements) > 1000 {
			c.fail("vehicles must have 1 to 1000 rows, got %d", len(x.Elements))
		}
		if _, ok := o.get("capacity"); ok {
			c.fail("capacity applies to vehicles: n; with vehicle rows give each row a capacity")
		}
		seen := map[string]bool{}
		for i, r := range x.Elements {
			where := fmt.Sprintf("vehicles[%d]", i)
			ro, isObj := r.(*ObjectVal)
			if !isObj {
				c.fail("%s must be an object {id, capacity?, maxDist?, maxTime?, start?, end?, window?}, got %s", where, describe(r))
			}
			for _, k := range ro.Keys() {
				if !contains(vehKeys, k) {
					c.fail("%s: unknown key %q. Keys: %s", where, k, joinKeys(vehKeys))
				}
			}
			id := Value(Num(float64(i + 1)))
			if v, ok := ro.Get("id"); ok && v != Null {
				id = v
			}
			key := idKey(c, where+" id", id)
			if seen[key] {
				c.fail("%s: duplicate vehicle id %s", where, id.Inspect())
			}
			seen[key] = true
			addVeh(id)
			get := func(k string) (Value, bool) {
				v, ok := ro.Get(k)
				return v, ok && v != Null
			}
			if v, ok := get("capacity"); ok {
				L.set(vcap, i, c.number(where+" capacity", v))
			}
			if v, ok := get("maxDist"); ok {
				D.set(vdist, i, c.number(where+" maxDist", v))
			}
			if v, ok := get("maxTime"); ok {
				T.set(vtime, i, c.number(where+" maxTime", v))
			}
			if v, ok := get("start"); ok {
				vstart[i] = ns.find(c, where+" start", v)
			}
			if v, ok := get("end"); ok {
				vend[i] = ns.find(c, where+" end", v)
			}
			if v, ok := get("window"); ok {
				window(c, where+" window", v, T, vopen, vclose, i)
			}
		}
	default:
		c.fail("vehicles must be a count or rows, got %s", describe(vv))
	}
	D.add(vdist)
	T.add(vtime)
	T.add(vopen)
	T.add(vclose)
	L.add(vcap)
	isDepot := map[int]bool{depot: true}
	for i := range vstart {
		isDepot[vstart[i]], isDepot[vend[i]] = true, true
	}

	// Stops.
	var snode []int
	var demand, sopen, sclose, service []float64
	stopKeys := []string{"id", "demand", "window", "service"}
	addStop := func(where string, node int) {
		if isDepot[node] {
			c.fail("%s: %s is a depot (a vehicle start or end), not a stop", where, ns.ids[node].Inspect())
		}
		snode = append(snode, node)
		demand, service = append(demand, 0), append(service, 0)
		sopen, sclose = append(sopen, 0), append(sclose, routing.Inf)
	}
	if sv, ok := o.get("stops"); ok {
		rows := rowsOf(c, "stops", sv)
		seen := map[int]bool{}
		for i, r := range rows {
			where := fmt.Sprintf("stops[%d]", i)
			ro, isObj := r.(*ObjectVal)
			idv := r
			if isObj {
				rowKeys(c, where, ro, stopKeys)
				v, ok := ro.Get("id")
				if !ok {
					c.fail("%s has no id. Row: %s", where, ro.Display())
				}
				idv = v
			}
			node := ns.find(c, where+" id", idv)
			if seen[node] {
				c.fail("%s: duplicate stop %s", where, idv.Inspect())
			}
			seen[node] = true
			addStop(where, node)
			if !isObj {
				continue
			}
			k := len(snode) - 1
			if v, ok := ro.Get("demand"); ok && v != Null {
				L.set(demand, k, c.number(where+" demand", v))
			}
			if v, ok := ro.Get("service"); ok && v != Null {
				T.set(service, k, c.number(where+" service", v))
			}
			if v, ok := ro.Get("window"); ok && v != Null {
				window(c, where+" window", v, T, sopen, sclose, k)
			}
		}
	} else {
		for v := range ns.ids {
			if !isDepot[v] {
				addStop("stops", v)
			}
		}
	}
	L.add(demand)
	T.add(service)
	T.add(sopen)
	T.add(sclose)

	// Nodes of the matrix: depots first, then stops.
	var R []int
	local := map[int]int{}
	addR := func(v int) {
		if _, ok := local[v]; !ok {
			local[v] = len(R)
			R = append(R, v)
		}
	}
	addR(depot)
	for i := range vstart {
		addR(vstart[i])
		addR(vend[i])
	}
	for _, v := range snode {
		addR(v)
	}
	objective := o.str("objective", "distance", "distance", "time", "vehicles")
	timeLimit := o.integer("timeLimit", 0, 1, math.MaxInt32)
	nLegs := float64(len(R) + 2)
	pathTerms := 1.0
	if s.kind == "edges" {
		pathTerms = float64(len(ns.ids))
	}

	// Distance.
	distCol := "dist"
	if s.kind == "edges" && !s.hasDist {
		distCol = "time"
	}
	dk := s.register(D, distCol, R, sh)
	D.finish(nLegs * pathTerms)
	ctx, done := routeContext(sh, c, timeLimit)
	defer done()
	distM := dk.dense(ctx, sh, R)

	// Time: times matrix, edge time column, dist / speed, or dist.
	var timeM *routing.Matrix
	var tk *costs
	var perm []int // times: ns index -> times matrix index
	tv, hasTimes := o.get("times")
	speedV, hasSpeed := o.get("speed")
	switch {
	case hasTimes && hasSpeed:
		c.fail("give times or speed, not both")
	case hasTimes:
		ts := &source{c: c, nodes: newNodeSet(), kind: "matrix"}
		ts.parseMatrix(tv, nil)
		if len(ts.nodes.ids) != len(ns.ids) {
			c.fail("times has %d nodes; the costs have %d. Give times as {ids, matrix} or in the same order", len(ts.nodes.ids), len(ns.ids))
		}
		// times with ids map by id; a plain [[...]] is in the order of the costs.
		_, withIDs := tv.(*ObjectVal)
		perm = make([]int, len(ns.ids))
		for i, id := range ns.ids {
			perm[i] = i
			if withIDs {
				j, ok := ts.nodes.idx[idKey(c, "id", id)]
				if !ok {
					c.fail("times has no id %s", id.Inspect())
				}
				perm[i] = j
			}
		}
		fillMatrix(T, ts.m, ts.mrows)
		ts.nodes = ns
		tk = &costs{s: ts, col: "time"}
	case s.kind == "edges" && s.hasTime && distCol == "dist":
		tk = s.register(T, "time", R, sh)
	default:
		timeM = &routing.Matrix{N: distM.N, C: make([]float64, len(distM.C))}
		T.add(timeM.C)
		div := new(big.Rat).SetInt64(1)
		if D.exact() {
			div.SetInt(D.scale)
		}
		speedExact := true
		if hasSpeed {
			sp := c.number("speed", speedV)
			if sp.V <= 0 {
				c.fail("speed must be > 0 (distance per time unit)")
			}
			if r, ok := sp.Exact(); ok {
				div.Mul(div, r)
			} else {
				speedExact = false
				div.Mul(div, new(big.Rat).SetFloat64(sp.V))
			}
		}
		divF, _ := div.Float64()
		for i, x := range distM.C {
			if D.exact() && speedExact {
				T.setRat(timeM.C, i, new(big.Rat).Quo(new(big.Rat).SetInt64(int64(x)), div))
			} else {
				T.setFloat(timeM.C, i, x/divF)
			}
		}
	}
	T.finish(4 * nLegs * pathTerms)
	L.finish(float64(len(snode) + 1))
	if perm != nil {
		tm, n := tk.s.m, len(perm)
		re := &routing.Matrix{N: n, C: make([]float64, n*n)}
		for i := range perm {
			for k := range perm {
				re.C[i*n+k] = tm.C[perm[i]*n+perm[k]]
			}
		}
		tk.s.m = re
	}
	if timeM == nil {
		timeM = tk.dense(ctx, sh, R)
	}

	p := &routing.VRP{Dist: distM, Time: timeM, Integer: D.exact() && T.exact() && L.exact()}
	p.Objective = map[string]routing.Objective{"distance": routing.MinDistance, "time": routing.MinTime, "vehicles": routing.MinVehicles}[objective]
	for i, v := range snode {
		p.Stops = append(p.Stops, routing.Stop{Node: local[v], Demand: demand[i], Open: sopen[i], Close: sclose[i], Service: service[i]})
	}
	for i := range vids {
		p.Vehicles = append(p.Vehicles, routing.Vehicle{Capacity: vcap[i], MaxDist: vdist[i], MaxTime: vtime[i],
			Start: local[vstart[i]], End: local[vend[i]], Open: vopen[i], Close: vclose[i]})
	}
	if it := o.integer("iterations", -2, 0, 1_000_000); it == 0 {
		p.Iterations = -1
	} else if it > 0 {
		p.Iterations = it
	}
	p.Seed = int64(o.integer("seed", 0, math.MinInt32, math.MaxInt32))
	res, err := routing.SolveVRP(ctx, p)
	if err != nil {
		c.fail("%s", err.Error())
	}

	out := NewObject()
	out.Set("status", Str(res.Status.String()))
	out.Set("objective", Str(objective))
	costDim := map[string]*dim{"distance": D, "time": T}[objective]
	if costDim == nil {
		out.Set("cost", Num(res.Cost))
	} else {
		out.Set("cost", costDim.out(res.Cost))
	}
	routes := make([]Value, 0, len(res.Routes))
	for _, rt := range res.Routes {
		row := NewObject()
		row.Set("vehicle", vids[rt.Vehicle])
		ids := make([]Value, len(rt.Stops))
		arr := make([]Value, len(rt.Stops))
		for k, i := range rt.Stops {
			ids[k] = ns.ids[snode[i]]
			arr[k] = T.out(rt.Arrive[k])
		}
		row.Set("stops", &ArrayVal{Elements: ids})
		row.Set("load", L.out(rt.Load))
		row.Set("dist", D.out(rt.Dist))
		row.Set("time", T.out(rt.Time))
		row.Set("arrivals", &ArrayVal{Elements: arr})
		row.Set("depart", T.out(rt.Depart))
		row.Set("back", T.out(rt.Back))
		routes = append(routes, row)
	}
	out.Set("routes", &ArrayVal{Elements: routes})
	un := make([]Value, 0, len(res.Unassigned))
	for _, u := range res.Unassigned {
		row := NewObject()
		row.Set("id", ns.ids[snode[u.Stop]])
		numDim := T
		if u.Why == routing.OverCapacity {
			numDim = L
		}
		row.Set("reason", Str(u.Reason(func(f float64) string { return numDim.out(f).Display() })))
		un = append(un, row)
	}
	out.Set("unassigned", &ArrayVal{Elements: un})
	out.Set("dist", D.out(res.Dist))
	out.Set("time", T.out(res.Time))
	out.Set("vehicles", Num(float64(len(res.Routes))))
	switch {
	case res.Status == routing.Infeasible:
		out.Set("bound", Null)
		out.Set("gap", Null)
	case costDim == nil:
		out.Set("bound", Num(res.Bound))
		out.Set("gap", gapVal(res.Cost, res.Bound))
	default:
		out.Set("bound", costDim.out(res.Bound))
		out.Set("gap", gapVal(res.Cost, res.Bound))
	}
	out.Set("iterations", Num(float64(res.Iterations)))
	stopped(sh, c, out, res.Stopped, timeLimit, res.Iterations, res.Status.String())
	return out
}
