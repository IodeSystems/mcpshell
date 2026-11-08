package toolkit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/iodesystems/mcpshell/graphalg"
	"github.com/iodesystems/mcpshell/routing"
	. "github.com/iodesystems/mcpshell/runtime"
)

// Graph algorithms (Graph.path, reach, components, cycle, topo, cpm, flow,
// match, mst). Input is rows: edges [{from, to, ...}] and optional nodes
// [{id, ...}], as Db.query, Route.matrix({format: "rows"}) and the graph
// store's nodes()/edges() give them. Computation is in graphalg/ and
// routing/; this file reads and writes values.

const (
	maxGraphNodes = 1_000_000
	maxGraphEdges = 2_000_000
)

var graphCommonKeys = []string{"nodes", "edges", "directed"}

func graphKeys(extra ...string) []string { return append(extra, graphCommonKeys...) }

func graphAlgoCommands(sh *Shell) []*CommandDef {
	cmd := func(name, sig, desc, example string, fn func(*Shell, rfail, *ropts) Value) *CommandDef {
		return &CommandDef{Namespace: "Graph", Name: name, Signature: sig, Description: desc,
			Examples: []string{example},
			Fn: func(args []Value) Value {
				c := rfail("Graph." + name)
				if len(args) != 1 {
					panic(WrongArguments(string(c), sig, args, example))
				}
				return fn(sh, c, c.options(args[0], "spec", graphSpecKeys[name]))
			}}
	}
	return []*CommandDef{
		cmd("path", "spec: {edges, from, to?, weight?, directed?, longest?, nodes?}",
			"shortest (or longest: true, on a DAG) path: {found, path, edges, method, <weight col>}; without to, rows [{to, path, <weight col>}] to every reachable node",
			`Graph.path({edges: [{from: "a", to: "b", weight: 4}, {from: "b", to: "c", weight: 3}], from: "a", to: "c"})`, graphPath),
		cmd("reach", "spec: {edges, from: id|[ids], directed?, order?: \"bfs\"|\"dfs\", maxDepth?, reverse?, nodes?}",
			"nodes reachable from from, in BFS or DFS order: rows [{id, depth, parent}]",
			`Graph.reach({edges: [{from: "a", to: "b"}, {from: "b", to: "c"}], from: "a", directed: true})`, graphReach),
		cmd("components", "spec: {edges, nodes?, directed?}",
			"connected components (directed: true = strongly connected): {count, kind, components: [[ids]], nodes: [{id, component}]}",
			`Graph.components({edges: [{from: "a", to: "b"}, {from: "c", to: "d"}]})`, graphComponents),
		cmd("cycle", "spec: {edges, directed?, nodes?}",
			"finds one cycle: {found, cycle: [ids, first again], edges}",
			`Graph.cycle({edges: [{from: "a", to: "b"}, {from: "b", to: "a"}], directed: true})`, graphCycle),
		cmd("topo", "spec: {edges, nodes?}",
			"topological order of a directed graph (from before to): [ids]; a cycle is an error naming it",
			`Graph.topo({edges: [{from: "wake", to: "coffee"}, {from: "coffee", to: "work"}]})`, graphTopo),
		cmd("cpm", "spec: {nodes: [{id, duration, after?}], edges?: [{from, to, lag?}], duration?, lag?}",
			"critical path schedule: {makespan, critical: [ids], tasks: [{id, earlyStart, earlyFinish, lateStart, lateFinish, slack, critical, ...}]}",
			`Graph.cpm({nodes: [{id: "dig", duration: 3}, {id: "pour", duration: 1, after: ["dig"]}]})`, graphCPM),
		cmd("flow", "spec: {edges, from?, to?, amount?, cap?, cost?, supply?, nodes?, directed?}",
			"max flow / min cut from from to to, or min-cost flow (cost column; amount, or node supply rows): {status, value, cost, flows, cut}",
			`Graph.flow({edges: [{from: "s", to: "a", cap: 3}, {from: "a", to: "t", cap: 2}], from: "s", to: "t", directed: true})`, graphFlow),
		cmd("match", "spec: {edges: [{from: left, to: right, cost?}], cost?, maximize?}",
			"maximum bipartite matching (from = left, to = right); with a cost column the cheapest (maximize: the largest) of maximum size: {status, size, cost, pairs, unmatched}",
			`Graph.match({edges: [{from: "ann", to: "t1", cost: 3}, {from: "ann", to: "t2", cost: 1}, {from: "bob", to: "t1", cost: 2}]})`, graphMatch),
		cmd("mst", "spec: {edges, weight?, maximize?, nodes?}",
			"minimum spanning tree (a forest when disconnected): {status, <weight col>, edges, trees}",
			`Graph.mst({edges: [{from: "a", to: "b", weight: 1}, {from: "b", to: "c", weight: 2}, {from: "a", to: "c", weight: 3}]})`, graphMST),
	}
}

var graphSpecKeys = map[string][]string{
	"path":       graphKeys("from", "to", "weight", "longest"),
	"reach":      graphKeys("from", "order", "maxDepth", "reverse"),
	"components": graphKeys(),
	"cycle":      graphKeys(),
	"topo":       graphKeys(),
	"cpm":        graphKeys("duration", "lag"),
	"flow":       graphKeys("from", "to", "amount", "cap", "cost", "supply"),
	"match":      {"edges", "cost", "maximize"},
	"mst":        graphKeys("weight", "maximize"),
}

// --- input ---------------------------------------------------------------

type dirMode int

const (
	twoWay     dirMode = iota // directed defaults to false (Route's convention)
	oneWay                    // always directed; directed: false is an error
	undirected                // directed: true is an error
)

