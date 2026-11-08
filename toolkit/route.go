package toolkit

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/iodesystems/mcpshell/routing"
	. "github.com/iodesystems/mcpshell/runtime"
)

// AddRoute registers the routing toolkit as the loadable "route" (namespace
// Route). It has no outside effect, so it needs no flag.
func AddRoute(sh *Shell) *Shell {
	return sh.AddLoadable(Loadable{
		Name:    "route",
		Summary: "routing: distance matrix, shortest path, TSP, vehicle routing with capacity and time windows (Route.*)",
		Guide:   routeGuide,
		Install: func(sh *Shell) (io.Closer, error) {
			for _, c := range routeCommands(sh) {
				sh.Register(c)
			}
			return nil, nil
		},
	})
}

const routeGuide = `Route — distances, shortest paths, TSP and vehicle routing. load("route") first.
Costs come from points [{id, lat, lon}] (haversine, meters) or [{id, x, y}] (euclidean;
metric: "manhattan"), a matrix [[...]] or {ids, matrix}, or edges [{from, to, dist|time}].
Edges are two-way unless directed: true. A missing pair costs the shortest path through others.
No road network built in: compose one (e.g. an OSRM table via --connect) into a matrix.
status: "optimal" = proven. "feasible" = valid plan, not proven: bound <= best possible cost,
gap = (cost - bound) / cost. "infeasible" = some stops are in unassigned, each with a reason
(bound and gap are null).

TYPICAL: Matrix from points, as rows for Db
  Route.matrix([{id: "a", lat: 52.52, lon: 13.40}, {id: "b", lat: 48.14, lon: 11.58}], {units: "km"})
  // {ids: ["a", "b"], matrix: [[0, 503.83...], [503.83..., 0]]}; {format: "rows"} gives [{from, to, dist}]
TYPICAL: Shortest path
  Route.path({edges: [{from: "a", to: "b", dist: 4}, {from: "b", to: "c", dist: 3}], from: "a", to: "c"})
  // {found: true, dist: 7, path: ["a", "b", "c"], legs: [...]}. points: rows -> A* (same answer, faster)
TYPICAL: TSP (exact up to 16 stops, else heuristic with a bound)
  let r = Route.tsp({points: shops, start: "depot"})      // roundTrip: false, end: id for a path
  r.order    // ["depot", "s3", "s1", ..., "depot"]: a round trip ends where it starts
TYPICAL: VRP with capacity and time windows (lat/lon in km, 0.5 km per minute: windows in minutes)
  Route.vrp({points: sites, units: "km", speed: 0.5, depot: "hub",
    stops: [{id: "s1", demand: 4, window: [480, 600], service: 10}, {id: "s2", demand: 2}],
    vehicles: [{id: "van1", capacity: 10, maxTime: 480}, {id: "van2", capacity: 6}]})
  // {status, cost, routes: [{vehicle, stops, load, dist, time, arrivals, depart, back}], unassigned, bound, gap}
TYPICAL: From a query
  let pts = Db.query("SELECT id, lat, lon FROM shops")
  let r = Route.tsp({points: pts, start: pts[0].id}); Db.load("plan", r.legs)

TRAPS
  ids must be unique. Window, service, maxTime use the time units: speed or times: matrix,
  else the distance units. Haversine distances are float64; exact inputs give exact costs.
  vehicles: n with capacity: c for n equal vehicles. objective: "distance" | "time" | "vehicles".
  The eval time limit stops a search and returns the best found (status "feasible", stopped: "time").
  Search is seeded (seed: n) and repeatable unless the time limit stops it.`

var (
	routeSourceKeys = []string{"points", "matrix", "ids", "edges", "metric", "units", "directed", "by"}
	routeMatrixKeys = []string{"metric", "units", "directed", "by", "format", "stops"}
	routePathKeys   = append([]string{"from", "to", "heuristic", "k"}, routeSourceKeys...)
	routeTSPKeys    = append([]string{"start", "end", "roundTrip", "stops", "exact", "iterations", "seed", "timeLimit"}, routeSourceKeys...)
	routeVRPKeys    = append([]string{"depot", "stops", "vehicles", "capacity", "times", "speed", "objective", "iterations", "seed", "timeLimit"}, routeSourceKeys...)
)

