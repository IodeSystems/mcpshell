package toolkit

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/iodesystems/mcpshell/geo"
	. "github.com/iodesystems/mcpshell/runtime"
)

func geoCommands(sh *Shell) []*CommandDef {
	cmd := func(name, sig, desc, ex string, fn func(*Shell, []Value) Value) *CommandDef {
		return &CommandDef{Namespace: "Geo", Name: name, Signature: sig, Description: desc,
			Examples: []string{ex}, Fn: func(args []Value) Value { return fn(sh, args) }}
	}
	return []*CommandDef{
		cmd("distance", "a: point, b: point, opts?: {units?}",
			"great-circle (haversine) distance in meters (units: \"km\" | \"mi\"); x/y points: straight-line",
			`Geo.distance({lat: 52.52, lon: 13.405}, {lat: 48.1351, lon: 11.582}, {units: "km"})`, geoDistance),
		cmd("bearing", "from: point, to: point",
			"initial bearing in degrees clockwise from north, 0..360 (x/y: from +y)",
			`Geo.bearing({lat: 52.52, lon: 13.405}, {lat: 48.1351, lon: 11.582})`, geoBearing),
		cmd("destination", "from: point, bearing: number, distance: number, opts?: {units?}",
			"the point reached from `from` along the bearing (degrees) after distance (meters, or units)",
			`Geo.destination({lat: 52.52, lon: 13.405}, 90, 10, {units: "km"})`, geoDestination),
		cmd("bbox", "geometry | rows | point, opts?: {radius?, units?, planar?}",
			"bounding box {west, south, east, north, crossesAntimeridian} (x/y: {minX, minY, maxX, maxY}); with radius: the box of a circle around one point",
			`Geo.bbox({lat: 52.52, lon: 13.405}, {radius: 5000})`, geoBBox),
		cmd("nearest", "from: point, rows: [{lat, lon} | {x, y}], opts?: {k?, maxDist?, units?, key?}",
			"the k nearest rows (default 1), nearest first, each a copy with dist (key) added",
			`Geo.nearest({lat: 52.5, lon: 13.4}, [{id: "a", lat: 52.52, lon: 13.405}, {id: "b", lat: 48.14, lon: 11.58}])`, geoNearest),
		cmd("length", "line: geometry | rows, opts?: {units?, planar?}",
			"length of lines (rows are a path in order), polygon perimeters: great-circle meters, or units",
			`Geo.length([{lat: 52.52, lon: 13.405}, {lat: 52.50, lon: 13.45}, {lat: 48.14, lon: 11.58}], {units: "km"})`, geoLength),
		cmd("area", "polygon: geometry | rows, opts?: {units?, planar?}",
			"area of a Polygon or MultiPolygon, holes subtracted: m2 on the WGS84 ellipsoid (units: km2 | ha | mi2 | acre); x/y: planar",
			`Geo.area([{lat: 0, lon: 0}, {lat: 0, lon: 1}, {lat: 1, lon: 1}, {lat: 1, lon: 0}], {units: "km2"})`, geoArea),
		cmd("centroid", "geometry | rows, opts?: {planar?}",
			"center: polygons by area, lines by length (planar in lon/lat); lat/lon points: the mean on the sphere",
			`Geo.centroid({type: "Polygon", coordinates: [[[0, 0], [4, 0], [4, 4], [0, 4], [0, 0]]]})`, geoCentroid),
		cmd("contains", "area: geometry | rows, geometry: point | geometry, opts?: {planar?}",
			"true when every point of the geometry is inside the area or on its boundary (DE-9IM covers)",
			`Geo.contains([{lat: 0, lon: 0}, {lat: 0, lon: 2}, {lat: 2, lon: 2}, {lat: 2, lon: 0}], {lat: 1, lon: 1})`, geoContains),
		cmd("intersects", "a: geometry, b: geometry, opts?: {planar?}",
			"true when a and b share a point (touching counts)",
			`Geo.intersects({type: "LineString", coordinates: [[0, 0], [2, 2]]}, {type: "LineString", coordinates: [[0, 2], [2, 0]]})`, geoIntersects),
		cmd("within", "rows: [{lat, lon} | {x, y}], area: geometry | rows, opts?: {planar?}",
			"the rows inside the area or on its boundary, in order",
			`Geo.within([{id: 1, lat: 1, lon: 1}, {id: 2, lat: 5, lon: 5}], [{lat: 0, lon: 0}, {lat: 0, lon: 2}, {lat: 2, lon: 2}, {lat: 2, lon: 0}])`, geoWithin),
		cmd("hull", "points: rows | geometry, opts?: {planar?}",
			"convex hull as a GeoJSON Polygon (counterclockwise); a Point or LineString when the points are equal or collinear",
			`Geo.hull([{x: 0, y: 0}, {x: 4, y: 0}, {x: 2, y: 1}, {x: 2, y: 3}])`, geoHull),
		cmd("circle", "center: point, radius: number, opts?: {units?, steps?}",
			"GeoJSON Polygon approximating the circle (steps vertices, default 64): a buffer around a point",
			`Geo.circle({lat: 52.52, lon: 13.405}, 500, {steps: 8})`, geoCircle),
		cmd("toGeoJSON", "rows: [{lat, lon, ...} | {x, y, ...} | {geometry, ...}], opts?: {id?}",
			"FeatureCollection: one Feature per row, Point from lat/lon or the geometry column; other columns are properties",
			`Geo.toGeoJSON([{id: "a", lat: 52.52, lon: 13.405, sales: 10}], {id: "id"})`, geoToGeoJSON),
		cmd("fromGeoJSON", "geojson: FeatureCollection | Feature | [Feature] | geometry, opts?: {geometry?}",
			"rows: properties plus lat, lon for a Point (or geometry: the GeoJSON geometry, always with {geometry: true})",
			`Geo.fromGeoJSON({type: "FeatureCollection", features: [{type: "Feature", geometry: {type: "Point", coordinates: [13.405, 52.52]}, properties: {name: "Berlin"}}]})`, geoFromGeoJSON),
		cmd("kmeans", "rows: [{lat, lon} | {x, y}], opts: {k, seed?, iterations?, init?, units?, key?, timeLimit?}",
			"k-means (Lloyd, k-means++ seeds): a local optimum, not proven best. {k, seed, converged, iterations, inertia, clusters, rows}",
			`Geo.kmeans([{x: 0, y: 0}, {x: 1, y: 0}, {x: 10, y: 10}, {x: 11, y: 10}], {k: 2})`, geoKMeans),
		cmd("dbscan", "rows: [{lat, lon} | {x, y}], opts: {eps, minPts?, units?, key?, timeLimit?}",
			"density clusters: rows within eps (meters, or units) of a core point (minPts rows within eps, itself included; default 4). {clusters, noise, rows}",
			`Geo.dbscan([{x: 0, y: 0}, {x: 1, y: 0}, {x: 0, y: 1}, {x: 10, y: 10}], {eps: 1.5, minPts: 3})`, geoDBSCAN),
	}
}

