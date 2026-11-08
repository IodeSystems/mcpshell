package toolkit_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Reference values: geo/testdata/refs.json (sklearn haversine, pyproj WGS84
// area); geo/geo_test.go checks the algorithms against shapely, pyproj and
// scikit-learn. These tests check the language surface.

func geoShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := toolkit.AddGeo(toolkit.AddRoute(toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))))
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	okEval(t, sh, `load("geo")`)
	t.Cleanup(func() { sh.Unload("db") })
	return sh
}

func num(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("not a number: %s", s)
	}
	return f
}

func TestGeoLoadAndHelp(t *testing.T) {
	sh := toolkit.AddGeo(toolkit.InstallCore(runtime.NewShell()))
	mustContain(t, "before load", failEval(t, sh, `Geo.distance({lat: 0, lon: 0}, {lat: 0, lon: 1})`), `load("geo")`)
	first := okEval(t, sh, `load("geo")`)
	for _, c := range []string{"distance", "bearing", "destination", "bbox", "nearest", "length", "area", "centroid",
		"contains", "intersects", "within", "hull", "circle", "toGeoJSON", "fromGeoJSON", "kmeans", "dbscan"} {
		mustContain(t, "load", first, "Geo."+c+"(")
	}
	g := okEval(t, sh, `help("geo")`)
	mustContain(t, "guide", g, "TYPICAL: Distance", "TYPICAL: Territories", "TYPICAL: Rows <-> GeoJSON", "TYPICAL: Split stops", "TRAPS", "[lon, lat]", "not optimal", "Not supported")
	if n := strings.Count(g, "\n") + 1; n > 45 {
		t.Errorf("guide is %d lines", n)
	}
}

func TestGeoDistanceBearingDestination(t *testing.T) {
	sh := geoShell(t)
	for _, c := range []struct {
		src  string
		want float64
	}{
		// sklearn haversine_distances on R = 6371008.8 m
		{`Geo.distance({lat: 52.52, lon: 13.405}, {lat: 48.1351, lon: 11.582})`, 504416.0281285079},
		{`Geo.distance({lat: 0, lon: 179.5}, {lat: 0, lon: -179.5})`, 111195.08023353321},
		{`Geo.distance({lat: 90, lon: 0}, {lat: -90, lon: 0})`, 20015114.442035925},
		{`Geo.distance({lat: 89.9, lon: 0}, {lat: 89.9, lon: 180})`, 22239.01604670598},
		{`Geo.distance({lat: -89.99, lon: 45}, {lat: -89.99, lon: -135})`, 2223.9016046713},
		{`Geo.distance({lat: 52.52, lng: 13.405}, {type: "Point", coordinates: [11.582, 48.1351]}, {units: "km"})`, 504.4160281285079},
		{`Geo.distance({x: 0, y: 0}, {x: 3, y: 4})`, 5},
		{`Geo.bearing({lat: 0, lon: 179.5}, {lat: 0, lon: -179.5})`, 90},
		{`Geo.bearing({x: 0, y: 0}, {x: -1, y: 0})`, 270},
		{`Geo.destination({lat: 0, lon: 179.5}, 90, 111195.08023353321).lon`, -179.5},
	} {
		if got := num(t, okEval(t, sh, c.src)); math.Abs(got-c.want) > 1e-9*math.Max(1, math.Abs(c.want)) {
			t.Errorf("%s = %v, want %v", c.src, got, c.want)
		}
	}
	got := okEval(t, sh, `Geo.destination({x: 1, y: 1}, 0, 2)`)
	if got != `{x: 1, y: 3}` {
		t.Errorf("planar destination %s", got)
	}
}