func routeCommands(sh *Shell) []*CommandDef {
	return []*CommandDef{
		{Namespace: "Route", Name: "matrix",
			Signature:   "rows: [{id, lat, lon}] | [{id, x, y}] | [{from, to, dist|time}], opts?: {units?, metric?, format?, stops?, directed?, by?}",
			Description: "distance matrix: {ids, matrix}, or rows [{from, to, dist}] with format: \"rows\"",
			Examples:    []string{`Route.matrix([{id: "a", x: 0, y: 0}, {id: "b", x: 3, y: 4}])`},
			Fn:          func(args []Value) Value { return routeMatrix(sh, args) }},
		{Namespace: "Route", Name: "path",
			Signature:   "spec: {edges|matrix, from, to, points?, heuristic?, directed?, by?}",
			Description: "shortest path (Dijkstra; A* when points give coordinates): {found, dist, path, legs, method}",
			Examples:    []string{`Route.path({edges: [{from: "a", to: "b", dist: 4}, {from: "b", to: "c", dist: 3}], from: "a", to: "c"})`},
			Fn:          func(args []Value) Value { return routePath(sh, args) }},
		{Namespace: "Route", Name: "paths",
			Signature:   "spec: {edges|matrix, from, to?, k?, directed?, by?}",
			Description: "rows [{to, dist, path}]: shortest paths from one node to every reachable node, or the k shortest loopless paths to one node (rank 1..k)",
			Examples:    []string{`Route.paths({edges: [{from: "a", to: "b", dist: 4}, {from: "a", to: "c", dist: 9}, {from: "b", to: "c", dist: 3}], from: "a", to: "c", k: 2})`},
			Fn:          func(args []Value) Value { return routePaths(sh, args) }},
		{Namespace: "Route", Name: "tsp",
			Signature:   "spec: {points|matrix|edges, start?, end?, roundTrip?, stops?, exact?, iterations?, seed?, timeLimit?}",
			Description: "shortest tour or path through all nodes (or stops): {status, cost, order, legs, bound, gap, method}",
			Examples:    []string{`Route.tsp({points: [{id: "a", x: 0, y: 0}, {id: "b", x: 4, y: 0}, {id: "c", x: 4, y: 3}, {id: "d", x: 0, y: 3}], start: "a"})`},
			Fn:          func(args []Value) Value { return routeTSP(sh, args) }},
		{Namespace: "Route", Name: "vrp",
			Signature:   "spec: {depot, stops, vehicles, points|matrix|edges, capacity?, times?, speed?, objective?, iterations?, seed?, timeLimit?}",
			Description: "vehicle routes with capacity, time windows, maxDist, maxTime: {status, cost, routes, unassigned, bound, gap}",
			Examples:    []string{`Route.vrp({points: [{id: "d", x: 0, y: 0}, {id: "a", x: 5, y: 0}, {id: "b", x: 0, y: 5}], depot: "d", stops: [{id: "a", demand: 3}, {id: "b", demand: 4}], vehicles: 2, capacity: 5})`},
			Fn:          func(args []Value) Value { return routeVRP(sh, args) }},
	}
}

// rfail raises a Route.* error.
type rfail string

func (c rfail) fail(f string, a ...any) { panic(Runtime(string(c) + ": " + fmt.Sprintf(f, a...))) }

// ropts reads an option object, refusing unknown keys.
type ropts struct {
	c rfail
	o *ObjectVal
}

func (c rfail) options(v Value, what string, keys []string) *ropts {
	o, ok := v.(*ObjectVal)
	if !ok {
		c.fail("%s must be an object, got %s", what, describe(v))
	}
	for _, k := range o.Keys() {
		if !contains(keys, k) {
			hint := ""
			if m := nearKey(k, keys); m != "" {
				hint = fmt.Sprintf(" (did you mean %q?)", m)
			}
			c.fail("unknown key %q%s. Keys: %s", k, hint, strings.Join(keys, ", "))
		}
	}
	return &ropts{c, o}
}