func nargs(c rfail, args []Value, lo, hi int, sig, ex string) {
	if len(args) < lo || len(args) > hi {
		panic(WrongArguments(string(c), sig, args, ex))
	}
}

func finite(c rfail, what string, v Value) float64 {
	n, ok := v.(*NumberVal)
	if !ok || math.IsNaN(n.V) || math.IsInf(n.V, 0) {
		c.fail("%s must be a finite number, got %s", what, describe(v))
	}
	return n.V
}

func twoPoints(c rfail, args []Value) (geo.Point, geo.Point, bool) {
	a, pa := point(c, "a", args[0], false)
	b, pb := point(c, "b", args[1], false)
	if pa != pb {
		c.fail("one point is x/y and the other lat/lon; use one kind")
	}
	return a, b, pa
}

func geoDistance(_ *Shell, args []Value) Value {
	c := rfail("Geo.distance")
	nargs(c, args, 2, 3, "a, b, opts?", `Geo.distance({lat: 52.52, lon: 13.405}, {lat: 48.1351, lon: 11.582})`)
	a, b, planar := twoPoints(c, args)
	u := distUnit(geoOpts(c, args, 2, []string{"units"}), planar)
	if planar {
		return Num(geo.PlanarDistance(a, b))
	}
	return Num(geo.Distance(a, b) / u)
}

func geoBearing(_ *Shell, args []Value) Value {
	c := rfail("Geo.bearing")
	nargs(c, args, 2, 2, "from, to", `Geo.bearing({lat: 52.52, lon: 13.405}, {lat: 48.1351, lon: 11.582})`)
	a, b, planar := twoPoints(c, args)
	if a == b {
		c.fail("the two points are equal: no bearing")
	}
	if planar {
		return Num(geo.PlanarBearing(a, b))
	}
	if math.Abs(a.Y) == 90 {
		c.fail("from is a pole: every direction is south (or north); no bearing")
	}
	return Num(geo.Bearing(a, b))
}