func TestGeoBBox(t *testing.T) {
	sh := geoShell(t)
	if got := okEval(t, sh, `Geo.bbox([{lat: 0, lon: 179}, {lat: 1, lon: -179}, {lat: -1, lon: 178}])`); got != `{west: 178, south: -1, east: -179, north: 1, crossesAntimeridian: true}` {
		t.Errorf("antimeridian %s", got)
	}
	if got := okEval(t, sh, `Geo.bbox({type: "LineString", coordinates: [[0, 0], [2, 3]]})`); got != `{west: 0, south: 0, east: 2, north: 3, crossesAntimeridian: false}` {
		t.Errorf("geojson %s", got)
	}
	if got := okEval(t, sh, `Geo.bbox({lat: 89.9, lon: 0}, {radius: 50, units: "km"})`); !strings.Contains(got, "west: -180") || !strings.Contains(got, "north: 90") {
		t.Errorf("over the pole %s", got)
	}
	if got := okEval(t, sh, `Geo.bbox({lat: 10, lon: 179.99}, {radius: 20000}).crossesAntimeridian`); got != "true" {
		t.Errorf("radius over the antimeridian %s", got)
	}
	if got := okEval(t, sh, `Geo.bbox([{x: 1, y: 5}, {x: -2, y: 3}])`); got != `{minX: -2, minY: 3, maxX: 1, maxY: 5}` {
		t.Errorf("planar %s", got)
	}
}

// The R-tree prefilter of the guide: a box query, then the exact distance.
func TestGeoWithDbRTree(t *testing.T) {
	sh := geoShell(t)
	okEval(t, sh, `load("db")`)
	got := okEval(t, sh, `
	let shops = [{id: 1, lat: 52.5200, lon: 13.4050}, {id: 2, lat: 52.5300, lon: 13.4100}, {id: 3, lat: 52.6000, lon: 13.5000}, {id: 4, lat: 48.14, lon: 11.58}]
	Db.exec("CREATE VIRTUAL TABLE idx USING rtree(id, minLon, maxLon, minLat, maxLat)")
	for (const s of shops) Db.exec("INSERT INTO idx VALUES (?, ?, ?, ?, ?)", [s.id, s.lon, s.lon, s.lat, s.lat])
	let me = {lat: 52.521, lon: 13.406}
	let b = Geo.bbox(me, {radius: 2000})
	let cand = Db.query("SELECT id FROM idx WHERE minLon <= ? AND maxLon >= ? AND minLat <= ? AND maxLat >= ? ORDER BY id",
	  [b.east, b.west, b.north, b.south]) |> map(r => r.id)
	let exact = shops |> filter(s => cand.includes(s.id) && Geo.distance(me, s) <= 2000) |> map(s => s.id)
	[cand |> join(","), exact |> join(",")] |> join(" ")`)
	if got != "1,2 1,2" {
		t.Errorf("rtree prefilter: %s", got)
	}
}

func TestGeoNearest(t *testing.T) {
	sh := geoShell(t)
	got := okEval(t, sh, `Geo.nearest({x: 0, y: 0}, [{id: "a", x: 5, y: 0}, {id: "b", x: 1, y: 1}, {id: "c", x: 0, y: 2}], {k: 2}) |> map(r => r.id + ":" + Math.round(r.dist * 100) / 100) |> join(" ")`)
	if got != "b:1.41 c:2" {
		t.Errorf("nearest %s", got)
	}
	got = okEval(t, sh, `Geo.nearest({lat: 0, lon: 179.9}, [{id: "west", lat: 0, lon: 170}, {id: "east", lat: 0, lon: -179.9}], {units: "km"})[0].id`)
	if got != `east` {
		t.Errorf("nearest over the antimeridian %s", got)
	}
	got = okEval(t, sh, `Geo.nearest({x: 0, y: 0}, [{x: 5, y: 0}, {x: 9, y: 0}], {k: 2, maxDist: 6}).length`)
	if got != "1" {
		t.Errorf("maxDist %s", got)
	}
	mustContain(t, "dist column", failEval(t, sh, `Geo.nearest({x: 0, y: 0}, [{x: 1, y: 0, dist: 3}])`), `already has a "dist" column`, "key")
	if got := okEval(t, sh, `Geo.nearest({x: 0, y: 0}, [{x: 1, y: 0, dist: 3}], {key: "d"})[0].d`); got != "1" {
		t.Errorf("key %s", got)
	}
}