// gin is a graph read from rows.
type gin struct {
	c        rfail
	ns       *nodeSet
	nrows    []*ObjectVal // node index -> its nodes row, or nil
	erows    []*ObjectVal
	from, to []int // per edge row
	directed bool
}

func readGraph(sh *Shell, c rfail, o *ropts, mode dirMode) *gin {
	g := &gin{c: c, ns: newNodeSet()}
	switch mode {
	case twoWay:
		g.directed = o.boolean("directed", false)
	case oneWay:
		if !o.boolean("directed", true) {
			c.fail("works on directed edges only (from before to); remove directed: false")
		}
		g.directed = true
	case undirected:
		if o.boolean("directed", false) {
			c.fail("works on undirected edges only; directed: true is not supported")
		}
	}
	nv, hasNodes := o.get("nodes")
	ev, hasEdges := o.get("edges")
	if !hasNodes && !hasEdges {
		c.fail("needs edges: [{from, to, ...}] (and nodes: [{id, ...}] for nodes without edges)")
	}
	if hasNodes {
		rows := graphRows(c, "nodes", nv)
		if len(rows) > maxGraphNodes {
			c.fail("%d nodes is over the limit of %d", len(rows), maxGraphNodes)
		}
		for i, r := range rows {
			where := fmt.Sprintf("nodes[%d]", i)
			ro, ok := r.(*ObjectVal)
			if !ok {
				c.fail("%s must be an object {id, ...}, got %s", where, describe(r))
			}
			idv, ok := ro.Get("id")
			if !ok {
				c.fail("%s has no id. Row: %s", where, ro.Display())
			}
			before := len(g.ns.ids)
			g.ns.intern(c, where+" id", idv)
			if len(g.ns.ids) == before {
				c.fail("%s: duplicate id %s", where, idv.Inspect())
			}
			g.nrows = append(g.nrows, ro)
		}
	}
	if hasEdges {
		rows := graphRows(c, "edges", ev)
		if len(rows) > maxGraphEdges {
			c.fail("%d edges is over the limit of %d", len(rows), maxGraphEdges)
		}
		sh.Limits().CheckAlloc(int64(len(rows))*120, fmt.Sprintf("%s: %d edges", string(c), len(rows)))
		for i, r := range rows {
			where := fmt.Sprintf("edges[%d]", i)
			ro, ok := r.(*ObjectVal)
			if !ok {
				c.fail("%s must be an object {from, to, ...}, got %s", where, describe(r))
			}
			fv, ok1 := ro.Get("from")
			tv, ok2 := ro.Get("to")
			if !ok1 || !ok2 {
				c.fail("%s needs from and to. Row: %s", where, ro.Display())
			}
			g.from = append(g.from, g.ns.intern(c, where+" from", fv))
			g.to = append(g.to, g.ns.intern(c, where+" to", tv))
			g.erows = append(g.erows, ro)
		}
	}
	if len(g.ns.ids) > maxGraphNodes {
		c.fail("%d nodes is over the limit of %d", len(g.ns.ids), maxGraphNodes)
	}
	for len(g.nrows) < len(g.ns.ids) {
		g.nrows = append(g.nrows, nil)
	}
	return g
}

// graphRows reads an array of rows; empty is allowed (a graph may have no edges).
func graphRows(c rfail, what string, v Value) []Value {
	a, ok := v.(*ArrayVal)
	if !ok {
		c.fail("%s must be an array of rows, got %s", what, describe(v))
	}
	return a.Elements
}

func (g *gin) n() int { return len(g.ns.ids) }

// arcs expands the edge rows into arcs: one per row when directed, else
// also the reverse of each row whose reverse pair no row gives (Route's
// rule: a given reverse row wins). row[k] is the edge row of arc k and
// rev[k] marks the added reverse arcs.
func (g *gin) arcs() (from, to, row []int, rev []bool) {
	for k := range g.erows {
		from, to, row, rev = append(from, g.from[k]), append(to, g.to[k]), append(row, k), append(rev, false)
	}
	if g.directed {
		return
	}
	given := map[[2]int]bool{}
	for k := range g.erows {
		given[[2]int{g.from[k], g.to[k]}] = true
	}
	for k := range g.erows {
		a, b := g.from[k], g.to[k]
		if a != b && !given[[2]int{b, a}] {
			from, to, row, rev = append(from, b), append(to, a), append(row, k), append(rev, true)
		}
	}
	return
}

func (g *gin) adj() [][]int {
	from, to, _, _ := g.arcs()
	return graphalg.Adj(g.n(), from, to)
}

func (g *gin) ids(idx []int) *ArrayVal { return idList(g.ns, idx) }

func (g *gin) rowList(rows []int) *ArrayVal {
	out := make([]Value, len(rows))
	for i, r := range rows {
		out[i] = copyProps(g.erows[r])
	}
	return &ArrayVal{Elements: out}
}

// column picks the edge (or node) column an option names. The option may be
// a column name or, when allowConst, a number used for every row. Absent,
// the first of defaults that some row has is used. A key one edit
// from a default is an error (a typo would otherwise drop the column).
// opt: null turns a default column off. Returns "" when no column applies.
func column(c rfail, o *ropts, opt string, defaults []string, rows []*ObjectVal, allowConst bool) (col string, constant *NumberVal) {
	if v, ok := o.o.Get(opt); ok && v == Null {
		return "", nil // opt: null turns the column off
	}
	if v, ok := o.get(opt); ok {
		switch x := v.(type) {
		case *StringVal:
			return x.V, nil
		case *NumberVal:
			if allowConst {
				return "", x
			}
		}
		what := "a column name"
		if allowConst {
			what = "a column name or a number for every row"
		}
		c.fail("%s must be %s, got %s", opt, what, describe(v))
	}
	if len(rows) == 0 {
		return "", nil
	}
	for _, d := range defaults {
		for _, r := range rows {
			if r.Has(d) {
				return d, nil
			}
		}
	}
	for i, r := range rows {
		if i == 100 {
			break
		}
		for _, k := range r.Keys() {
			if m := nearKey(k, defaults); m != "" && k != m {
				c.fail("rows have %q: did you mean %q? Name the column with %s: %q", k, m, opt, k)
			}
		}
	}
	return "", nil
}