// nearKey returns the key one edit from k, or "".
func nearKey(k string, keys []string) string {
	for _, c := range keys {
		if strings.EqualFold(c, k) || (len(c) >= 4 && len(k) >= 4 && editOne(strings.ToLower(c), strings.ToLower(k))) {
			return c
		}
	}
	return ""
}

func editOne(a, b string) bool {
	if a == b {
		return false
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	if len(a)-len(b) > 1 {
		return false
	}
	i := 0
	for i < len(b) && a[i] == b[i] {
		i++
	}
	if len(a) == len(b) {
		if a[i+1:] == b[i+1:] {
			return true
		}
		// transposition
		return i+1 < len(a) && a[i] == b[i+1] && a[i+1] == b[i] && a[i+2:] == b[i+2:]
	}
	return a[i+1:] == b[i:]
}

func (o *ropts) get(k string) (Value, bool) {
	v, ok := o.o.Get(k)
	if !ok || v == Null {
		return nil, false
	}
	return v, true
}

func (o *ropts) str(k, def string, allowed ...string) string {
	v, ok := o.get(k)
	if !ok {
		return def
	}
	s, isStr := v.(*StringVal)
	if !isStr || !contains(allowed, s.V) {
		o.c.fail("%s must be %s, got %s", k, quoteList(allowed), describe(v))
	}
	return s.V
}

func quoteList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = fmt.Sprintf("%q", x)
	}
	return strings.Join(q, " | ")
}

func (o *ropts) boolean(k string, def bool) bool {
	v, ok := o.get(k)
	if !ok {
		return def
	}
	b, isB := v.(*BoolVal)
	if !isB {
		o.c.fail("%s must be true or false, got %s", k, describe(v))
	}
	return b.V
}

func (o *ropts) integer(k string, def, lo, hi int) int {
	v, ok := o.get(k)
	if !ok {
		return def
	}
	n, isNum := v.(*NumberVal)
	if !isNum || n.V != math.Trunc(n.V) || n.V < float64(lo) || n.V > float64(hi) {
		o.c.fail("%s must be an integer from %d to %d, got %s", k, lo, hi, describe(v))
	}
	return int(n.V)
}

// number reads a number >= 0.
func (c rfail) number(what string, v Value) *NumberVal {
	n, ok := v.(*NumberVal)
	if !ok || math.IsNaN(n.V) {
		c.fail("%s must be a number, got %s", what, describe(v))
	}
	if n.V < 0 {
		c.fail("%s must be >= 0, got %s", what, n.Display())
	}
	return n
}

// nodeSet maps ids (strings or numbers) to indexes, keeping the original values.
type nodeSet struct {
	ids []Value
	idx map[string]int
}

func newNodeSet() *nodeSet { return &nodeSet{idx: map[string]int{}} }

func idKey(c rfail, what string, v Value) string {
	switch x := v.(type) {
	case *StringVal:
		if x.V == "" {
			c.fail("%s is an empty string", what)
		}
		return x.V
	case *NumberVal:
		return x.Display()
	}
	c.fail("%s must be a string or number id, got %s", what, describe(v))
	return ""
}

// intern returns the index of id, adding it when new.
func (n *nodeSet) intern(c rfail, what string, v Value) int {
	k := idKey(c, what, v)
	if i, ok := n.idx[k]; ok {
		return i
	}
	n.idx[k] = len(n.ids)
	n.ids = append(n.ids, v)
	return n.idx[k]
}

// find returns the index of an existing id.
func (n *nodeSet) find(c rfail, what string, v Value) int {
	k := idKey(c, what, v)
	i, ok := n.idx[k]
	if !ok {
		c.fail("%s %s is not a node%s", what, v.Inspect(), n.sample())
	}
	return i
}