func geoDestination(_ *Shell, args []Value) Value {
	c := rfail("Geo.destination")
	nargs(c, args, 3, 4, "from, bearing, distance, opts?", `Geo.destination({lat: 52.52, lon: 13.405}, 90, 1000)`)
	p, planar := point(c, "from", args[0], false)
	brg := finite(c, "bearing", args[1])
	d := finite(c, "distance", args[2])
	u := distUnit(geoOpts(c, args, 3, []string{"units"}), planar)
	if planar {
		return pointOut(geo.PlanarDestination(p, brg, d), true)
	}
	return pointOut(geo.Destination(p, brg, d*u), false)
}

func boxOut(b geo.Box, planar bool) *ObjectVal {
	o := NewObject()
	if planar {
		o.Set("minX", Num(b.MinX))
		o.Set("minY", Num(b.MinY))
		o.Set("maxX", Num(b.MaxX))
		o.Set("maxY", Num(b.MaxY))
		return o
	}
	o.Set("west", Num(b.MinX))
	o.Set("south", Num(b.MinY))
	o.Set("east", Num(b.MaxX))
	o.Set("north", Num(b.MaxY))
	o.Set("crossesAntimeridian", Bln(b.CrossesAntimeridian()))
	return o
}

// allPoints lists every coordinate of a geometry.
func allPoints(g geo.Geometry) []geo.Point {
	pts := append([]geo.Point(nil), g.Points...)
	for _, l := range g.Lines {
		pts = append(pts, l...)
	}
	for _, pg := range g.Polys {
		for _, r := range pg {
			pts = append(pts, r...)
		}
	}
	return pts
}

func geoBBox(_ *Shell, args []Value) Value {
	c := rfail("Geo.bbox")
	nargs(c, args, 1, 2, "geometry, opts?", `Geo.bbox([{lat: 52.52, lon: 13.405}, {lat: 48.14, lon: 11.58}])`)
	o := geoOpts(c, args, 1, []string{"radius", "units", "planar"})
	planarOpt := o.boolean("planar", false)
	if _, ok := o.get("radius"); ok {
		p, planar := point(c, "center", args[0], planarOpt)
		r := o.num("radius", 0, false)
		if r < 0 {
			c.fail("radius must be >= 0, got %s", fmtNum(r))
		}
		if planar {
			distUnit(o, true)
			return boxOut(geo.PlanarRadiusBounds(p, r), true)
		}
		return boxOut(geo.RadiusBounds(p, r*distUnit(o, false)), false)
	}
	if _, ok := o.get("units"); ok {
		c.fail("units applies with radius")
	}
	s := parseShape(c, "geometry", args[0], asPoints, planarOpt)
	pts := allPoints(s.g)
	if len(pts) == 0 {
		c.fail("the geometry is empty: no box")
	}
	if s.planar {
		return boxOut(geo.PlanarBounds(pts), true)
	}
	return boxOut(geo.LonLatBounds(pts), false)
}

type ranked struct {
	i int
	d float64
}

func geoNearest(_ *Shell, args []Value) Value {
	c := rfail("Geo.nearest")
	nargs(c, args, 2, 3, "from, rows, opts?", `Geo.nearest({lat: 52.5, lon: 13.4}, depots, {k: 3})`)
	from, planar := point(c, "from", args[0], false)
	pts, rows, rp := pointRows(c, "rows", args[1])
	if rp != planar {
		c.fail("from and rows must both be lat/lon or both x/y")
	}
	o := geoOpts(c, args, 2, []string{"k", "maxDist", "units", "key"})
	k := o.integer("k", 1, 1, len(pts))
	u := distUnit(o, planar)
	maxD := o.num("maxDist", math.Inf(1), false)
	key := o.keyName("dist")
	rs := make([]ranked, len(pts))
	for i, p := range pts {
		d := geo.PlanarDistance(from, p)
		if !planar {
			d = geo.Distance(from, p) / u
		}
		rs[i] = ranked{i, d}
	}
	sort.SliceStable(rs, func(a, b int) bool { return rs[a].d < rs[b].d })
	var out []Value
	for _, r := range rs[:k] {
		if r.d > maxD {
			break
		}
		out = append(out, copyRow(c, fmt.Sprintf("rows[%d]", r.i), rows[r.i], key, Num(r.d)))
	}
	return &ArrayVal{Elements: out}
}

// keyName reads the name of an added column.
func (o *ropts) keyName(def string) string {
	v, ok := o.get("key")
	if !ok {
		return def
	}
	s, isStr := v.(*StringVal)
	if !isStr || s.V == "" {
		o.c.fail("key must be a non-empty string (the column to add), got %s", describe(v))
	}
	return s.V
}