func TestGeoAreaLengthCentroid(t *testing.T) {
	sh := geoShell(t)
	// pyproj Geod(ellps="WGS84"): 12308778361.469452 m2 for the 1 degree square at the equator.
	a := num(t, okEval(t, sh, `Geo.area({type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]]]})`))
	if math.Abs(a-12308778361.469452)/a > 1e-6 {
		t.Errorf("area %v", a)
	}
	// The same as rows, clockwise and unclosed, in km2.
	a = num(t, okEval(t, sh, `Geo.area([{lat: 0, lon: 0}, {lat: 1, lon: 0}, {lat: 1, lon: 1}, {lat: 0, lon: 1}], {units: "km2"})`))
	if math.Abs(a-12308.778361469452)/a > 1e-6 {
		t.Errorf("area rows km2 %v", a)
	}
	// pyproj: 9231614224.814873 m2 with the hole.
	a = num(t, okEval(t, sh, `Geo.area({type: "Feature", properties: {}, geometry: {type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]], [[0.25, 0.25], [0.75, 0.25], [0.75, 0.75], [0.25, 0.75], [0.25, 0.25]]]}})`))
	if math.Abs(a-9231614224.814873)/a > 1e-6 {
		t.Errorf("area with hole %v", a)
	}
	if got := okEval(t, sh, `Geo.area([{x: 0, y: 0}, {x: 4, y: 0}, {x: 4, y: 3}])`); got != "6" {
		t.Errorf("planar area %s", got)
	}
	mustContain(t, "area of a line", failEval(t, sh, `Geo.area({type: "LineString", coordinates: [[0, 0], [1, 1]]})`), "needs a Polygon")
	l := num(t, okEval(t, sh, `Geo.length([{lat: 0, lon: 179.5}, {lat: 0, lon: -179.5}])`))
	if math.Abs(l-111195.08023353321) > 1e-6 {
		t.Errorf("length over the antimeridian %v", l)
	}
	if got := okEval(t, sh, `Geo.length({type: "Polygon", coordinates: [[[0, 0], [3, 0], [3, 4], [0, 0]]], planar: true})`); got != "12" {
		t.Errorf("planar perimeter %s", got)
	}
	if got := okEval(t, sh, `Geo.centroid({type: "Polygon", coordinates: [[[0, 0], [4, 0], [4, 4], [0, 4], [0, 0]]]})`); got != `{lat: 2, lon: 2}` {
		t.Errorf("centroid %s", got)
	}
	// Points: the mean on the sphere, which holds across the antimeridian.
	got := okEval(t, sh, `let c = Geo.centroid([{lat: 0, lon: 179}, {lat: 0, lon: -179}]); [Math.round(Math.abs(c.lon)), Math.round(c.lat)] |> join(",")`)
	if got != "180,0" {
		t.Errorf("centroid over the antimeridian %s", got)
	}
}

func TestGeoPredicates(t *testing.T) {
	sh := geoShell(t)
	zone := `{type: "Polygon", coordinates: [[[0, 0], [4, 0], [4, 4], [0, 4], [0, 0]], [[1, 1], [2, 1], [2, 2], [1, 2], [1, 1]]]}`
	for src, want := range map[string]string{
		`Geo.contains(` + zone + `, {lat: 3, lon: 3})`:                                                                                "true",
		`Geo.contains(` + zone + `, {lat: 1.5, lon: 1.5})`:                                                                            "false", // in the hole
		`Geo.contains(` + zone + `, {lat: 2, lon: 0})`:                                                                                "true",  // on the boundary
		`Geo.contains(` + zone + `, {type: "LineString", coordinates: [[0, 0], [4, 4]]})`:                                             "false", // crosses the hole
		`Geo.contains(` + zone + `, {type: "LineString", coordinates: [[3, 0], [3, 4]]})`:                                             "true",
		`Geo.intersects(` + zone + `, {type: "LineString", coordinates: [[4, 4], [5, 5]]})`:                                           "true", // touches a corner
		`Geo.intersects(` + zone + `, {type: "Point", coordinates: [1.5, 1.5]})`:                                                      "false",
		`Geo.contains([{x: 0, y: 0}, {x: 2, y: 0}, {x: 2, y: 2}, {x: 0, y: 2}], {x: 1, y: 1})`:                                        "true",
		`Geo.within([{id: 1, lat: 3, lon: 3}, {id: 2, lat: 1.5, lon: 1.5}, {id: 3, lat: 9, lon: 9}], ` + zone + `) |> map(r => r.id)`: "[1]",
	} {
		if got := okEval(t, sh, src); got != want {
			t.Errorf("%s = %s, want %s", src, got, want)
		}
	}
	mustContain(t, "mixed", failEval(t, sh, `Geo.contains(`+zone+`, {x: 1, y: 1})`), "x/y", "lat/lon")
	mustContain(t, "contains a point area", failEval(t, sh, `Geo.contains({lat: 1, lon: 1}, {lat: 1, lon: 1})`), "must be a Polygon or MultiPolygon")
}