func (n *nodeSet) sample() string {
	var s []string
	for i, v := range n.ids {
		if i == 8 {
			s = append(s, "...")
			break
		}
		s = append(s, v.Inspect())
	}
	return " (ids: " + strings.Join(s, ", ") + ")"
}

// source is where costs come from: points, a dense matrix or edge rows.
type source struct {
	c     rfail
	nodes *nodeSet
	kind  string // "points", "matrix", "edges"
	// points
	geo    bool // lat/lon (haversine) else x/y
	xs, ys []float64
	metric string
	unit   float64 // meters per output unit (haversine)
	// matrix: raw values (Inf = no arc); mrows are the input rows.
	m     *routing.Matrix
	mrows []Value
	// edges: arcs with both directions expanded; arcRow[k] is the input row of arc k.
	from, to, arcRow []int
	erows            []*ObjectVal
	hasDist, hasTime bool
	by               string // "dist", "time" or "" (default)
}

// parseSource reads points, matrix (with ids) or edges from o. With
// pointsExtra, points next to edges or matrix are coordinates for A*, not
// the costs.
func parseSource(c rfail, o *ropts, pointsExtra bool) *source {
	var have []string
	for _, k := range []string{"edges", "matrix", "points"} {
		if _, ok := o.get(k); ok {
			have = append(have, k)
		}
	}
	extra := pointsExtra && len(have) == 2 && have[1] == "points"
	if extra {
		have = have[:1]
	}
	if len(have) > 1 {
		c.fail("give one of points, matrix, edges (got %s)", strings.Join(have, " and "))
	}
	if len(have) == 0 {
		c.fail("needs points: [{id, lat, lon}] or [{id, x, y}], matrix: [[...]], or edges: [{from, to, dist}]")
	}
	kind := have[0]
	v, _ := o.get(kind)
	s := &source{c: c, nodes: newNodeSet(), kind: kind}
	s.by = o.str("by", "", "dist", "time")
	switch kind {
	case "points":
		s.parsePoints(o, v)
	case "matrix":
		ids, _ := o.get("ids")
		s.parseMatrix(v, ids)
	case "edges":
		s.parseEdges(v, o.boolean("directed", false))
	}
	if kind != "points" && !extra {
		for _, k := range []string{"metric", "units"} {
			if _, ok := o.get(k); ok {
				c.fail("%s applies to points, not %s", k, kind)
			}
		}
	}
	if _, ok := o.get("directed"); ok && kind != "edges" {
		c.fail("directed applies to edges, not %s", kind)
	}
	if _, ok := o.get("ids"); ok && kind != "matrix" {
		c.fail("ids applies to matrix: [[...]], not %s", kind)
	}
	if s.by == "time" && kind != "edges" {
		c.fail("by applies to edges with a time column")
	}
	return s
}

func rowsOf(c rfail, what string, v Value) []Value {
	a, ok := v.(*ArrayVal)
	if !ok {
		c.fail("%s must be an array of rows, got %s", what, describe(v))
	}
	if len(a.Elements) == 0 {
		c.fail("%s is empty", what)
	}
	return a.Elements
}