func geoLength(_ *Shell, args []Value) Value {
	c := rfail("Geo.length")
	nargs(c, args, 1, 2, "line, opts?", `Geo.length({type: "LineString", coordinates: [[13.405, 52.52], [11.582, 48.1351]]})`)
	o := geoOpts(c, args, 1, []string{"units", "planar"})
	s := parseShape(c, "line", args[0], asLine, o.boolean("planar", false))
	lines := s.g.Lines
	for _, pg := range s.g.Polys {
		for _, r := range pg {
			lines = append(lines, r)
		}
	}
	if len(lines) == 0 {
		c.fail("needs a LineString, MultiLineString, Polygon or rows in path order; got only points (%s)", s.kind)
	}
	if s.planar {
		distUnit(o, true)
		return Num(geo.PlanarLength(lines))
	}
	return Num(geo.Length(lines) / distUnit(o, false))
}

func geoArea(_ *Shell, args []Value) Value {
	c := rfail("Geo.area")
	nargs(c, args, 1, 2, "polygon, opts?", `Geo.area({type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]]]}, {units: "km2"})`)
	o := geoOpts(c, args, 1, []string{"units", "planar"})
	s := parseShape(c, "polygon", args[0], asRing, o.boolean("planar", false))
	if len(s.g.Polys) == 0 {
		c.fail("needs a Polygon or MultiPolygon (or rows as a ring); got %s with no polygon", s.kind)
	}
	if s.planar {
		if _, ok := o.get("units"); ok {
			c.fail("units applies to lat/lon; x/y areas are in the x/y units squared")
		}
		return Num(geo.PlanarArea(s.g.Polys))
	}
	u := areaUnits[o.str("units", "m2", "m2", "km2", "ha", "mi2", "acre")]
	a, err := geo.GeodesicArea(s.g.Polys)
	if err != nil {
		c.fail("%v", err) // parse refused such edges already
	}
	return Num(a / u)
}

func geoCentroid(_ *Shell, args []Value) Value {
	c := rfail("Geo.centroid")
	nargs(c, args, 1, 2, "geometry, opts?", `Geo.centroid([{lat: 52.52, lon: 13.405}, {lat: 48.14, lon: 11.58}])`)
	o := geoOpts(c, args, 1, []string{"planar"})
	s := parseShape(c, "geometry", args[0], asPoints, o.boolean("planar", false))
	if s.g.Empty() {
		c.fail("the geometry is empty: no centroid")
	}
	if !s.planar && len(s.g.Lines) == 0 && len(s.g.Polys) == 0 {
		p, err := geo.SphereMean(s.g.Points)
		if err != nil {
			c.fail("%v", err)
		}
		return pointOut(p, false)
	}
	checkLineEdges(c, "geometry", s)
	p, _ := geo.Centroid(s.g)
	return pointOut(p, s.planar)
}

// areaShape reads an area argument: polygons only.
func areaShape(c rfail, what string, v Value, planarOpt bool) shape {
	s := parseShape(c, what, v, asRing, planarOpt)
	if len(s.g.Polys) == 0 || len(s.g.Points) > 0 || len(s.g.Lines) > 0 {
		c.fail("%s must be a Polygon or MultiPolygon (or rows as a ring); got %s", what, s.kind)
	}
	return s
}

func geoContains(_ *Shell, args []Value) Value {
	c := rfail("Geo.contains")
	nargs(c, args, 2, 3, "area, geometry, opts?", `Geo.contains(zone, {lat: 52.5, lon: 13.4})`)
	po := geoOpts(c, args, 2, []string{"planar"}).boolean("planar", false)
	a := areaShape(c, "area", args[0], po)
	b := parseShape(c, "geometry", args[1], asPoints, po)
	pair(c, a, b)
	checkLineEdges(c, "geometry", b)
	return Bln(geo.Covers(a.g.Polys, b.g))
}

func geoIntersects(_ *Shell, args []Value) Value {
	c := rfail("Geo.intersects")
	nargs(c, args, 2, 3, "a, b, opts?", `Geo.intersects(zoneA, zoneB)`)
	po := geoOpts(c, args, 2, []string{"planar"}).boolean("planar", false)
	a := parseShape(c, "a", args[0], asPoints, po)
	b := parseShape(c, "b", args[1], asPoints, po)
	pair(c, a, b)
	checkLineEdges(c, "a", a)
	checkLineEdges(c, "b", b)
	return Bln(geo.Intersects(a.g, b.g))
}

func geoWithin(_ *Shell, args []Value) Value {
	c := rfail("Geo.within")
	nargs(c, args, 2, 3, "rows, area, opts?", `Geo.within(customers, zone)`)
	po := geoOpts(c, args, 2, []string{"planar"}).boolean("planar", false)
	pts, rows, planar := pointRows(c, "rows", args[0])
	a := areaShape(c, "area", args[1], po)
	if a.planar != planar {
		c.fail("rows and area must both be lat/lon or both x/y")
	}
	var out []Value
	for i, p := range pts {
		if geo.Locate(a.g.Polys, p) >= 0 {
			out = append(out, rows[i])
		}
	}
	return &ArrayVal{Elements: out}
}