func TestGeoHullCircle(t *testing.T) {
	sh := geoShell(t)
	if got := okEval(t, sh, `Geo.hull([{x: 0, y: 0}, {x: 4, y: 0}, {x: 2, y: 1}, {x: 2, y: 3}])`); got != `{type: "Polygon", coordinates: [[[0, 0], [4, 0], [2, 3], [0, 0]]], planar: true}` {
		t.Errorf("hull %s", got)
	}
	if got := okEval(t, sh, `Geo.hull([{lat: 0, lon: 0}, {lat: 1, lon: 1}, {lat: 2, lon: 2}]).type`); got != `LineString` {
		t.Errorf("collinear hull %s", got)
	}
	// The planar hull goes back into the predicates as x/y without an option.
	if got := okEval(t, sh, `Geo.contains(Geo.hull([{x: 0, y: 0}, {x: 4, y: 0}, {x: 2, y: 3}]), {x: 2, y: 1})`); got != "true" {
		t.Errorf("hull round trip %s", got)
	}
	mustContain(t, "hull antimeridian", failEval(t, sh, `Geo.hull([{lat: 0, lon: 179}, {lat: 1, lon: -179}, {lat: 2, lon: 178}])`), "antimeridian")
	// Every vertex of a circle is at the radius on the sphere. Its area is on the
	// ellipsoid, where at 52.5 N the local radius sqrt(M N) = 6387 km exceeds the
	// sphere's 6371 km: (6387/6371)^2 = 1.005 times pi r^2 (less the 360-gon's 0.99995).
	got := okEval(t, sh, `let c = {lat: 52.52, lon: 13.405}; let g = Geo.circle(c, 1000, {steps: 360});
	  let d = g.coordinates[0] |> map(p => Geo.distance(c, {lat: p[1], lon: p[0]}));
	  [Math.round(Math.min(...d) * 1000) / 1000, Math.round(Math.max(...d) * 1000) / 1000, Math.round(Geo.area(g) / (Math.PI * 1e6) * 1000) / 1000] |> join(",")`)
	if got != "1000,1000,1.004" {
		t.Errorf("circle %s", got)
	}
	mustContain(t, "circle antimeridian", failEval(t, sh, `Geo.circle({lat: 0, lon: 179.99}, 5000)`), "antimeridian")
}

func TestGeoJSONRoundTrip(t *testing.T) {
	sh := geoShell(t)
	got := okEval(t, sh, `let rows = [{id: "a", lat: 52.52, lon: 13.405, sales: 10}, {id: "b", geometry: {type: "LineString", coordinates: [[0, 0], [1, 1]]}, sales: 2}]
	let fc = Geo.toGeoJSON(rows, {id: "id"})
	[JSON.stringify(fc.features[0]), JSON.stringify(Geo.fromGeoJSON(fc))] |> join("\n")`)
	want := `{"type":"Feature","id":"a","geometry":{"type":"Point","coordinates":[13.405,52.52]},"properties":{"id":"a","sales":10}}` + "\n" +
		`[{"id":"a","sales":10,"lat":52.52,"lon":13.405},{"id":"b","sales":2,"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}]`
	if got != want {
		t.Errorf("round trip:\n%s\nwant\n%s", got, want)
	}
	got = okEval(t, sh, `Geo.fromGeoJSON(Geo.toGeoJSON([{x: 3, y: 4, n: 1}]))`)
	if got != `[{n: 1, x: 3, y: 4}]` {
		t.Errorf("planar round trip %s", got)
	}
	got = okEval(t, sh, `Geo.fromGeoJSON({type: "Feature", id: 7, properties: {id: "x"}, geometry: null})`)
	if got != `[{id: "x", featureId: 7, geometry: null}]` {
		t.Errorf("feature id and null geometry %s", got)
	}
	got = okEval(t, sh, `Geo.fromGeoJSON({type: "Point", coordinates: [1, 2]}, {geometry: true})`)
	if got != `[{geometry: {type: "Point", coordinates: [1, 2]}}]` {
		t.Errorf("bare geometry kept %s", got)
	}
	mustContain(t, "collision", failEval(t, sh, `Geo.fromGeoJSON({type: "Feature", properties: {lat: 1}, geometry: {type: "Point", coordinates: [1, 2]}})`), `"lat"`, "geometry: true")
	mustContain(t, "string", failEval(t, sh, `Geo.fromGeoJSON("{}")`), "JSON.parse")
}