// num reads row[col] as a number (null and missing are errors).
func (g *gin) num(where string, ro *ObjectVal, col string) *NumberVal {
	v, ok := ro.Get(col)
	if !ok || v == Null {
		g.c.fail("%s has no %s. Row: %s", where, col, ro.Display())
	}
	n, isNum := v.(*NumberVal)
	if !isNum || math.IsNaN(n.V) || math.IsInf(n.V, 0) {
		g.c.fail("%s %s must be a finite number, got %s", where, col, describe(v))
	}
	return n
}

// weights reads one weight per edge row into a dim span.
func (g *gin) weights(d *dim, col string, constant *NumberVal, nonNeg bool) []float64 {
	w := make([]float64, len(g.erows))
	d.add(w)
	for k, ro := range g.erows {
		n := constant
		if n == nil {
			n = g.num(fmt.Sprintf("edges[%d]", k), ro, col)
		}
		if nonNeg && n.V < 0 {
			g.c.fail("edges[%d] %s must be >= 0, got %s", k, col, n.Display())
		}
		d.set(w, k, n)
	}
	return w
}

func weightCol(c rfail, o *ropts, g *gin) (string, *NumberVal) {
	col, k := column(c, o, "weight", []string{"weight", "dist", "time", "cost"}, g.erows, true)
	if col == "" && k == nil && len(g.erows) > 0 {
		c.fail("edges need a weight column (weight, dist, time or cost), weight: \"col\" to name one, or weight: 1 to count hops. Row: %s", g.erows[0].Display())
	}
	return col, k
}

func outKey(col string) string {
	if col == "" {
		return "dist"
	}
	return col
}

// graphCtx is routeContext with a graph error for a stop.
func graphStop(sh *Shell, c rfail, err error) {
	lm := sh.Limits()
	if lm.Cancelled() || errors.Is(err, context.Canceled) {
		c.fail("Execution cancelled")
	}
	c.fail("stopped at the time limit before a result. Raise the limit (%s), or give a smaller graph", lm.RaiseHint("timeout", lm.TimeoutMs*2))
}

// cycleText names a cycle: "a -> b -> a (edges[3], edges[5])".
func (g *gin) cycleText(cyc []int) string {
	var ids, rows []string
	for _, v := range cyc {
		ids = append(ids, g.ns.ids[v].Inspect())
	}
	ids = append(ids, g.ns.ids[cyc[0]].Inspect())
	for _, r := range g.cycleRows(cyc) {
		if r >= 0 {
			rows = append(rows, fmt.Sprintf("edges[%d]", r))
		}
	}
	s := strings.Join(ids, " -> ")
	if len(rows) > 0 {
		s += " (" + strings.Join(rows, ", ") + ")"
	}
	return s
}

// cycleRows maps each arc of a cycle to an edge row (-1 when none, e.g. an
// arc from a cpm after list).
func (g *gin) cycleRows(cyc []int) []int {
	at := map[[2]int]int{}
	for k := len(g.erows) - 1; k >= 0; k-- {
		at[[2]int{g.from[k], g.to[k]}] = k
		if !g.directed {
			if _, ok := at[[2]int{g.to[k], g.from[k]}]; !ok {
				at[[2]int{g.to[k], g.from[k]}] = k
			}
		}
	}
	out := make([]int, len(cyc))
	for i, u := range cyc {
		v := cyc[(i+1)%len(cyc)]
		r, ok := at[[2]int{u, v}]
		if !ok {
			r = -1
		}
		out[i] = r
	}
	return out
}

func (g *gin) findIDs(what string, v Value) []int {
	if a, ok := v.(*ArrayVal); ok {
		if len(a.Elements) == 0 {
			g.c.fail("%s is an empty array", what)
		}
		out := make([]int, len(a.Elements))
		for i, e := range a.Elements {
			out[i] = g.ns.find(g.c, fmt.Sprintf("%s[%d]", what, i), e)
		}
		return out
	}
	return []int{g.ns.find(g.c, what, v)}
}

// --- path ----------------------------------------------------------------