func geoHull(_ *Shell, args []Value) Value {
	c := rfail("Geo.hull")
	nargs(c, args, 1, 2, "points, opts?", `Geo.hull([{x: 0, y: 0}, {x: 4, y: 0}, {x: 2, y: 3}])`)
	o := geoOpts(c, args, 1, []string{"planar"})
	s := parseShape(c, "points", args[0], asPoints, o.boolean("planar", false))
	pts := allPoints(s.g)
	if len(pts) == 0 {
		c.fail("no points")
	}
	if !s.planar && geo.LonLatBounds(pts).CrossesAntimeridian() {
		c.fail("the points span the antimeridian; the hull is computed in lon/lat degrees and would wrap the other way. Shift longitudes (lon < 0 ? lon + 360 : lon) and pass {planar: true}, or split the points")
	}
	h := geo.Hull(pts)
	switch len(h) {
	case 1:
		return geometryOut("Point", posOut(h[0]), s.planar)
	case 2:
		return geometryOut("LineString", lineOut(h), s.planar)
	}
	return geometryOut("Polygon", Arr(lineOut(h)), s.planar)
}

func geoCircle(_ *Shell, args []Value) Value {
	c := rfail("Geo.circle")
	nargs(c, args, 2, 3, "center, radius, opts?", `Geo.circle({lat: 52.52, lon: 13.405}, 500)`)
	p, planar := point(c, "center", args[0], false)
	r := finite(c, "radius", args[1])
	if r <= 0 {
		c.fail("radius must be > 0, got %s", fmtNum(r))
	}
	o := geoOpts(c, args, 2, []string{"units", "steps"})
	steps := o.integer("steps", 64, 4, 4096)
	u := distUnit(o, planar)
	ring := geo.Circle(p, r*u, steps, planar)
	if !planar {
		if r*u >= geo.EarthRadius*math.Pi/2 {
			c.fail("radius %s m is a quarter of the earth or more: the circle is not a polygon in lon/lat", fmtNum(r*u))
		}
		for i := 0; i+1 < len(ring); i++ {
			if math.Abs(ring[i+1].X-ring[i].X) > 180 {
				c.fail("the circle crosses the antimeridian or goes around a pole: as GeoJSON it must be split at lon 180 (RFC 7946 3.1.9), which Geo.circle does not do. Use Geo.bbox with radius, or Geo.distance <= radius")
			}
		}
	}
	return geometryOut("Polygon", Arr(lineOut(ring)), planar)
}

func geoToGeoJSON(_ *Shell, args []Value) Value {
	c := rfail("Geo.toGeoJSON")
	nargs(c, args, 1, 2, "rows, opts?", `Geo.toGeoJSON([{id: "a", lat: 52.52, lon: 13.405}])`)
	o := geoOpts(c, args, 1, []string{"id"})
	idCol := ""
	if v, ok := o.get("id"); ok {
		s, isStr := v.(*StringVal)
		if !isStr || s.V == "" {
			c.fail("id must be the name of a column, got %s", describe(v))
		}
		idCol = s.V
	}
	a, ok := args[0].(*ArrayVal)
	if !ok {
		c.fail("rows must be an array of rows, got %s", describe(args[0]))
	}
	feats := make([]Value, len(a.Elements))
	skip := map[string]bool{"lat": true, "lon": true, "lng": true, "x": true, "y": true, "geometry": true}
	for i, e := range a.Elements {
		w := fmt.Sprintf("rows[%d]", i)
		row, isObj := e.(*ObjectVal)
		if !isObj {
			c.fail("%s must be an object, got %s", w, describe(e))
		}
		f := NewObject()
		f.Set("type", Str("Feature"))
		if idCol != "" {
			id, has := row.Get(idCol)
			switch id.(type) {
			case *StringVal, *NumberVal:
			default:
				if !has {
					c.fail("%s has no %q column (opts.id). Row: %s", w, idCol, short(row))
				}
				c.fail("%s: a Feature id must be a string or number, got %s", w, describe(id))
			}
			f.Set("id", id)
		}
		gv, hasGeom := row.Get("geometry")
		p, planar, hasPt := rowPoint(c, w, row)
		switch {
		case hasGeom && hasPt:
			c.fail("%s has both a geometry column and lat/lon or x/y; keep one", w)
		case hasGeom && gv == Null:
			f.Set("geometry", Null)
		case hasGeom:
			go2, isObj := gv.(*ObjectVal)
			if !isObj || !go2.Has("type") {
				c.fail("%s.geometry must be a GeoJSON geometry, got %s", w, describe(gv))
			}
			geoJSON(c, w+".geometry", go2, false) // validate
			f.Set("geometry", go2)
		case hasPt:
			f.Set("geometry", geometryOut("Point", posOut(p), planar))
		default:
			c.fail("%s has no lat/lon, x/y or geometry column. Row: %s", w, short(row))
		}
		props := NewObject()
		for _, k := range row.Keys() {
			if skip[k] && (k == "geometry" || hasPt) {
				continue
			}
			v, _ := row.Get(k)
			props.Set(k, v)
		}
		f.Set("properties", props)
		feats[i] = f
	}
	out := NewObject()
	out.Set("type", Str("FeatureCollection"))
	out.Set("features", &ArrayVal{Elements: feats})
	return out
}

