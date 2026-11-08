package toolkit_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func routeShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := toolkit.AddRoute(toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell())))
	okEval(t, sh, `load("route")`)
	return sh
}

func TestRouteLoadHint(t *testing.T) {
	sh := toolkit.AddRoute(toolkit.InstallCore(runtime.NewShell()))
	mustContain(t, "call before load", failEval(t, sh, `Route.tsp({})`), `load("route")`)
	first := okEval(t, sh, `load("route")`)
	mustContain(t, "first load", first, "Route.matrix(", "Route.path(", "Route.paths(", "Route.tsp(", "Route.vrp(", "TYPICAL: VRP", "TRAPS")
	if n := strings.Count(routeGuideOf(t), "\n") + 1; n > 40 {
		t.Errorf("guide is %d lines", n)
	}
}

func routeGuideOf(t *testing.T) string {
	return okEval(t, routeShell(t), `help("route")`)
}

func TestRouteMatrix(t *testing.T) {
	got := okEval(t, routeShell(t), `Route.matrix([{id: "a", x: 0, y: 0}, {id: "b", x: 3, y: 4}])`)
	if got != `{ids: ["a", "b"], matrix: [[0, 5], [5, 0]]}` {
		t.Errorf("euclidean: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.matrix([{id: 1, x: 0, y: 0}, {id: 2, x: 3, y: 4}], {metric: "manhattan", format: "rows"})`)
	if got != `[{from: 1, to: 2, dist: 7}, {from: 2, to: 1, dist: 7}]` {
		t.Errorf("rows: %s", got)
	}
	// Equator, 1 degree: 2*pi*R/360 = 111.19508 km.
	got = okEval(t, routeShell(t), `Math.round(Route.matrix([{id: "a", lat: 0, lon: 0}, {id: "b", lat: 0, lng: 1}], {units: "km"}).matrix[0][1] * 1000)`)
	if got != "111195" {
		t.Errorf("haversine: %s", got)
	}
	// Exact decimals stay exact through a shortest-path fill: 0.1 + 0.2 == 0.3.
	got = okEval(t, routeShell(t), `Route.matrix([{from: "a", to: "b", dist: 0.1}, {from: "b", to: "c", dist: 0.2}]).matrix[0][2] == 0.3`)
	if got != "true" {
		t.Errorf("exact fill: %s", got)
	}
	// One-way edges and a matrix with null = no arc.
	got = okEval(t, routeShell(t), `Route.matrix([{from: "a", to: "b", dist: 2}, {from: "b", to: "a", dist: 5}]).matrix`)
	if got != `[[0, 2], [5, 0]]` {
		t.Errorf("asymmetric: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.tsp({matrix: [[0, 1, null], [null, 0, 1], [1, null, 0]]}).cost`)
	if got != "3" {
		t.Errorf("null fill tsp: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.matrix([{id: "a", x: 0, y: 0}, {id: "b", x: 3, y: 4}, {id: "c", x: 6, y: 8}], {stops: ["a", "c"]})`)
	if got != `{ids: ["a", "c"], matrix: [[0, 10], [10, 0]]}` {
		t.Errorf("stops: %s", got)
	}
}

func TestRoutePath(t *testing.T) {
	edges := `[{from: "a", to: "b", dist: 4}, {from: "b", to: "c", dist: 3}, {from: "a", to: "c", dist: 9}, {from: "c", to: "d", dist: 1}]`
	got := okEval(t, routeShell(t), `let r = Route.path({edges: `+edges+`, from: "a", to: "d"}); [r.found, r.dist, r.path |> join(""), r.legs |> map(l => l.dist) |> join(","), r.method] |> join(" ")`)
	if got != "true 8 abcd 4,3,1 dijkstra" {
		t.Errorf("path: %s", got)
	}
	pts := `[{id: "a", x: 0, y: 0}, {id: "b", x: 4, y: 0}, {id: "c", x: 4, y: 3}, {id: "d", x: 5, y: 3}]`
	got = okEval(t, routeShell(t), `let r = Route.path({edges: `+edges+`, points: `+pts+`, from: "a", to: "d"}); [r.dist, r.path |> join(""), r.method] |> join(" ")`)
	if got != "8 abcd astar" {
		t.Errorf("astar: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.path({edges: [{from: "a", to: "b", dist: 1}, {from: "c", to: "d", dist: 1}], from: "a", to: "d"})`)
	if got != `{found: false, dist: null, path: [], legs: [], method: "dijkstra"}` {
		t.Errorf("not found: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.path({edges: [{from: "a", to: "b", dist: 1}], directed: true, from: "b", to: "a"}).found`)
	if got != "false" {
		t.Errorf("directed: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.path({edges: [{from: "a", to: "b", time: 7}], from: "a", to: "b"}).time`)
	if got != "7" {
		t.Errorf("time column: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.paths({edges: `+edges+`, from: "a"}) |> map(r => r.to + "=" + str(r.dist)) |> join(",")`)
	if got != "b=4,c=7,d=8" {
		t.Errorf("paths: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.paths({edges: `+edges+`, from: "a", to: "c", k: 3}) |> map(r => str(r.rank) + ":" + str(r.dist) + ":" + (r.path |> join(""))) |> join(",")`)
	if got != "1:7:abc,2:9:ac" {
		t.Errorf("k paths: %s", got)
	}
}

func TestRouteTSP(t *testing.T) {
	square := `[{id: "a", x: 0, y: 0}, {id: "b", x: 4, y: 0}, {id: "c", x: 4, y: 3}, {id: "d", x: 0, y: 3}]`
	got := okEval(t, routeShell(t), `let r = Route.tsp({points: `+square+`, start: "b"}); [r.status, r.cost, r.order[0], r.order[4], r.order.length, r.bound, r.gap, r.method] |> join(" ")`)
	if got != "optimal 14 b b 5 14 0 held-karp" {
		t.Errorf("square: %s", got)
	}
	got = okEval(t, routeShell(t), `let r = Route.tsp({points: `+square+`, start: "a", end: "d"}); [r.cost, r.order |> join("")] |> join(" ")`)
	if got != "11 abcd" {
		t.Errorf("fixed end: %s", got)
	}
	got = okEval(t, routeShell(t), `Route.tsp({points: `+square+`, start: "a", roundTrip: false}).cost`)
	if got != "10" {
		t.Errorf("open: %s", got)
	}
	// Exact fractions in, exact cost out.
	got = okEval(t, routeShell(t), `Route.tsp({matrix: [[0, 1/3, 2], [1/3, 0, 0.5], [2, 0.5, 0]]}).cost == 17/6`)
	if got != "true" {
		t.Errorf("exact cost: %s", got)
	}
	// Heuristic on 80 points: feasible, bound <= cost, a valid tour, repeatable.
	rng := rand.New(rand.NewSource(1))
	var pts []string
	for i := 0; i < 80; i++ {
		pts = append(pts, fmt.Sprintf(`{id: "p%d", x: %d, y: %d}`, i, rng.Intn(1000), rng.Intn(1000)))
	}
	p := "[" + strings.Join(pts, ", ") + "]"
	src := `let r = Route.tsp({points: ` + p + `, metric: "manhattan", seed: 3});
	  [r.status, r.method, r.bound <= r.cost, r.gap < 0.1, r.order.length, (r.order |> unique).length, r.legs |> map(l => l.dist) |> sum == r.cost, r.cost] |> join(" ")`
	a := okEval(t, routeShell(t), src)
	if !strings.HasPrefix(a, "feasible heuristic true true 81 80 true ") {
		t.Errorf("heuristic: %s", a)
	}
	if b := okEval(t, routeShell(t), src); a != b {
		t.Errorf("not repeatable: %s vs %s", a, b)
	}
	got = okEval(t, routeShell(t), `Route.tsp({points: `+p+`, stops: ["p1", "p2", "p3", "p4", "p5"], start: "p3"}).order.length`)
	if got != "6" {
		t.Errorf("stops subset: %s", got)
	}
}

func TestRouteVRP(t *testing.T) {
	pts := `[{id: "d", x: 0, y: 0}, {id: "a", x: 10, y: 0}, {id: "b", x: 0, y: 10}, {id: "c", x: 10, y: 10}, {id: "e", x: -10, y: 0}]`
	got := okEval(t, routeShell(t), `let r = Route.vrp({points: `+pts+`, metric: "manhattan", depot: "d",
	  stops: [{id: "a", demand: 3}, {id: "b", demand: 3}, {id: "c", demand: 3}, {id: "e", demand: 3}], vehicles: 2, capacity: 6});
	  [r.status, r.routes.length, r.routes |> map(x => x.load) |> join("+"), r.unassigned.length, r.dist == (r.routes |> map(x => x.dist) |> sum), r.bound <= r.cost] |> join(" ")`)
	if got != "feasible 2 6+6 0 true true" {
		t.Errorf("capacity: %s", got)
	}
	// Windows: b must be served by 15, a after 50. Travel time = dist (no speed).
	got = okEval(t, routeShell(t), `let r = Route.vrp({points: `+pts+`, metric: "manhattan", depot: "d",
	  stops: [{id: "a", window: [50, 100], service: 5}, {id: "b", window: [0, 15]}], vehicles: 1});
	  let x = r.routes[0]; [r.status, x.stops |> join(""), x.arrivals |> join(","), x.depart, x.back, x.time] |> join(" ")`)
	if got != "feasible ba 10,50 0 65 65" {
		t.Errorf("windows: %s", got)
	}
	// speed converts distance to time; exact.
	got = okEval(t, routeShell(t), `Route.vrp({points: `+pts+`, metric: "manhattan", depot: "d", stops: ["a"], vehicles: 1, speed: 3}).routes[0].time == 20/3`)
	if got != "true" {
		t.Errorf("speed: %s", got)
	}
	// Unassigned with reasons; bound and gap null.
	got = okEval(t, routeShell(t), `let r = Route.vrp({points: `+pts+`, depot: "d", stops: [{id: "a", demand: 9}, {id: "b", window: [0, 5]}, {id: "c"}],
	  vehicles: [{id: "v1", capacity: 5}]});
	  [r.status, r.unassigned |> map(u => u.id + ": " + u.reason) |> join("; "), r.bound, r.gap] |> join(" | ")`)
	if got != "infeasible | a: demand 9 is over the largest vehicle capacity 5; b: window closes at 5; the earliest arrival is 10 | null | null" {
		t.Errorf("unassigned: %s", got)
	}
	// Vehicle rows: start/end, maxDist, objective vehicles.
	got = okEval(t, routeShell(t), `let r = Route.vrp({points: `+pts+`, metric: "manhattan", depot: "d", stops: ["a", "c", "b"],
	  vehicles: [{id: "x", maxDist: 30}, {id: "y", end: "e"}], objective: "vehicles"});
	  [r.status, r.vehicles, r.cost, r.routes[0].vehicle] |> join(" ")`)
	if !strings.HasPrefix(got, "feasible 1 1 ") {
		t.Errorf("vehicles objective: %s", got)
	}
	// times matrix by id.
	got = okEval(t, routeShell(t), `Route.vrp({matrix: {ids: ["d", "a"], matrix: [[0, 4], [4, 0]]}, times: {ids: ["a", "d"], matrix: [[0, 7], [7, 0]]},
	  depot: "d", stops: ["a"], vehicles: 1}).routes[0] |> (x => [x.dist, x.time] |> join(","))`)
	if got != "8,14" {
		t.Errorf("times: %s", got)
	}
}

func TestRouteFromDbRows(t *testing.T) {
	sh := toolkit.AddRoute(toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell())))
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	t.Cleanup(func() { sh.Unload("db") })
	okEval(t, sh, `load("route"); load("db")`)
	okEval(t, sh, `Db.load("shops", [{id: "s1", name: "A", lat: 52.52, lon: 13.40}, {id: "s2", name: "B", lat: 52.50, lon: 13.45},
	  {id: "s3", name: "C", lat: 52.48, lon: 13.38}, {id: "s4", name: "D", lat: 52.55, lon: 13.35}])`)
	got := okEval(t, sh, `let pts = Db.query("SELECT id, name, lat, lon FROM shops ORDER BY id");
	  let r = Route.tsp({points: pts, start: pts[0].id, units: "km"});
	  Db.load("legs", r.legs);
	  [r.status, Db.query("SELECT count(*) AS n FROM legs")[0].n] |> join(" ")`)
	if got != "optimal 4" {
		t.Errorf("db: %s", got)
	}
}

func TestRouteTimeLimit(t *testing.T) {
	sh := routeShell(t)
	sh.Limits().SetDefaults(1_000_000, 256, 1500)
	start := time.Now()
	got := okEval(t, sh, `let pts = range(0, 1200) |> map(i => ({id: i, x: (i * 7919) % 1000, y: (i * 104729) % 997}));
	  let r = Route.tsp({points: pts, iterations: 1000000}); [r.status, r.stopped, r.bound <= r.cost, r.order.length] |> join(" ")`)
	if got != "feasible time true 1201" {
		t.Errorf("time stop: %s", got)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Errorf("took %v", el)
	}
	notes := strings.Join(sh.Limits().Notes(), "\n")
	mustContain(t, "note", notes, "Route.tsp: the time limit stopped the search", "best found")
	// timeLimit stops earlier than the eval limit.
	got = okEval(t, sh, `Route.tsp({points: range(0, 300) |> map(i => ({id: i, x: (i * 7919) % 1000, y: (i * 104729) % 997})), iterations: 1000000, timeLimit: 100}).stopped`)
	if got != "time" {
		t.Errorf("timeLimit: %s", got)
	}
}

func TestRouteErrors(t *testing.T) {
	sh := routeShell(t)
	pts := `[{id: "a", x: 0, y: 0}, {id: "b", x: 1, y: 1}]`
	cases := []struct {
		src  string
		want []string
	}{
		{`Route.tsp({points: [{id: "a", x: 0, y: 0}, {id: "a", x: 1, y: 1}]})`, []string{"Route.tsp", "points[1]", `duplicate id "a"`}},
		{`Route.tsp({points: ` + pts + `, strat: "a"})`, []string{`unknown key "strat"`, `did you mean "start"`, "Usage: Route.tsp"}},
		{`Route.tsp({points: ` + pts + `, start: "z"})`, []string{`start "z" is not a node`, `ids: "a", "b"`}},
		{`Route.tsp({points: ` + pts + `}, 1)`, []string{"Route.tsp", "1"}},
		{`Route.tsp({points: ` + pts + `, matrix: [[0]]})`, []string{"give one of points, matrix, edges"}},
		{`Route.tsp({})`, []string{"needs points"}},
		{`Route.tsp({points: [{id: "a", lat: 0}]})`, []string{"points[0] needs both lat and lon"}},
		{`Route.tsp({points: [{id: "a", lat: 0, lon: 0}, {id: "b", x: 1, y: 1}]})`, []string{"mix of lat/lon and x/y"}},
		{`Route.tsp({points: ` + pts + `, units: "km"})`, []string{"units applies to lat/lon points"}},
		{`Route.tsp({matrix: [[0, 1], [1]]})`, []string{"matrix[1] must be an array of 2 numbers"}},
		{`Route.tsp({matrix: [[0, -1], [1, 0]]})`, []string{"matrix[0][1] must be >= 0"}},
		{`Route.tsp({edges: [{from: "a", to: "b", dist: 1}, {from: "c", to: "d", dist: 1}]})`, []string{"no path from", "two-way unless directed"}},
		{`Route.tsp({points: range(0, 25) |> map(i => ({id: i, x: i, y: 0})), exact: true})`, []string{"exact: true works up to 20 nodes"}},
		{`Route.tsp({points: ` + pts + `, end: "b", roundTrip: true})`, []string{"end is for a path"}},
		{`Route.path({points: ` + pts + `, from: "a", to: "b"})`, []string{"needs edges or matrix"}},
		{`Route.path({edges: [{from: "a", to: "b", dist: 1}], points: [{id: "a", x: 0, y: 0}], from: "a", to: "b"})`, []string{`points has no row for node "b"`}},
		{`Route.path({edges: [{from: "a", to: "b"}], from: "a", to: "b"})`, []string{"edges[0] needs dist or time"}},
		{`Route.paths({edges: [{from: "a", to: "b", dist: 1}], from: "a", k: 2})`, []string{"k needs to"}},
		{`Route.vrp({points: ` + pts + `, stops: ["b"], vehicles: 1})`, []string{"needs depot"}},
		{`Route.vrp({points: ` + pts + `, depot: "a", stops: [{id: "b", demnad: 3}], vehicles: 1})`, []string{`unknown key "demnad"`, `did you mean "demand"`}},
		{`Route.vrp({points: ` + pts + `, depot: "a", stops: [{id: "b", window: [10, 5]}], vehicles: 1})`, []string{"closes before it opens"}},
		{`Route.vrp({points: ` + pts + `, depot: "a", stops: ["a"], vehicles: 1})`, []string{"is a depot"}},
		{`Route.vrp({points: ` + pts + `, depot: "a", vehicles: [{id: "v", cap: 3}]})`, []string{`vehicles[0]: unknown key "cap"`}},
		{`Route.vrp({points: ` + pts + `, depot: "a", vehicles: 0})`, []string{"vehicles must be a count from 1 to 1000"}},
		{`Route.vrp({points: ` + pts + `, depot: "a", vehicles: 1, speed: 0})`, []string{"speed must be > 0"}},
		{`Route.vrp({points: ` + pts + `, depot: "a", vehicles: 1, objective: "cost"})`, []string{`objective must be "distance" | "time" | "vehicles"`}},
		{`Route.matrix(range(0, 5001) |> map(i => ({id: i, x: i, y: 0})))`, []string{"5001 points is over the limit of 5000"}},
	}
	for _, c := range cases {
		t.Run(c.src[:min(len(c.src), 60)], func(t *testing.T) { mustContain(t, "error", failEval(t, sh, c.src), c.want...) })
	}
}