func (s *source) parsePoints(o *ropts, v Value) {
	c := s.c
	rows := rowsOf(c, "points", v)
	if len(rows) > routing.MaxNodes {
		c.fail("%d points is over the limit of %d (the matrix is n x n)", len(rows), routing.MaxNodes)
	}
	for i, r := range rows {
		where := fmt.Sprintf("points[%d]", i)
		ro, ok := r.(*ObjectVal)
		if !ok {
			c.fail("%s must be an object {id, lat, lon} or {id, x, y}, got %s", where, describe(r))
		}
		idv, ok := ro.Get("id")
		if !ok {
			c.fail("%s has no id. Row: %s", where, ro.Display())
		}
		before := len(s.nodes.ids)
		s.nodes.intern(c, where+" id", idv)
		if len(s.nodes.ids) == before {
			c.fail("%s: duplicate id %s", where, idv.Inspect())
		}
		lat, hasLat := ro.Get("lat")
		lon, hasLon := ro.Get("lon")
		if !hasLon {
			lon, hasLon = ro.Get("lng")
		}
		x, hasX := ro.Get("x")
		y, hasY := ro.Get("y")
		geo := hasLat || hasLon
		switch {
		case geo && !(hasLat && hasLon):
			c.fail("%s needs both lat and lon. Row: %s", where, ro.Display())
		case !geo && !(hasX && hasY):
			c.fail("%s needs lat and lon, or x and y. Row: %s", where, ro.Display())
		case geo && (hasX || hasY):
			c.fail("%s has both lat/lon and x/y; use one. Row: %s", where, ro.Display())
		}
		if i == 0 {
			s.geo = geo
		} else if geo != s.geo {
			c.fail("%s: mix of lat/lon and x/y points", where)
		}
		a, b := lat, lon
		if !geo {
			a, b = x, y
		}
		an, ok1 := a.(*NumberVal)
		bn, ok2 := b.(*NumberVal)
		if !ok1 || !ok2 {
			c.fail("%s: coordinates must be numbers. Row: %s", where, ro.Display())
		}
		if geo && (math.Abs(an.V) > 90 || math.Abs(bn.V) > 180) {
			c.fail("%s: lat must be in -90..90 and lon in -180..180, got %s, %s", where, an.Display(), bn.Display())
		}
		s.xs, s.ys = append(s.xs, an.V), append(s.ys, bn.V)
	}
	if s.geo {
		s.metric = o.str("metric", "haversine", "haversine")
		s.unit = map[string]float64{"m": 1, "km": 1000, "mi": 1609.344}[o.str("units", "m", "m", "km", "mi")]
	} else {
		s.metric = o.str("metric", "euclidean", "euclidean", "manhattan")
		if _, ok := o.get("units"); ok {
			c.fail("units applies to lat/lon points; x/y distances are in the x/y units")
		}
	}
}

// pointDist is the metric distance between points i and j.
func (s *source) pointDist(i, j int) float64 {
	switch s.metric {
	case "haversine":
		return routing.Haversine(s.xs[i], s.ys[i], s.xs[j], s.ys[j]) / s.unit
	case "manhattan":
		return routing.Manhattan(s.xs[i], s.ys[i], s.xs[j], s.ys[j])
	}
	return routing.Euclidean(s.xs[i], s.ys[i], s.xs[j], s.ys[j])
}

// matrixRows reads a matrix: [[...]] or {ids, matrix}.
func (s *source) parseMatrix(v Value, ids Value) {
	c := s.c
	if o, ok := v.(*ObjectVal); ok {
		mv, hasM := o.Get("matrix")
		iv, hasI := o.Get("ids")
		if !hasM || !hasI || o.Len() != 2 {
			c.fail("matrix must be [[...]] or {ids, matrix} (the Route.matrix result), got keys %s", strings.Join(o.Keys(), ", "))
		}
		if ids != nil {
			c.fail("ids given twice: in matrix and as ids")
		}
		v, ids = mv, iv
	}
	rows := rowsOf(c, "matrix", v)
	n := len(rows)
	if n > routing.MaxNodes {
		c.fail("a %d x %d matrix is over the limit of %d", n, n, routing.MaxNodes)
	}
	if ids != nil {
		ia, ok := ids.(*ArrayVal)
		if !ok || len(ia.Elements) != n {
			c.fail("ids must be an array of %d ids (one per matrix row), got %s", n, describe(ids))
		}
		for i, id := range ia.Elements {
			before := len(s.nodes.ids)
			s.nodes.intern(c, fmt.Sprintf("ids[%d]", i), id)
			if len(s.nodes.ids) == before {
				c.fail("ids[%d]: duplicate id %s", i, id.Inspect())
			}
		}
	} else {
		for i := 0; i < n; i++ {
			s.nodes.intern(c, "", Num(float64(i)))
		}
	}
	s.m = s.readMatrix("matrix", rows, n)
	s.mrows = rows
}