func graphPath(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, twoWay)
	longest := o.boolean("longest", false)
	if longest && !g.directed {
		c.fail("longest: true needs directed: true (a longest path on two-way edges would go back and forth)")
	}
	fv, ok := o.get("from")
	if !ok {
		c.fail("needs from (and to for one path)")
	}
	src := g.ns.find(c, "from", fv)
	dst := -1
	if tv, ok := o.get("to"); ok {
		dst = g.ns.find(c, "to", tv)
	}
	col, k := weightCol(c, o, g)
	d := &dim{}
	w := g.weights(d, col, k, false)
	d.finish(float64(g.n()))
	from, to, row, _ := g.arcs()
	negative := -1
	for a := range from {
		if w[row[a]] < 0 {
			negative = row[a]
			break
		}
	}
	if negative >= 0 && !g.directed {
		c.fail("edges[%d] has a negative weight on a two-way edge, which is a negative cycle. Pass directed: true", negative)
	}
	adj := make([][]graphalg.Arc, g.n())
	rg := routing.NewGraph(g.n())
	for a := range from {
		adj[from[a]] = append(adj[from[a]], graphalg.Arc{To: to[a], W: w[row[a]]})
		rg.AddArc(from[a], to[a], w[row[a]])
	}
	var dist []float64
	var prev []int
	method := "dijkstra"
	ctx, done := routeContext(sh, c, 0)
	defer done()
	var err error
	switch {
	case longest:
		method = "dag-longest"
		dist, prev, err = graphalg.Longest(adj, src)
		var ce *graphalg.CycleError
		if errors.As(err, &ce) {
			c.fail("longest: true needs a DAG, but the edges have a cycle: %s", g.cycleText(ce.Nodes))
		}
	case negative >= 0:
		method = "bellman-ford"
		dist, prev, err = graphalg.BellmanFord(ctx, adj, src)
		var ne *graphalg.NegativeCycleError
		if errors.As(err, &ne) {
			c.fail("negative cycle reachable from %s: %s; shortest paths are not defined", fv.Inspect(), g.cycleText(ne.Nodes))
		}
	case dst >= 0:
		var p routing.Path
		var found bool
		p, found, err = routing.ShortestPath(ctx, rg, src, dst, nil)
		if err == nil {
			dist = make([]float64, g.n())
			for i := range dist {
				dist[i] = math.Inf(1)
			}
			if found {
				dist[dst] = p.Dist
				prev = make([]int, g.n())
				for i := range prev {
					prev[i] = -1
				}
				for i := 1; i < len(p.Nodes); i++ {
					prev[p.Nodes[i]] = p.Nodes[i-1]
				}
			}
		}
	default:
		dist, prev, err = routing.ShortestFrom(ctx, rg, src)
	}
	if err != nil {
		graphStop(sh, c, err)
	}
	reached := func(v int) bool { return !math.IsInf(dist[v], 0) }
	key := outKey(col)
	// best arc row between consecutive nodes: cheapest (longest: dearest).
	pick := map[[2]int]int{}
	for a := range from {
		p := [2]int{from[a], to[a]}
		if b, ok := pick[p]; !ok || (!longest && w[row[a]] < w[b]) || (longest && w[row[a]] > w[b]) {
			pick[p] = row[a]
		}
	}
	pathOf := func(v int) ([]int, []int) {
		nodes := routing.PathTo(prev, v)
		var rows []int
		for i := 0; i+1 < len(nodes); i++ {
			rows = append(rows, pick[[2]int{nodes[i], nodes[i+1]}])
		}
		return nodes, rows
	}
	if dst >= 0 {
		out := NewObject()
		out.Set("found", Bln(reached(dst)))
		if reached(dst) {
			nodes, rows := pathOf(dst)
			out.Set(key, d.out(dist[dst]))
			out.Set("path", g.ids(nodes))
			out.Set("edges", g.rowList(rows))
		} else {
			out.Set(key, Null)
			out.Set("path", Arr())
			out.Set("edges", Arr())
		}
		out.Set("method", Str(method))
		out.Set("directed", Bln(g.directed))
		return out
	}
	var order []int
	for v := range dist {
		if v != src && reached(v) {
			order = append(order, v)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		if longest {
			return dist[order[a]] > dist[order[b]]
		}
		return dist[order[a]] < dist[order[b]]
	})
	sh.Limits().CheckAlloc(int64(len(order))*600, fmt.Sprintf("%s: %d rows", string(c), len(order)))
	rows := make([]Value, len(order))
	for i, v := range order {
		nodes, _ := pathOf(v)
		r := NewObject()
		r.Set("to", g.ns.ids[v])
		r.Set(key, d.out(dist[v]))
		r.Set("path", g.ids(nodes))
		rows[i] = r
	}
	return &ArrayVal{Elements: rows}
}

// --- reach, components, cycle, topo --------------------------------------

func graphReach(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, twoWay)
	fv, ok := o.get("from")
	if !ok {
		c.fail("needs from: an id or an array of ids")
	}
	srcs := g.findIDs("from", fv)
	dfs := o.str("order", "bfs", "bfs", "dfs") == "dfs"
	maxDepth := o.integer("maxDepth", -1, 0, math.MaxInt32)
	reverse := o.boolean("reverse", false)
	if reverse && !g.directed {
		c.fail("reverse applies to directed: true (two-way edges reach the same nodes both ways)")
	}
	from, to, _, _ := g.arcs()
	if reverse {
		from, to = to, from
	}
	visits := graphalg.Reach(graphalg.Adj(g.n(), from, to), srcs, dfs, maxDepth)
	sh.Limits().CheckAlloc(int64(len(visits))*600, fmt.Sprintf("%s: %d rows", string(c), len(visits)))
	out := make([]Value, len(visits))
	for i, v := range visits {
		r := NewObject()
		r.Set("id", g.ns.ids[v.Node])
		r.Set("depth", Num(float64(v.Depth)))
		if v.Parent < 0 {
			r.Set("parent", Null)
		} else {
			r.Set("parent", g.ns.ids[v.Parent])
		}
		out[i] = r
	}
	return &ArrayVal{Elements: out}
}