func geoFromGeoJSON(_ *Shell, args []Value) Value {
	c := rfail("Geo.fromGeoJSON")
	nargs(c, args, 1, 2, "geojson, opts?", `Geo.fromGeoJSON(JSON.parse(text))`)
	keepGeom := geoOpts(c, args, 1, []string{"geometry"}).boolean("geometry", false)
	var feats []Value
	where := func(i int) string { return fmt.Sprintf("features[%d]", i) }
	switch x := args[0].(type) {
	case *ArrayVal:
		feats = x.Elements
	case *ObjectVal:
		tv, _ := x.Get("type")
		t, _ := tv.(*StringVal)
		switch {
		case t != nil && t.V == "FeatureCollection":
			fv, _ := x.Get("features")
			feats = arrayOf(c, "features", fv)
		case t != nil && contains(geoJSONTypes, t.V):
			if t.V != "Feature" {
				// A bare geometry is one row.
				f := NewObject()
				f.Set("type", Str("Feature"))
				f.Set("geometry", x)
				f.Set("properties", NewObject())
				x = f
			}
			feats = []Value{x}
			where = func(int) string { return "geojson" }
		default:
			geoJSON(c, "geojson", x, false) // fails, naming the type problem
		}
	case *StringVal:
		c.fail("geojson must be an object; parse text first: Geo.fromGeoJSON(JSON.parse(text))")
	default:
		c.fail("geojson must be a FeatureCollection, Feature, array of Features or geometry, got %s", describe(args[0]))
	}
	rows := make([]Value, len(feats))
	for i, fv := range feats {
		w := where(i)
		f, isObj := fv.(*ObjectVal)
		tv, _ := orObj(f).Get("type")
		if ts, isStr := tv.(*StringVal); !isObj || !isStr || ts.V != "Feature" {
			c.fail("%s must be a Feature {type: \"Feature\", geometry, properties}, got %s", w, short(orNull(fv)))
		}
		s := geoJSON(c, w, f, false)
		row := NewObject()
		if pv, ok := f.Get("properties"); ok && pv != Null {
			po, isObj := pv.(*ObjectVal)
			if !isObj {
				c.fail("%s.properties must be an object or null, got %s", w, describe(pv))
			}
			for _, k := range po.Keys() {
				v, _ := po.Get(k)
				row.Set(k, v)
			}
		}
		if id, ok := f.Get("id"); ok && id != Null {
			if old, has := row.Get("id"); !has {
				row.Set("id", id)
			} else if !valuesEqual(old, id) {
				row.Set("featureId", id)
			}
		}
		gv, _ := f.Get("geometry")
		isPoint := false
		if g, ok := gv.(*ObjectVal); ok {
			t, _ := g.Get("type")
			isPoint = t != nil && t.Display() == "Point" && !s.planar
		}
		if isPoint && !keepGeom {
			p := s.g.Points[0]
			for _, k := range []string{"lat", "lon"} {
				if row.Has(k) {
					c.fail("%s: properties has %q, which the Point would overwrite; pass {geometry: true} to keep the geometry as a column instead", w, k)
				}
			}
			row.Set("lat", Num(p.Y))
			row.Set("lon", Num(p.X))
		} else {
			if row.Has("geometry") {
				c.fail("%s: properties has \"geometry\", which the geometry column would overwrite", w)
			}
			row.Set("geometry", orNull(gv))
		}
		rows[i] = row
	}
	return &ArrayVal{Elements: rows}
}

func orObj(o *ObjectVal) *ObjectVal {
	if o == nil {
		return NewObject()
	}
	return o
}

// --- clustering --------------------------------------------------------------------