// readMatrix reads n rows of n numbers (null = no arc) into a Matrix of the
// raw values. The values are classified later (dim.set on each).
func (s *source) readMatrix(what string, rows []Value, n int) *routing.Matrix {
	c := s.c
	m := &routing.Matrix{N: n, C: make([]float64, n*n)}
	for i, r := range rows {
		ra, ok := r.(*ArrayVal)
		if !ok || len(ra.Elements) != n {
			c.fail("%s[%d] must be an array of %d numbers (the matrix is square), got %s", what, i, n, describe(r))
		}
		for j, e := range ra.Elements {
			if e == Null {
				m.C[i*n+j] = routing.Inf
				if i == j {
					m.C[i*n+j] = 0
				}
				continue
			}
			c.number(fmt.Sprintf("%s[%d][%d]", what, i, j), e)
		}
	}
	return m
}

// fillMatrix registers the raw matrix values of rows in d.
func fillMatrix(d *dim, m *routing.Matrix, rows []Value) {
	d.add(m.C)
	n := m.N
	for i, r := range rows {
		for j, e := range r.(*ArrayVal).Elements {
			if nv, ok := e.(*NumberVal); ok {
				d.set(m.C, i*n+j, nv)
			}
		}
	}
}

func (s *source) parseEdges(v Value, directed bool) {
	c := s.c
	rows := rowsOf(c, "edges", v)
	type pair struct{ a, b int }
	given := map[pair]bool{}
	for i, r := range rows {
		where := fmt.Sprintf("edges[%d]", i)
		ro, ok := r.(*ObjectVal)
		if !ok {
			c.fail("%s must be an object {from, to, dist|time}, got %s", where, describe(r))
		}
		fv, ok1 := ro.Get("from")
		tv, ok2 := ro.Get("to")
		if !ok1 || !ok2 {
			c.fail("%s needs from and to. Row: %s", where, ro.Display())
		}
		a := s.nodes.intern(c, where+" from", fv)
		b := s.nodes.intern(c, where+" to", tv)
		hd, ht := has(ro, "dist"), has(ro, "time")
		if i == 0 {
			s.hasDist, s.hasTime = hd, ht
			if !hd && !ht {
				c.fail("%s needs dist or time. Row: %s", where, ro.Display())
			}
		} else if hd != s.hasDist || ht != s.hasTime {
			c.fail("%s: every edge needs the same columns as edges[0] (%s). Row: %s", where, s.cols(), ro.Display())
		}
		for _, col := range []string{"dist", "time"} {
			if x, ok := ro.Get(col); ok && x != Null {
				c.number(where+" "+col, x)
			}
		}
		s.from, s.to, s.arcRow = append(s.from, a), append(s.to, b), append(s.arcRow, i)
		s.erows = append(s.erows, ro)
		given[pair{a, b}] = true
	}
	if !directed {
		for k := range len(rows) {
			a, b := s.from[k], s.to[k]
			if !given[pair{b, a}] {
				s.from, s.to, s.arcRow = append(s.from, b), append(s.to, a), append(s.arcRow, k)
			}
		}
	}
	if s.by == "dist" && !s.hasDist || s.by == "time" && !s.hasTime {
		c.fail("by: %q, but the edges have no %s column", s.by, s.by)
	}
}

func has(o *ObjectVal, k string) bool {
	v, ok := o.Get(k)
	return ok && v != Null
}

// col is the cost column: by, else dist when edges have it, else time.
func (s *source) col() string {
	if s.by != "" {
		return s.by
	}
	if s.kind == "edges" && !s.hasDist {
		return "time"
	}
	return "dist"
}

// edgeWeights registers the edge column col in d and returns the arc weights.
func (s *source) edgeWeights(d *dim, col string) []float64 {
	w := make([]float64, len(s.from))
	d.add(w)
	for k, r := range s.arcRow {
		v, _ := s.erows[r].Get(col)
		d.set(w, k, v.(*NumberVal))
	}
	return w
}

func (s *source) cols() string {
	var c []string
	if s.hasDist {
		c = append(c, "dist")
	}
	if s.hasTime {
		c = append(c, "time")
	}
	return strings.Join(c, ", ")
}