func graphComponents(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, twoWay)
	var label []int
	var count int
	kind := "connected"
	if g.directed {
		label, count = graphalg.SCC(g.adj())
		kind = "strong"
	} else {
		label, count = graphalg.Components(g.n(), g.from, g.to)
	}
	groups := make([][]int, count)
	for v, l := range label {
		groups[l] = append(groups[l], v)
	}
	// Largest first; ties by lowest node (labels are in that order already).
	sort.SliceStable(groups, func(a, b int) bool { return len(groups[a]) > len(groups[b]) })
	sh.Limits().CheckAlloc(int64(g.n())*600, fmt.Sprintf("%s: %d rows", string(c), g.n()))
	comps := make([]Value, count)
	at := make([]int, g.n())
	for i, grp := range groups {
		comps[i] = g.ids(grp)
		for _, v := range grp {
			at[v] = i
		}
	}
	nodes := make([]Value, g.n())
	for v := range nodes {
		r := NewObject()
		r.Set("id", g.ns.ids[v])
		r.Set("component", Num(float64(at[v])))
		nodes[v] = r
	}
	out := NewObject()
	out.Set("count", Num(float64(count)))
	out.Set("kind", Str(kind))
	out.Set("components", &ArrayVal{Elements: comps})
	out.Set("nodes", &ArrayVal{Elements: nodes})
	out.Set("directed", Bln(g.directed))
	return out
}

func graphCycle(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, twoWay)
	var cyc []int
	if g.directed {
		cyc = graphalg.FindCycle(g.adj())
	} else {
		// One edge per unordered pair: a row and its reverse are one edge.
		seen := map[[2]int]bool{}
		var from, to []int
		for k := range g.erows {
			a, b := g.from[k], g.to[k]
			p := [2]int{min(a, b), max(a, b)}
			if !seen[p] {
				seen[p] = true
				from, to = append(from, a), append(to, b)
			}
		}
		cyc = graphalg.FindCycleUndirected(g.n(), from, to)
	}
	out := NewObject()
	out.Set("found", Bln(cyc != nil))
	if cyc == nil {
		out.Set("cycle", Arr())
		out.Set("edges", Arr())
	} else {
		out.Set("cycle", g.ids(append(cyc, cyc[0])))
		out.Set("edges", g.rowList(g.cycleRows(cyc)))
	}
	out.Set("directed", Bln(g.directed))
	return out
}

func graphTopo(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, oneWay)
	order, err := graphalg.TopoSort(g.adj())
	var ce *graphalg.CycleError
	if errors.As(err, &ce) {
		c.fail("the edges have a cycle, so there is no order: %s. Remove or reverse one of these edges", g.cycleText(ce.Nodes))
	}
	return g.ids(order)
}

// --- cpm -----------------------------------------------------------------

func graphCPM(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, oneWay)
	// after: [ids] on node rows adds edges after[i] -> id.
	type extra struct{ from, to int }
	var afters []extra
	for v, ro := range g.nrows {
		if ro == nil {
			continue
		}
		av, ok := ro.Get("after")
		if !ok || av == Null {
			continue
		}
		list := []Value{av}
		if a, isArr := av.(*ArrayVal); isArr {
			list = a.Elements
		}
		for j, e := range list {
			where := fmt.Sprintf("nodes[%d].after[%d]", v, j)
			if _, isID := e.(*StringVal); !isID {
				if _, isNum := e.(*NumberVal); !isNum {
					c.fail("%s must be a task id, got %s", where, describe(e))
				}
			}
			k, known := g.ns.idx[idKey(c, where, e)]
			if !known {
				c.fail("%s: %s is not a task%s", where, e.Inspect(), g.ns.sample())
			}
			afters = append(afters, extra{k, v})
		}
	}
	durCol, durK := column(c, o, "duration", []string{"duration"}, nonNil(g.nrows), true)
	if durCol == "" && durK == nil {
		durCol = "duration"
	}
	lagCol, _ := column(c, o, "lag", []string{"lag"}, g.erows, false)
	_, lagGiven := o.get("lag")
	if _, opt := o.o.Get("lag"); lagCol == "" && !opt {
		lagCol = "lag" // later rows may have it although the first do not
	}
	d := &dim{}
	dur := make([]float64, g.n())
	d.add(dur)
	for v := range dur {
		if durK != nil {
			if durK.V < 0 {
				c.fail("duration must be >= 0, got %s", durK.Display())
			}
			d.set(dur, v, durK)
			continue
		}
		ro := g.nrows[v]
		if ro == nil {
			c.fail("task %s is in edges but has no row in nodes; every task needs a %s (or duration: n for all)", g.ns.ids[v].Inspect(), durCol)
		}
		n := g.num(fmt.Sprintf("nodes[%d]", v), ro, durCol)
		if n.V < 0 {
			c.fail("nodes[%d] %s must be >= 0, got %s", v, durCol, n.Display())
		}
		d.set(dur, v, n)
	}
	lag := make([]float64, len(g.erows))
	d.add(lag)
	for k, ro := range g.erows {
		if lagCol == "" {
			break
		}
		v, ok := ro.Get(lagCol)
		if (!ok || v == Null) && !lagGiven {
			continue
		}
		n := g.num(fmt.Sprintf("edges[%d]", k), ro, lagCol)
		if n.V < 0 {
			c.fail("edges[%d] %s must be >= 0, got %s", k, lagCol, n.Display())
		}
		d.set(lag, k, n)
	}
	d.finish(float64(2*g.n() + 1))
	adj := make([][]graphalg.Arc, g.n())
	for k := range g.erows {
		adj[g.from[k]] = append(adj[g.from[k]], graphalg.Arc{To: g.to[k], W: lag[k]})
	}
	for _, e := range afters {
		adj[e.from] = append(adj[e.from], graphalg.Arc{To: e.to})
	}
	tol := 0.0
	if !d.exact() {
		total := 0.0
		for _, x := range dur {
			total += math.Abs(x)
		}
		for _, x := range lag {
			total += math.Abs(x)
		}
		tol = 1e-9 * math.Max(total, 1)
	}
	s, err := graphalg.CPM(adj, dur, tol)
	var ce *graphalg.CycleError
	if errors.As(err, &ce) {
		c.fail("the precedences have a cycle, so no schedule exists: %s. Remove one of them", g.cycleText(ce.Nodes))
	}
	order := make([]int, g.n())
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return s.ES[order[a]] < s.ES[order[b]] })
	sh.Limits().CheckAlloc(int64(g.n())*800, fmt.Sprintf("%s: %d rows", string(c), g.n()))
	tasks := make([]Value, len(order))
	for i, v := range order {
		r := NewObject()
		if ro := g.nrows[v]; ro != nil {
			r = copyProps(ro)
		} else {
			r.Set("id", g.ns.ids[v])
		}
		r.Set("duration", d.out(dur[v]))
		r.Set("earlyStart", d.out(s.ES[v]))
		r.Set("earlyFinish", d.out(s.EF[v]))
		r.Set("lateStart", d.out(s.LS[v]))
		r.Set("lateFinish", d.out(s.LF[v]))
		slack := s.Slack[v]
		if s.Critical[v] {
			slack = 0
		}
		r.Set("slack", d.out(slack))
		r.Set("critical", Bln(s.Critical[v]))
		tasks[i] = r
	}
	out := NewObject()
	out.Set("makespan", d.out(s.Makespan))
	out.Set("critical", g.ids(s.Path))
	out.Set("tasks", &ArrayVal{Elements: tasks})
	return out
}

