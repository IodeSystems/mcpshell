package toolkit

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/iodesystems/mcpshell/geo"
	. "github.com/iodesystems/mcpshell/runtime"
)

// AddGeo registers the spatial toolkit as the loadable "geo" (namespace Geo).
// It has no outside effect, so it needs no flag.
func AddGeo(sh *Shell) *Shell {
	return sh.AddLoadable(Loadable{
		Name:    "geo",
		Summary: "spatial: distance, nearest, boxes, point-in-polygon, area, hull, GeoJSON rows, k-means and DBSCAN clusters (Geo.*)",
		Guide:   geoGuide,
		Install: func(sh *Shell) (io.Closer, error) {
			for _, c := range geoCommands(sh) {
				sh.Register(c)
			}
			return nil, nil
		},
	})
}

const geoGuide = `Geo — distance, nearest, boxes, polygons, GeoJSON and clusters. load("geo") first.
Points are rows {lat, lon} (degrees; lng works) or {x, y} (plane), as in Route. GeoJSON
positions are [lon, lat]. A geometry argument is a GeoJSON geometry, Feature or
FeatureCollection; where an area is expected, rows [{lat, lon}, ...] are a polygon ring.
lat/lon: distance, bearing, destination, boxes, length and clusters are on a sphere
(haversine, meters; units: "km" | "mi"); area is on the WGS84 ellipsoid (m2; units: "km2" |
"ha" | "mi2" | "acre"). contains, intersects, within, centroid and hull use lon/lat as plane
coordinates (straight edges, as GeoJSON and PostGIS geometry). x/y: all planar, your units.

TYPICAL: Distance, nearest depots, a radius box for a Db R-tree prefilter
  Geo.distance({lat: 52.52, lon: 13.405}, {lat: 48.1351, lon: 11.582}, {units: "km"})  // 504.416...
  Geo.nearest(shop, depots, {k: 2})       // the 2 nearest depot rows, each with dist (m)
  let b = Geo.bbox(shop, {radius: 5000})   // {west, south, east, north, crossesAntimeridian}
  Db.query("SELECT id FROM idx WHERE minLon <= ? AND maxLon >= ? AND minLat <= ? AND maxLat >= ?",
    [b.east, b.west, b.north, b.south])   // then Geo.distance for the exact cut
TYPICAL: Territories: who is inside, area, centroid
  let zone = Geo.fromGeoJSON(JSON.parse(read("zones.geojson")))[0].geometry
  Geo.within(customers, zone)              // the rows inside or on the boundary
  Geo.contains(zone, {lat: 52.5, lon: 13.4}); Geo.area(zone, {units: "km2"}); Geo.centroid(zone)
TYPICAL: Rows <-> GeoJSON
  let fc = Geo.toGeoJSON(rows, {id: "id"})   // Point features; other columns are properties
  Geo.fromGeoJSON(fc)                       // rows: properties + lat, lon (Points) or geometry
TYPICAL: Split stops into areas, then route each
  let km = Geo.kmeans(stops, {k: 3, seed: 1})   // {converged, iterations, inertia, clusters, rows}
  km.rows |> groupBy(r => r.cluster) |> values |> map(g => Route.tsp({points: [depot, ...g], start: depot.id}))
  Geo.dbscan(stops, {eps: 500, minPts: 4})       // groups within 500 m; cluster: null = noise

TRAPS
  k-means is a local search from seeded k-means++ starts: another seed can give a lower
  inertia. It is not optimal. converged: false comes with stopped: "iterations" | "time".
  Bare [a, b] pairs are refused (which one is lat?). |lat| > 90 is an error that names the row.
  A polygon edge over the antimeridian is refused: split it at 180 (RFC 7946 3.1.9).
  bbox across the antimeridian has west > east: query it as two boxes.
  Projected GeoJSON (UTM, web mercator meters): pass {planar: true}.
  Not supported: polygon intersection, union and difference; buffers of lines and polygons
  (Geo.circle buffers a point); ellipsoidal distance (haversine is within 0.5% of it).`

// --- values in ------------------------------------------------------------------

// shape is a parsed geometry argument.
type shape struct {
	g      geo.Geometry
	planar bool
	kind   string // GeoJSON type at the top, "rows" or "point"
}

// rowsAs says what an array of point rows is.
type rowsAs int

const (
	asPoints rowsAs = iota
	asRing
	asLine
)

var geoJSONTypes = []string{"Point", "MultiPoint", "LineString", "MultiLineString", "Polygon", "MultiPolygon", "GeometryCollection", "Feature", "FeatureCollection"}