// Bad input fails with what to do: swapped coordinates, bare pairs, unknown
// types and options, open rings, antimeridian edges.
func TestGeoErrors(t *testing.T) {
	sh := geoShell(t)
	for src, wants := range map[string][]string{
		`Geo.distance({lat: 13.4, lon: 52.5}, {lat: 120, lon: 45})`:                                                                {"b:", "lat must be in -90..90", "look swapped", "Usage: Geo.distance("},
		`Geo.area({type: "Polygon", coordinates: [[[52.5, 13.4], [52.6, 13.4], [52.6, 113.5], [52.5, 13.4]]]})`:                    {"coordinates[0][2]", "looks like [lat, lon]", "[lon, lat]"},
		`Geo.area({type: "Polygon", coordinates: [[[500000, 5800000], [500100, 5800000], [500100, 5800100], [500000, 5800000]]]})`: {"planar: true"},
		`Geo.distance([52.5, 13.4], {lat: 0, lon: 0})`:                                                                             {"ambiguous", "{lat, lon}"},
		`Geo.area([[0, 0], [1, 0], [1, 1]])`:                                                                                       {"ambiguous", "Polygon"},
		`Geo.bbox({type: "polygon", coordinates: []})`:                                                                             {"unsupported GeoJSON type", `did you mean "Polygon"`, "Supported: Point, MultiPoint"},
		`Geo.bbox({type: "Circle", coordinates: [0, 0]})`:                                                                          {"unsupported GeoJSON type", "Supported:"},
		`Geo.area({type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1]]]})`:                                             {"must be closed", "RFC 7946"},
		`Geo.area({type: "Polygon", coordinates: [[[170, 0], [-170, 0], [-170, 5], [170, 0]]]})`:                                   {"antimeridian", "Split"},
		`Geo.distance({lat: 0, lon: 0}, {lat: 0, lon: 1}, {unit: "km"})`:                                                           {`unknown key "unit"`, `did you mean "units"`},
		`Geo.distance({lat: 0, lon: 0}, {lat: 0, lon: 1}, {units: "ft"})`:                                                          {`"m" | "km" | "mi"`},
		`Geo.distance({x: 0, y: 0}, {x: 0, y: 1}, {units: "km"})`:                                                                  {"units applies to lat/lon"},
		`Geo.distance({x: 0, y: 0}, {lat: 0, lon: 1})`:                                                                             {"x/y", "lat/lon"},
		`Geo.distance({lat: 0}, {lat: 0, lon: 1})`:                                                                                 {"needs both lat and lon"},
		`Geo.distance({lat: "1", lon: 0}, {lat: 0, lon: 1})`:                                                                       {"lat must be a finite number"},
		`Geo.kmeans([{x: 0, y: 0}], {})`:                                                                                           {"needs k"},
		`Geo.kmeans([{x: 0, y: 0}, {x: 0, y: 0}], {k: 2})`:                                                                         {"distinct", "lower k"},
		`Geo.dbscan([{x: 0, y: 0}], {})`:                                                                                           {"needs eps"},
		`Geo.dbscan([{x: 0, y: 0}], {eps: 0})`:                                                                                     {"eps must be a number > 0"},
		`Geo.toGeoJSON([{name: "a"}])`:                                                                                             {"rows[0]", "no lat/lon, x/y or geometry"},
		`Geo.bearing({lat: 1, lon: 1}, {lat: 1, lon: 1})`:                                                                          {"equal"},
	} {
		mustContain(t, src, failEval(t, sh, src), wants...)
	}
}