func nonNil(rows []*ObjectVal) []*ObjectVal {
	var out []*ObjectVal
	for _, r := range rows {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}

// --- exact integers for flows and matching ---------------------------------

// ints scales exact numbers to int64 by their common denominator. Float-only
// values are refused (flows and costs stay exact, as Solver.lp's numbers).
type ints struct {
	c     rfail
	rats  []*big.Rat
	where []string
}

func (x *ints) add(where string, n *NumberVal) int {
	r, ok := n.Exact()
	if !ok {
		x.c.fail("%s is not an exact number (%s comes from a float operation such as sqrt). Flows and costs are exact: use an integer, decimal or fraction", where, n.Display())
	}
	x.rats = append(x.rats, r)
	x.where = append(x.where, where)
	return len(x.rats) - 1
}

// scale returns the values times the common denominator, and that denominator.
func (x *ints) scale() ([]int64, *big.Int) {
	lcm := big.NewInt(1)
	for i, r := range x.rats {
		den := r.Denom()
		g := new(big.Int).GCD(nil, nil, lcm, den)
		lcm.Mul(lcm, new(big.Int).Quo(den, g))
		if lcm.BitLen() > 40 {
			x.c.fail("%s: the denominators of the numbers multiply past 2^40; round them (to cents, say)", x.where[i])
		}
	}
	out := make([]int64, len(x.rats))
	lim := big.NewInt(graphalg.MaxInt)
	for i, r := range x.rats {
		v := new(big.Int).Mul(r.Num(), new(big.Int).Quo(lcm, r.Denom()))
		if v.CmpAbs(lim) >= 0 {
			x.c.fail("%s is too large: exact flows and costs must stay below 2^62 after scaling", x.where[i])
		}
		out[i] = v.Int64()
	}
	return out, lcm
}

func ratOut(v *big.Int, scale *big.Int) Value {
	return NumRat(new(big.Rat).SetFrac(v, scale))
}

// --- flow ----------------------------------------------------------------

func graphFlow(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, twoWay)
	if len(g.erows) == 0 {
		c.fail("needs edges: [{from, to, cap?, cost?}]")
	}
	capCol, _ := column(c, o, "cap", []string{"cap", "capacity"}, g.erows, false)
	costCol, _ := column(c, o, "cost", []string{"cost"}, g.erows, false)
	supCol, _ := column(c, o, "supply", []string{"supply"}, nonNil(g.nrows), false)
	fv, hasFrom := o.get("from")
	tv, hasTo := o.get("to")
	if hasFrom != hasTo {
		c.fail("from and to go together (a source and a sink); for many sources use node rows with supply")
	}
	amountV, hasAmount := o.get("amount")
	if hasAmount && !hasFrom {
		c.fail("amount needs from and to; with supplies, the amount is the supply")
	}
	stMode := hasFrom
	if stMode && supCol != "" {
		if _, given := o.get("supply"); given {
			c.fail("give from/to or node supplies, not both")
		}
		supCol = ""
	}
	if !stMode && supCol == "" {
		c.fail("needs from and to (max flow), or nodes: [{id, supply}] (supply > 0 ships out, < 0 takes in)")
	}
	flowN := &ints{c: c} // caps, supplies, amount: one unit
	costN := &ints{c: c}
	from, to, row, rev := g.arcs()
	capIdx := make([]int, len(from)) // -1 = no limit
	costIdx := make([]int, len(from))
	rowCap := make([]int, len(g.erows))
	rowCost := make([]int, len(g.erows))
	for k, ro := range g.erows {
		where := fmt.Sprintf("edges[%d]", k)
		rowCap[k], rowCost[k] = -1, -1
		if capCol != "" {
			if v, ok := ro.Get(capCol); ok && v != Null {
				n := g.num(where, ro, capCol)
				if n.V < 0 {
					c.fail("%s %s must be >= 0, got %s (null = no limit)", where, capCol, n.Display())
				}
				rowCap[k] = flowN.add(where+" "+capCol, n)
			}
		}
		if costCol != "" {
			n := g.num(where, ro, costCol)
			if n.V < 0 && !g.directed {
				c.fail("%s has a negative %s on a two-way edge (flow around it forever). Pass directed: true", where, costCol)
			}
			rowCost[k] = costN.add(where+" "+costCol, n)
		}
	}
	for a := range from {
		capIdx[a], costIdx[a] = rowCap[row[a]], rowCost[row[a]]
	}
	supIdx := make([]int, g.n())
	for v := range supIdx {
		supIdx[v] = -1
		if ro := g.nrows[v]; supCol != "" && ro != nil {
			if sv, ok := ro.Get(supCol); ok && sv != Null {
				supIdx[v] = flowN.add(fmt.Sprintf("nodes[%d] %s", v, supCol), g.num(fmt.Sprintf("nodes[%d]", v), ro, supCol))
			}
		}
	}
	amountIdx := -1
	if hasAmount {
		n, ok := amountV.(*NumberVal)
		if !ok || n.V < 0 {
			c.fail("amount must be a number >= 0, got %s", describe(amountV))
		}
		amountIdx = flowN.add("amount", n)
	}
	fl, fscale := flowN.scale()
	cs, cscale := costN.scale()
	arcs := make([]graphalg.FlowArc, len(from))
	for a := range from {
		arcs[a] = graphalg.FlowArc{From: from[a], To: to[a], Cap: -1}
		if capIdx[a] >= 0 {
			arcs[a].Cap = fl[capIdx[a]]
		}
		if costIdx[a] >= 0 {
			arcs[a].Cost = cs[costIdx[a]]
		}
	}
	ctx, done := routeContext(sh, c, 0)
	defer done()
	out := NewObject()
	setNull := func(keys ...string) {
		for _, k := range keys {
			out.Set(k, Null)
		}
	}
	var res *graphalg.FlowResult
	var cut []bool
	var err error
	supply := make([]int64, g.n())
	if stMode {
		s, t := g.ns.find(c, "from", fv), g.ns.find(c, "to", tv)
		if s == t {
			c.fail("from and to are the same node")
		}
		var amt int64
		if amountIdx >= 0 {
			amt = fl[amountIdx]
		} else {
			mf, err := graphalg.MaxFlow(ctx, g.n(), arcs, s, t)
			if err != nil {
				graphStop(sh, c, err)
			}
			if mf.Status == graphalg.Unbounded {
				out.Set("status", Str("unbounded"))
				setNull("value", "cost", "flows", "cut")
				sh.Note(fmt.Sprintf("%s: a path of edges with no %s limit joins %s to %s, so the flow has no maximum", string(c), orCap(capCol), fv.Inspect(), tv.Inspect()))
				return out
			}
			amt, cut = mf.Value, mf.Source
			if costCol == "" {
				res = mf
			}
		}
		supply[s], supply[t] = amt, -amt
	} else {
		var pos, neg big.Int
		for v, i := range supIdx {
			if i >= 0 {
				supply[v] = fl[i]
				if fl[i] > 0 {
					pos.Add(&pos, big.NewInt(fl[i]))
				} else {
					neg.Sub(&neg, big.NewInt(fl[i]))
				}
			}
		}
		if pos.Cmp(&neg) != 0 {
			c.fail("supplies sum to %s and demands to %s; they must be equal. Add a node {id: \"slack\", %s: %s} with edges of cost 0 from the sources (or to the sinks) for the difference",
				ratOut(&pos, fscale).Display(), ratOut(&neg, fscale).Display(), supCol,
				ratOut(new(big.Int).Sub(&neg, &pos), fscale).Display())
		}
	}
	if res == nil {
		res, err = graphalg.MinCostFlow(ctx, g.n(), arcs, supply)
		if err != nil {
			if errors.Is(err, graphalg.ErrInvalid) {
				c.fail("%s", strings.TrimPrefix(err.Error(), "graphalg: invalid input: "))
			}
			graphStop(sh, c, err)
		}
	}
	switch res.Status {
	case graphalg.Unbounded:
		out.Set("status", Str("unbounded"))
		setNull("value", "cost", "flows")
		sh.Note(fmt.Sprintf("%s: a cycle of negative %s with no %s limit makes the cost fall forever", string(c), costCol, orCap(capCol)))
		return out
	case graphalg.Infeasible:
		out.Set("status", Str("infeasible"))
		out.Set("value", ratOut(big.NewInt(res.Value), fscale))
		setNull("cost", "flows")
		var need big.Int
		for _, x := range supply {
			if x > 0 {
				need.Add(&need, big.NewInt(x))
			}
		}
		sh.Note(fmt.Sprintf("%s: only %s of %s can be shipped (value); raise capacities or lower the amount", string(c),
			ratOut(big.NewInt(res.Value), fscale).Display(), ratOut(&need, fscale).Display()))
		return out
	}
	out.Set("status", Str("optimal"))
	var total int64
	for _, x := range supply {
		if x > 0 {
			total += x
		}
	}
	if res.Cost == nil {
		total = res.Value
	}
	out.Set("value", ratOut(big.NewInt(total), fscale))
	if costCol != "" && res.Cost != nil {
		out.Set("cost", NumRat(new(big.Rat).SetFrac(res.Cost, new(big.Int).Mul(fscale, cscale))))
	} else {
		out.Set("cost", Null)
	}
	// Net flow per edge row: forward arc minus its added reverse arc.
	net := make([]int64, len(g.erows))
	for a := range from {
		if rev[a] {
			net[row[a]] -= res.Flow[a]
		} else {
			net[row[a]] += res.Flow[a]
		}
	}
	var flows []Value
	for k, ro := range g.erows {
		if net[k] != 0 {
			r := copyProps(ro)
			r.Set("flow", ratOut(big.NewInt(net[k]), fscale))
			flows = append(flows, r)
		}
	}
	out.Set("flows", &ArrayVal{Elements: flows})
	if flows == nil {
		out.Set("flows", Arr())
	}
	if cut != nil {
		var rows []Value
		for k, ro := range g.erows {
			a, b := g.from[k], g.to[k]
			if cut[a] && !cut[b] || !g.directed && cut[b] && !cut[a] {
				rows = append(rows, copyProps(ro))
			}
		}
		if rows == nil {
			rows = []Value{}
		}
		out.Set("cut", &ArrayVal{Elements: rows})
	}
	out.Set("directed", Bln(g.directed))
	return out
}