// rowPoint reads {lat, lon} / {lat, lng} / {x, y} from a row. ok is false
// when the row has none of those keys.
func rowPoint(c rfail, what string, o *ObjectVal) (p geo.Point, planar, ok bool) {
	lat, hasLat := o.Get("lat")
	lon, hasLon := o.Get("lon")
	if !hasLon {
		lon, hasLon = o.Get("lng")
	}
	x, hasX := o.Get("x")
	y, hasY := o.Get("y")
	g := hasLat || hasLon
	switch {
	case !g && !hasX && !hasY:
		return p, false, false
	case g && !(hasLat && hasLon):
		c.fail("%s needs both lat and lon. Row: %s", what, short(o))
	case !g && !(hasX && hasY):
		c.fail("%s needs both x and y. Row: %s", what, short(o))
	case g && (hasX || hasY):
		c.fail("%s has both lat/lon and x/y; use one. Row: %s", what, short(o))
	}
	if !g {
		return geo.Point{X: coord(c, what, "x", x, o), Y: coord(c, what, "y", y, o)}, true, true
	}
	la, lo := coord(c, what, "lat", lat, o), coord(c, what, "lon", lon, o)
	checkLatLon(c, what, la, lo, "row")
	return geo.Point{X: lo, Y: la}, false, true
}

func coord(c rfail, what, k string, v Value, o *ObjectVal) float64 {
	n, ok := v.(*NumberVal)
	if !ok || math.IsNaN(n.V) || math.IsInf(n.V, 0) {
		c.fail("%s: %s must be a finite number, got %s. Row: %s", what, k, describe(v), short(o))
	}
	return n.V
}

// checkLatLon refuses out-of-range coordinates and names a likely swap. form
// is "row" ({lat, lon}) or "position" (GeoJSON [lon, lat]).
func checkLatLon(c rfail, what string, lat, lon float64, form string) {
	if math.Abs(lat) <= 90 && math.Abs(lon) <= 180 {
		return
	}
	swap := math.Abs(lat) > 90 && math.Abs(lat) <= 180 && math.Abs(lon) <= 90
	hint := ""
	switch {
	case swap && form == "row":
		hint = ": lat and lon look swapped"
	case swap:
		hint = ": this looks like [lat, lon]; GeoJSON positions are [lon, lat]"
	case form == "position":
		hint = ". Projected coordinates (meters)? Pass {planar: true}"
	}
	c.fail("%s: lat must be in -90..90 and lon in -180..180, got lat %s, lon %s%s", what, fmtNum(lat), fmtNum(lon), hint)
}

func fmtNum(f float64) string { return Num(f).Display() }