// clusterInput reads the rows of Geo.kmeans / Geo.dbscan as vectors: unit
// vectors on the sphere for lat/lon, [x, y] for planar.
func clusterInput(sh *Shell, c rfail, v Value) ([]geo.Point, []*ObjectVal, [][]float64, bool) {
	pts, rows, planar := pointRows(c, "rows", v)
	if len(pts) > geo.MaxClusterPoints {
		c.fail("%d rows is over the limit of %d", len(pts), geo.MaxClusterPoints)
	}
	sh.Limits().CheckAlloc(int64(len(pts))*(3*8+64), fmt.Sprintf("%s: %d points", string(c), len(pts)))
	X := make([][]float64, len(pts))
	for i, p := range pts {
		if planar {
			X[i] = []float64{p.X, p.Y}
		} else {
			u := geo.Unit(p)
			X[i] = u[:]
		}
	}
	return pts, rows, X, planar
}

// vecPoint is the point of a center vector.
func vecPoint(v []float64, planar bool) geo.Point {
	if planar {
		return geo.Point{X: v[0], Y: v[1]}
	}
	return geo.FromUnit([3]float64{v[0], v[1], v[2]})
}

func pdist(a, b geo.Point, planar bool, u float64) float64 {
	if planar {
		return geo.PlanarDistance(a, b)
	}
	return geo.Distance(a, b) / u
}

// idsOf lists the id of each member when every row has one, else nil.
func idsOf(rows []*ObjectVal, members []int) Value {
	ids := make([]Value, len(members))
	for j, i := range members {
		id, ok := rows[i].Get("id")
		if !ok {
			return nil
		}
		ids[j] = id
	}
	return &ArrayVal{Elements: ids}
}

func geoKMeans(sh *Shell, args []Value) Value {
	c := rfail("Geo.kmeans")
	nargs(c, args, 2, 2, "rows, opts", `Geo.kmeans(stops, {k: 3})`)
	o := c.options(args[1], "opts", []string{"k", "seed", "iterations", "init", "units", "key", "timeLimit"})
	pts, rows, X, planar := clusterInput(sh, c, args[0])
	if _, ok := o.get("k"); !ok {
		c.fail("opts needs k: the number of clusters (1..%d)", min(len(pts), geo.MaxK))
	}
	k := o.integer("k", 0, 1, min(len(pts), geo.MaxK))
	seed := o.integer("seed", 1, 0, math.MaxInt32)
	maxIter := o.integer("iterations", 300, 1, geo.MaxIterations)
	u := distUnit(o, planar)
	key := o.keyName("cluster")
	timeLimit := o.integer("timeLimit", 0, 0, math.MaxInt32)
	initName := "k-means++"
	var centers [][]float64
	if iv, ok := o.get("init"); ok {
		if s, isStr := iv.(*StringVal); !isStr || s.V != "k-means++" {
			initName = "given"
			ia, isArr := iv.(*ArrayVal)
			if !isArr || len(ia.Elements) != k {
				c.fail("init must be \"k-means++\" or an array of %d center points, got %s", k, short(iv))
			}
			for i, e := range ia.Elements {
				p, pl := point(c, fmt.Sprintf("init[%d]", i), e, false)
				if pl != planar {
					c.fail("init[%d] and rows must both be lat/lon or both x/y", i)
				}
				if planar {
					centers = append(centers, []float64{p.X, p.Y})
				} else {
					v := geo.Unit(p)
					centers = append(centers, v[:])
				}
			}
		}
	}
	if centers == nil {
		var err error
		centers, err = geo.KMeansPlusPlus(X, k, rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15)))
		if err != nil {
			c.fail("%v: lower k", err)
		}
	}
	ctx, done := routeContext(sh, c, timeLimit)
	res := geo.KMeans(ctx, X, centers, maxIter, !planar)
	done()
	if res.Stopped && sh.Limits().Cancelled() {
		c.fail("Execution cancelled")
	}
	// Report: clusters with center, size, ids, distances; rows with the cluster column.
	members := make([][]int, k)
	for i, l := range res.Labels {
		members[l] = append(members[l], i)
	}
	inertia := 0.0
	cl := make([]Value, k)
	for j := range k {
		ctr := vecPoint(res.Centers[j], planar)
		maxD, sumD := 0.0, 0.0
		for _, i := range members[j] {
			d := pdist(pts[i], ctr, planar, u)
			maxD, sumD = math.Max(maxD, d), sumD+d
			inertia += d * d
		}
		co := NewObject()
		co.Set("cluster", Num(float64(j)))
		co.Set("center", pointOut(ctr, planar))
		co.Set("size", Num(float64(len(members[j]))))
		if ids := idsOf(rows, members[j]); ids != nil {
			co.Set("ids", ids)
		}
		co.Set("maxDist", Num(maxD))
		mean := 0.0
		if len(members[j]) > 0 {
			mean = sumD / float64(len(members[j]))
		}
		co.Set("meanDist", Num(mean))
		cl[j] = co
	}
	outRows := make([]Value, len(rows))
	for i, r := range rows {
		outRows[i] = copyRow(c, fmt.Sprintf("rows[%d]", i), r, key, Num(float64(res.Labels[i])))
	}
	out := NewObject()
	out.Set("method", Str("lloyd"))
	out.Set("init", Str(initName))
	out.Set("k", Num(float64(k)))
	out.Set("seed", Num(float64(seed)))
	out.Set("converged", Bln(res.Converged))
	if !res.Converged {
		reason := "iterations"
		if res.Stopped {
			reason = "time"
		}
		out.Set("stopped", Str(reason))
		sh.Note(fmt.Sprintf("Geo.kmeans: stopped by the %s limit after %d iterations before converging; the clusters are usable but not a fixed point. Raise iterations or the time limit", reason, res.Iterations))
	}
	out.Set("iterations", Num(float64(res.Iterations)))
	out.Set("inertia", Num(inertia))
	out.Set("clusters", &ArrayVal{Elements: cl})
	out.Set("rows", &ArrayVal{Elements: outRows})
	return out
}