func orCap(col string) string {
	if col == "" {
		return "cap"
	}
	return col
}

// --- match ---------------------------------------------------------------

func graphMatch(sh *Shell, c rfail, o *ropts) Value {
	ev, ok := o.get("edges")
	if !ok {
		c.fail("needs edges: [{from: left, to: right, cost?}]")
	}
	rows := graphRows(c, "edges", ev)
	if len(rows) > maxGraphEdges {
		c.fail("%d edges is over the limit of %d", len(rows), maxGraphEdges)
	}
	left, right := newNodeSet(), newNodeSet()
	var erows []*ObjectVal
	var edges []graphalg.MatchEdge
	for i, r := range rows {
		where := fmt.Sprintf("edges[%d]", i)
		ro, isObj := r.(*ObjectVal)
		if !isObj {
			c.fail("%s must be an object {from, to, cost?}, got %s", where, describe(r))
		}
		fv, ok1 := ro.Get("from")
		tv, ok2 := ro.Get("to")
		if !ok1 || !ok2 {
			c.fail("%s needs from (left) and to (right). Row: %s", where, ro.Display())
		}
		l := left.intern(c, where+" from", fv)
		rr := right.intern(c, where+" to", tv)
		erows = append(erows, ro)
		edges = append(edges, graphalg.MatchEdge{L: l, R: rr})
	}
	for k := range left.idx {
		if _, both := right.idx[k]; both {
			c.fail("%q is both a from (left) and a to (right) node; matching is bipartite: left ids in from, right ids in to. General (non-bipartite) matching is not supported", k)
		}
	}
	costCol, _ := column(c, o, "cost", []string{"cost"}, erows, false)
	maximize := o.boolean("maximize", false)
	if maximize && costCol == "" {
		c.fail("maximize needs a cost column (cost: \"col\")")
	}
	var cscale *big.Int
	if costCol != "" {
		cn := &ints{c: c}
		g := &gin{c: c}
		for k, ro := range erows {
			cn.add(fmt.Sprintf("edges[%d] %s", k, costCol), g.num(fmt.Sprintf("edges[%d]", k), ro, costCol))
		}
		var cs []int64
		cs, cscale = cn.scale()
		for k := range edges {
			edges[k].Cost = cs[k]
			if maximize {
				edges[k].Cost = -cs[k]
			}
		}
	}
	ctx, done := routeContext(sh, c, 0)
	defer done()
	var m *graphalg.Matching
	var err error
	if costCol != "" {
		m, err = graphalg.Assignment(ctx, len(left.ids), len(right.ids), edges)
	} else {
		m, err = graphalg.MaxMatching(ctx, len(left.ids), len(right.ids), edges)
	}
	if err != nil {
		if errors.Is(err, graphalg.ErrInvalid) {
			c.fail("%s", strings.TrimPrefix(err.Error(), "graphalg: invalid input: "))
		}
		graphStop(sh, c, err)
	}
	usedL, usedR := make([]bool, len(left.ids)), make([]bool, len(right.ids))
	pairs := make([]Value, len(m.Edges))
	for i, e := range m.Edges {
		pairs[i] = copyProps(erows[e])
		usedL[edges[e].L], usedR[edges[e].R] = true, true
	}
	free := func(ns *nodeSet, used []bool) *ArrayVal {
		var idx []int
		for i, u := range used {
			if !u {
				idx = append(idx, i)
			}
		}
		return idList(ns, idx)
	}
	out := NewObject()
	out.Set("status", Str("optimal"))
	out.Set("size", Num(float64(len(m.Edges))))
	if costCol != "" {
		total := new(big.Int).Set(m.Cost)
		if maximize {
			total.Neg(total)
		}
		out.Set("cost", ratOut(total, cscale))
	} else {
		out.Set("cost", Null)
	}
	out.Set("pairs", &ArrayVal{Elements: pairs})
	un := NewObject()
	un.Set("from", free(left, usedL))
	un.Set("to", free(right, usedR))
	out.Set("unmatched", un)
	return out
}

// --- mst -----------------------------------------------------------------

func graphMST(sh *Shell, c rfail, o *ropts) Value {
	g := readGraph(sh, c, o, undirected)
	col, k := weightCol(c, o, g)
	d := &dim{}
	w := g.weights(d, col, k, false)
	d.finish(float64(g.n()))
	chosen, trees := graphalg.MST(g.n(), g.from, g.to, w, o.boolean("maximize", false))
	total := 0.0
	for _, e := range chosen {
		total += w[e]
	}
	out := NewObject()
	out.Set("status", Str("optimal"))
	out.Set(outKey(col), d.out(total))
	out.Set("edges", g.rowList(chosen))
	out.Set("trees", Num(float64(trees)))
	if trees > 1 {
		sh.Note(fmt.Sprintf("%s: the graph has %d components, so the result is a spanning forest (one tree each); Graph.components lists them", string(c), trees))
	}
	return out
}