func TestGeoKMeans(t *testing.T) {
	sh := geoShell(t)
	pts := `[{id: "a", x: 0, y: 0}, {id: "b", x: 1, y: 0}, {id: "c", x: 0, y: 1}, {id: "d", x: 10, y: 10}, {id: "e", x: 11, y: 10}, {id: "f", x: 10, y: 11}]`
	got := okEval(t, sh, `let r = Geo.kmeans(`+pts+`, {k: 2, seed: 4});
	[r.method, r.init, r.seed, r.converged, r.inertia, r.clusters |> map(c => c.ids |> join("")) |> sort() |> join(" "), r.rows |> map(x => x.cluster) |> join("")] |> join(";")`)
	parts := strings.Split(got, ";")
	if len(parts) != 7 || parts[0] != "lloyd" || parts[1] != "k-means++" || parts[2] != "4" || parts[3] != "true" || parts[5] != "abc def" {
		t.Errorf("kmeans: %s", got)
	}
	// inertia: each triangle (0,0),(1,0),(0,1) about its mean (1/3, 1/3): 4/3, twice.
	if f := num(t, parts[4]); math.Abs(f-8.0/3) > 1e-9 {
		t.Errorf("inertia %v", f)
	}
	// Same seed, same result.
	a := okEval(t, sh, `JSON.stringify(Geo.kmeans(`+pts+`, {k: 3, seed: 11}))`)
	b := okEval(t, sh, `JSON.stringify(Geo.kmeans(`+pts+`, {k: 3, seed: 11}))`)
	if a != b {
		t.Error("kmeans is not repeatable for a seed")
	}
	// The iteration limit is reported, not hidden.
	got = okEval(t, sh, `let r2 = Geo.kmeans(`+pts+`, {k: 2, init: [{x: 0, y: 0}, {x: 1, y: 0}], iterations: 1}); [r2.converged, r2.stopped, r2.init] |> join(",")`)
	if got != "false,iterations,given" {
		t.Errorf("iteration limit: %s", got)
	}
	// lat/lon clusters across the antimeridian stay together; centers are on the sphere.
	got = okEval(t, sh, `let r3 = Geo.kmeans([{lat: 0, lon: 179.9}, {lat: 0.1, lon: -179.9}, {lat: 0, lon: -179.95}, {lat: 50, lon: 10}, {lat: 50.1, lon: 10.1}], {k: 2, seed: 1});
	  r3.rows |> map(x => x.cluster) |> join("")`)
	if got != "00011" && got != "11100" {
		t.Errorf("antimeridian clusters %s", got)
	}
}

func TestGeoDBSCAN(t *testing.T) {
	sh := geoShell(t)
	got := okEval(t, sh, `let r = Geo.dbscan([{id: 1, x: 0, y: 0}, {id: 2, x: 1, y: 0}, {id: 3, x: 0, y: 1}, {id: 4, x: 10, y: 10}, {id: 5, x: 30, y: 30}, {id: 6, x: 31, y: 30}, {id: 7, x: 30, y: 31}], {eps: 1.5, minPts: 3});
	[r.clusters.length, r.noise, r.rows |> map(x => x.cluster ?? "-") |> join(""), r.clusters[1].ids |> join("")] |> join(";")`)
	if got != "2;1;000-111;567" {
		t.Errorf("dbscan %s", got)
	}
	// 800 m on lat/lon: two shops 500 m apart over the antimeridian are one cluster.
	got = okEval(t, sh, `let r2 = Geo.dbscan([{lat: 0, lon: 179.998}, {lat: 0, lon: -179.9975}, {lat: 0.001, lon: 179.999}, {lat: 40, lon: 10}], {eps: 800, minPts: 2});
	  r2.rows |> map(x => x.cluster ?? "-") |> join("")`)
	if got != "000-" {
		t.Errorf("dbscan antimeridian %s", got)
	}
}

// The guide's routing example: k-means areas, then a TSP per area.
func TestGeoGuideSplitThenRoute(t *testing.T) {
	sh := geoShell(t)
	okEval(t, sh, `load("route")`)
	got := okEval(t, sh, `let depot = {id: "hub", lat: 52.52, lon: 13.40}
	let stops = [{id: "a", lat: 52.53, lon: 13.41}, {id: "b", lat: 52.54, lon: 13.42}, {id: "c", lat: 52.45, lon: 13.30},
	  {id: "d", lat: 52.44, lon: 13.31}, {id: "e", lat: 52.60, lon: 13.55}, {id: "f", lat: 52.61, lon: 13.56}]
	let km = Geo.kmeans(stops, {k: 3, seed: 1})
	let tours = km.rows |> groupBy(r => r.cluster) |> values |> map(g => Route.tsp({points: [depot, ...g], start: depot.id}))
	[km.converged, tours.length, tours |> map(t => t.order.length) |> join(",")] |> join(" ")`)
	if got != "true 3 4,4,4" {
		t.Errorf("split then route: %s", got)
	}
}