func geoDBSCAN(sh *Shell, args []Value) Value {
	c := rfail("Geo.dbscan")
	nargs(c, args, 2, 2, "rows, opts", `Geo.dbscan(stops, {eps: 500})`)
	o := c.options(args[1], "opts", []string{"eps", "minPts", "units", "key", "timeLimit"})
	pts, rows, X, planar := clusterInput(sh, c, args[0])
	if _, ok := o.get("eps"); !ok {
		c.fail("opts needs eps: the neighbor distance (meters for lat/lon, or units)")
	}
	eps := o.num("eps", 0, true)
	minPts := o.integer("minPts", 4, 1, len(pts)+1)
	u := distUnit(o, planar)
	key := o.keyName("cluster")
	timeLimit := o.integer("timeLimit", 0, 0, math.MaxInt32)
	geps := eps
	if !planar {
		ang := eps * u / geo.EarthRadius
		if ang >= math.Pi {
			geps = 2.000001 // every pair: the chord is at most 2
		} else {
			geps = 2 * math.Sin(ang/2) // chord on the unit sphere
		}
	}
	ctx, done := routeContext(sh, c, timeLimit)
	labels, _, err := geo.DBSCAN(ctx, X, geps, minPts)
	done()
	switch {
	case errors.Is(err, geo.ErrEpsSmall):
		c.fail("eps %s is too small for these coordinates (more than 1e15 cells)", fmtNum(eps))
	case err != nil && sh.Limits().Cancelled():
		c.fail("Execution cancelled")
	case err != nil:
		lm := sh.Limits()
		c.fail("stopped at the time limit. Raise the limit (%s) or timeLimit, or give fewer rows", lm.RaiseHint("timeout", lm.TimeoutMs*2))
	}
	var members [][]int
	noise := 0
	for i, l := range labels {
		if l == geo.Noise {
			noise++
			continue
		}
		for len(members) <= l {
			members = append(members, nil)
		}
		members[l] = append(members[l], i)
	}
	cl := make([]Value, len(members))
	for j, m := range members {
		mp := make([]geo.Point, len(m))
		for t, i := range m {
			mp[t] = pts[i]
		}
		var ctr geo.Point
		if planar {
			ctr, _ = geo.Centroid(geo.Geometry{Points: mp})
		} else if ctr, err = geo.SphereMean(mp); err != nil {
			c.fail("cluster %d: %v", j, err)
		}
		co := NewObject()
		co.Set("cluster", Num(float64(j)))
		co.Set("center", pointOut(ctr, planar))
		co.Set("size", Num(float64(len(m))))
		if ids := idsOf(rows, m); ids != nil {
			co.Set("ids", ids)
		}
		cl[j] = co
	}
	outRows := make([]Value, len(rows))
	for i, r := range rows {
		var v Value = Null
		if labels[i] != geo.Noise {
			v = Num(float64(labels[i]))
		}
		outRows[i] = copyRow(c, fmt.Sprintf("rows[%d]", i), r, key, v)
	}
	out := NewObject()
	out.Set("eps", Num(eps))
	out.Set("minPts", Num(float64(minPts)))
	out.Set("clusters", &ArrayVal{Elements: cl})
	out.Set("noise", Num(float64(noise)))
	out.Set("rows", &ArrayVal{Elements: outRows})
	return out
}