// short is a row for an error, cut to 200 bytes.
func short(v Value) string {
	s := v.Display()
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// point reads one point argument: a row or a GeoJSON Point (or Feature of one).
func point(c rfail, what string, v Value, planarOpt bool) (geo.Point, bool) {
	switch x := v.(type) {
	case *ObjectVal:
		if x.Has("type") {
			s := geoJSON(c, what, x, planarOpt)
			if len(s.g.Points) != 1 || len(s.g.Lines) > 0 || len(s.g.Polys) > 0 {
				c.fail("%s must be one point, got a GeoJSON %s", what, s.kind)
			}
			return s.g.Points[0], s.planar
		}
		p, planar, ok := rowPoint(c, what, x)
		if !ok {
			c.fail("%s must be a point {lat, lon} or {x, y}, got %s", what, short(x))
		}
		return p, planar
	case *ArrayVal:
		c.fail("%s: a bare [a, b] pair is ambiguous (GeoJSON is [lon, lat]); give {lat, lon}, {x, y} or {type: \"Point\", coordinates: [lon, lat]}", what)
	}
	c.fail("%s must be a point {lat, lon} or {x, y}, got %s", what, describe(v))
	return geo.Point{}, false
}

// parseShape reads a geometry argument: GeoJSON, a point row, or an array of
// point rows (as points, a ring or a line) or of GeoJSON objects.
func parseShape(c rfail, what string, v Value, as rowsAs, planarOpt bool) shape {
	switch x := v.(type) {
	case *ObjectVal:
		if x.Has("type") {
			return geoJSON(c, what, x, planarOpt)
		}
		p, planar := point(c, what, x, planarOpt)
		return shape{g: geo.Geometry{Points: []geo.Point{p}}, planar: planar, kind: "point"}
	case *ArrayVal:
		if len(x.Elements) == 0 {
			c.fail("%s is empty", what)
		}
		var pts []geo.Point
		out := shape{kind: "rows"}
		for i, e := range x.Elements {
			w := fmt.Sprintf("%s[%d]", what, i)
			if _, isArr := e.(*ArrayVal); isArr {
				c.fail("%s: bare coordinate arrays are ambiguous (which one is lat?); give rows [{lat, lon}] or a GeoJSON geometry {type: \"Polygon\", coordinates: [[[lon, lat], ...]]}", w)
			}
			if o, ok := e.(*ObjectVal); ok && o.Has("type") && as == asPoints {
				s := geoJSON(c, w, o, planarOpt)
				mergePlanar(c, w, &out, i, s.planar)
				out.g.Add(s.g)
				continue
			}
			p, planar := point(c, w, e, planarOpt)
			mergePlanar(c, w, &out, i, planar)
			pts = append(pts, p)
		}
		switch as {
		case asPoints:
			out.g.Points = append(out.g.Points, pts...)
		case asLine:
			if len(pts) < 2 {
				c.fail("%s: a line needs at least 2 points", what)
			}
			out.g.Lines = [][]geo.Point{pts}
		case asRing:
			if pts[0] != pts[len(pts)-1] {
				pts = append(pts, pts[0])
			}
			if len(pts) < 4 {
				c.fail("%s: a polygon ring needs at least 3 distinct points, got %d", what, len(pts)-1)
			}
			r := geo.Ring(pts)
			if !out.planar {
				checkRingEdges(c, what, r)
			}
			out.g.Polys = []geo.Polygon{{r}}
		}
		return out
	}
	c.fail("%s must be a GeoJSON object, a point {lat, lon} or {x, y}, or an array of points; got %s", what, describe(v))
	return shape{}
}

func mergePlanar(c rfail, what string, s *shape, i int, planar bool) {
	if i == 0 {
		s.planar = planar
	} else if planar != s.planar {
		c.fail("%s: mix of lat/lon and x/y points", what)
	}
}

// checkRingEdges refuses a lon/lat edge longer than 180 degrees of longitude.
func checkRingEdges(c rfail, what string, l []geo.Point) {
	for i := 0; i+1 < len(l); i++ {
		if math.Abs(l[i+1].X-l[i].X) > 180 {
			c.fail("%s: the edge from [%s, %s] to [%s, %s] spans more than 180 degrees of longitude: it crosses the antimeridian (or the ring goes around a pole). Split the geometry at lon 180 (RFC 7946 3.1.9), or shift longitudes to 0..360 and pass {planar: true}",
				what, fmtNum(l[i].X), fmtNum(l[i].Y), fmtNum(l[i+1].X), fmtNum(l[i+1].Y))
		}
	}
}

// geoJSON reads a GeoJSON object (RFC 7946). planarOpt or a "planar: true"
// member (written by this toolkit for x/y data) skips the lon/lat checks.
func geoJSON(c rfail, what string, o *ObjectVal, planarOpt bool) shape {
	tv, _ := o.Get("type")
	tv = orNull(tv)
	ts, ok := tv.(*StringVal)
	if !ok || !contains(geoJSONTypes, ts.V) {
		hint := ""
		if ok {
			for _, t := range geoJSONTypes {
				if strings.EqualFold(t, ts.V) {
					hint = fmt.Sprintf(" (did you mean %q?)", t)
				}
			}
		}
		c.fail("%s: unsupported GeoJSON type %s%s. Supported: %s", what, tv.Inspect(), hint, strings.Join(geoJSONTypes, ", "))
	}
	planar := planarOpt
	if pv, ok := o.Get("planar"); ok && pv.IsTruthy() {
		planar = true
	}
	s := shape{planar: planar, kind: ts.V}
	switch ts.V {
	case "Feature":
		gv, ok := o.Get("geometry")
		if !ok {
			c.fail("%s: a Feature needs a geometry member (null for none)", what)
		}
		if gv == Null {
			return s
		}
		go2, isObj := gv.(*ObjectVal)
		if !isObj {
			c.fail("%s.geometry must be a GeoJSON geometry object, got %s", what, describe(gv))
		}
		inner := geoJSON(c, what+".geometry", go2, planar)
		inner.kind = "Feature"
		return inner
	case "FeatureCollection", "GeometryCollection":
		key := map[string]string{"FeatureCollection": "features", "GeometryCollection": "geometries"}[ts.V]
		mv, _ := o.Get(key)
		arr, isArr := mv.(*ArrayVal)
		if !isArr {
			c.fail("%s: a %s needs %s: [...], got %s", what, ts.V, key, describe(orNull(mv)))
		}
		for i, e := range arr.Elements {
			w := fmt.Sprintf("%s.%s[%d]", what, key, i)
			eo, isObj := e.(*ObjectVal)
			if !isObj || !eo.Has("type") {
				c.fail("%s must be a GeoJSON object, got %s", w, describe(e))
			}
			inner := geoJSON(c, w, eo, planar)
			if i == 0 {
				s.planar = inner.planar
			} else if inner.planar != s.planar {
				c.fail("%s: mix of planar and lon/lat members", w)
			}
			s.g.Add(inner.g)
		}
		return s
	}
	cv, ok := o.Get("coordinates")
	if !ok {
		c.fail("%s: a %s needs coordinates", what, ts.V)
	}
	w := what + ".coordinates"
	switch ts.V {
	case "Point":
		s.g.Points = []geo.Point{position(c, w, cv, planar)}
	case "MultiPoint":
		s.g.Points = positions(c, w, cv, planar, 1)
	case "LineString":
		s.g.Lines = [][]geo.Point{positions(c, w, cv, planar, 2)}
	case "MultiLineString":
		for i, l := range arrayOf(c, w, cv) {
			s.g.Lines = append(s.g.Lines, positions(c, fmt.Sprintf("%s[%d]", w, i), l, planar, 2))
		}
	case "Polygon":
		s.g.Polys = []geo.Polygon{polygon(c, w, cv, planar)}
	case "MultiPolygon":
		for i, p := range arrayOf(c, w, cv) {
			s.g.Polys = append(s.g.Polys, polygon(c, fmt.Sprintf("%s[%d]", w, i), p, planar))
		}
	}
	return s
}

func orNull(v Value) Value {
	if v == nil {
		return Null
	}
	return v
}

func arrayOf(c rfail, what string, v Value) []Value {
	a, ok := v.(*ArrayVal)
	if !ok {
		c.fail("%s must be an array, got %s", what, describe(orNull(v)))
	}
	return a.Elements
}

// position reads [lon, lat] or [lon, lat, alt] (altitude is ignored).
func position(c rfail, what string, v Value, planar bool) geo.Point {
	a, ok := v.(*ArrayVal)
	if !ok || len(a.Elements) < 2 || len(a.Elements) > 3 {
		c.fail("%s must be a position [lon, lat] (or [lon, lat, alt]), got %s", what, short(orNull(v)))
	}
	var xy [2]float64
	for i := range 2 {
		n, isNum := a.Elements[i].(*NumberVal)
		if !isNum || math.IsNaN(n.V) || math.IsInf(n.V, 0) {
			c.fail("%s must hold finite numbers, got %s", what, short(a))
		}
		xy[i] = n.V
	}
	if !planar {
		checkLatLon(c, what, xy[1], xy[0], "position")
	}
	return geo.Point{X: xy[0], Y: xy[1]}
}

func positions(c rfail, what string, v Value, planar bool, min int) []geo.Point {
	els := arrayOf(c, what, v)
	if len(els) < min {
		c.fail("%s needs at least %d positions, got %d", what, min, len(els))
	}
	out := make([]geo.Point, len(els))
	for i, e := range els {
		out[i] = position(c, fmt.Sprintf("%s[%d]", what, i), e, planar)
	}
	return out
}

func polygon(c rfail, what string, v Value, planar bool) geo.Polygon {
	rings := arrayOf(c, what, v)
	if len(rings) == 0 {
		c.fail("%s: a polygon needs an exterior ring", what)
	}
	pg := make(geo.Polygon, len(rings))
	for i, r := range rings {
		w := fmt.Sprintf("%s[%d]", what, i)
		ps := positions(c, w, r, planar, 4)
		if ps[0] != ps[len(ps)-1] {
			c.fail("%s: a ring must be closed: its last position must equal the first (RFC 7946 3.1.6), got %s and %s", w, short(arrayOf(c, w, r)[0]), short(arrayOf(c, w, r)[len(ps)-1]))
		}
		if !planar {
			checkRingEdges(c, w, ps)
		}
		pg[i] = ps
	}
	return pg
}

// pair checks two shapes are both planar or both lon/lat.
func pair(c rfail, a, b shape) bool {
	if a.planar != b.planar {
		c.fail("one argument is x/y (planar) and the other lat/lon; use one kind")
	}
	return a.planar
}

// checkLineEdges refuses lon/lat lines over the antimeridian where the
// planar predicates would misread them.
func checkLineEdges(c rfail, what string, s shape) {
	if s.planar {
		return
	}
	for _, l := range s.g.Lines {
		checkRingEdges(c, what, l)
	}
}

// --- options ---------------------------------------------------------------------

func geoOpts(c rfail, args []Value, i int, keys []string) *ropts {
	v := arg(args, i)
	if v == Null {
		v = NewObject()
	}
	return c.options(v, "opts", keys)
}

var distUnits = map[string]float64{"m": 1, "km": 1000, "mi": 1609.344}
var areaUnits = map[string]float64{"m2": 1, "km2": 1e6, "ha": 1e4, "mi2": 1609.344 * 1609.344, "acre": 4046.8564224}

// unit is the meters per output unit, or 1 for planar data (where units is an error).
func distUnit(o *ropts, planar bool) float64 {
	if planar {
		if _, ok := o.get("units"); ok {
			o.c.fail("units applies to lat/lon; x/y distances are in the x/y units")
		}
		return 1
	}
	return distUnits[o.str("units", "m", "m", "km", "mi")]
}

// num reads an optional finite number option.
func (o *ropts) num(k string, def float64, positive bool) float64 {
	v, ok := o.get(k)
	if !ok {
		return def
	}
	n, isNum := v.(*NumberVal)
	if !isNum || math.IsNaN(n.V) || math.IsInf(n.V, 0) || (positive && n.V <= 0) {
		req := "a finite number"
		if positive {
			req = "a number > 0"
		}
		o.c.fail("%s must be %s, got %s", k, req, describe(v))
	}
	return n.V
}

// --- values out ------------------------------------------------------------------

func pointOut(p geo.Point, planar bool) *ObjectVal {
	o := NewObject()
	if planar {
		o.Set("x", Num(p.X))
		o.Set("y", Num(p.Y))
	} else {
		o.Set("lat", Num(p.Y))
		o.Set("lon", Num(p.X))
	}
	return o
}

func posOut(p geo.Point) *ArrayVal { return Arr(Num(p.X), Num(p.Y)) }

func lineOut(l []geo.Point) *ArrayVal {
	out := make([]Value, len(l))
	for i, p := range l {
		out[i] = posOut(p)
	}
	return &ArrayVal{Elements: out}
}

// geometryOut is a GeoJSON geometry; planar ones carry "planar: true".
func geometryOut(typ string, coords Value, planar bool) *ObjectVal {
	o := NewObject()
	o.Set("type", Str(typ))
	o.Set("coordinates", coords)
	if planar {
		o.Set("planar", Bln(true))
	}
	return o
}

// copyRow is a shallow copy of a row with key set to v; it refuses to
// overwrite a column the row has.
func copyRow(c rfail, what string, row *ObjectVal, key string, v Value) *ObjectVal {
	if row.HasOwn(key) {
		c.fail("%s already has a %q column; pass {key: \"other\"} to name the new one", what, key)
	}
	o := NewObject()
	for _, k := range row.Keys() {
		x, _ := row.Get(k)
		o.Set(k, x)
	}
	o.Set(key, v)
	return o
}

// pointRows reads an array of point rows (not GeoJSON): the points, the rows,
// and whether they are planar.
func pointRows(c rfail, what string, v Value) ([]geo.Point, []*ObjectVal, bool) {
	a, ok := v.(*ArrayVal)
	if !ok {
		c.fail("%s must be an array of rows [{lat, lon}] or [{x, y}], got %s", what, describe(v))
	}
	if len(a.Elements) == 0 {
		c.fail("%s is empty", what)
	}
	pts := make([]geo.Point, len(a.Elements))
	rows := make([]*ObjectVal, len(a.Elements))
	planar := false
	for i, e := range a.Elements {
		w := fmt.Sprintf("%s[%d]", what, i)
		o, isObj := e.(*ObjectVal)
		if !isObj {
			if _, isArr := e.(*ArrayVal); isArr {
				c.fail("%s: a bare [a, b] pair is ambiguous; give rows {lat, lon} or {x, y}", w)
			}
			c.fail("%s must be a row {lat, lon} or {x, y}, got %s", w, describe(e))
		}
		p, pl, has := rowPoint(c, w, o)
		if !has {
			c.fail("%s needs lat and lon, or x and y. Row: %s", w, short(o))
		}
		if i == 0 {
			planar = pl
		} else if pl != planar {
			c.fail("%s: mix of lat/lon and x/y points", w)
		}
		pts[i], rows[i] = p, o
	}
	return pts, rows, planar
}
