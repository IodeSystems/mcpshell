package geo

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"testing"
)

// testdata/refs.json comes from testdata/gen_refs.py: shapely 2.2 (covers,
// intersects, centroid, convex hull), pyproj 3.8 Geod WGS84 (area),
// scikit-learn 1.9 (haversine_distances, KMeans, DBSCAN).

type refs struct {
	Predicates []struct {
		A, B               gj
		Covers, Intersects bool
	}
	Centroids []struct {
		G        gj
		Centroid [2]float64
		Area     float64
	}
	Hulls []struct {
		Points [][2]float64
		Hull   gj
		Area   float64
	}
	Areas []struct {
		Name string
		Ring [][2]float64
		Geom *gj
		Area float64
	}
	Distances []struct {
		A, B struct{ Lat, Lon float64 }
		Dist float64
	}
	Kmeans struct {
		Points  [][]float64
		Init    []int
		Labels  []int
		Centers [][]float64
		Inertia float64
	}
	Dbscan struct {
		Latlon  [][2]float64
		Eps     float64
		MinPts  int
		Labels  []int
		Core    []int
		XY      [][]float64 `json:"xy"`
		Eps2    float64
		MinPts2 int
		Labels2 []int
		Core2   []int
	}
}

type gj struct {
	Type        string
	Coordinates json.RawMessage
}

func (g gj) geom(t *testing.T) Geometry {
	t.Helper()
	var out Geometry
	pt := func(c []float64) Point { return Point{c[0], c[1]} }
	line := func(cs [][]float64) []Point {
		l := make([]Point, len(cs))
		for i, c := range cs {
			l[i] = pt(c)
		}
		return l
	}
	poly := func(rs [][][]float64) Polygon {
		p := make(Polygon, len(rs))
		for i, r := range rs {
			p[i] = line(r)
		}
		return p
	}
	var err error
	switch g.Type {
	case "Point":
		var c []float64
		err = json.Unmarshal(g.Coordinates, &c)
		out.Points = []Point{pt(c)}
	case "LineString":
		var c [][]float64
		err = json.Unmarshal(g.Coordinates, &c)
		out.Lines = [][]Point{line(c)}
	case "Polygon":
		var c [][][]float64
		err = json.Unmarshal(g.Coordinates, &c)
		out.Polys = []Polygon{poly(c)}
	case "MultiPolygon":
		var c [][][][]float64
		err = json.Unmarshal(g.Coordinates, &c)
		for _, p := range c {
			out.Polys = append(out.Polys, poly(p))
		}
	default:
		t.Fatalf("type %s", g.Type)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func loadRefs(t *testing.T) refs {
	t.Helper()
	b, err := os.ReadFile("testdata/refs.json")
	if err != nil {
		t.Fatal(err)
	}
	var r refs
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func near(a, b, rel float64) bool { return math.Abs(a-b) <= rel*math.Max(1, math.Abs(b)) }

func TestPredicatesMatchShapely(t *testing.T) {
	r := loadRefs(t)
	for i, c := range r.Predicates {
		a, b := c.A.geom(t), c.B.geom(t)
		if got := Covers(a.Polys, b); got != c.Covers {
			t.Errorf("case %d covers = %v, shapely %v\n a=%s\n b=%s", i, got, c.Covers, c.A.Coordinates, c.B.Coordinates)
		}
		if got := Intersects(a, b); got != c.Intersects {
			t.Errorf("case %d intersects = %v, shapely %v\n a=%s\n b=%s", i, got, c.Intersects, c.A.Coordinates, c.B.Coordinates)
		}
		if got := Intersects(b, a); got != c.Intersects {
			t.Errorf("case %d intersects(b, a) = %v, shapely %v", i, got, c.Intersects)
		}
	}
}

func TestCentroidAndAreaMatchShapely(t *testing.T) {
	r := loadRefs(t)
	for i, c := range r.Centroids {
		g := c.G.geom(t)
		p, ok := Centroid(g)
		if !ok || !near(p.X, c.Centroid[0], 1e-9) || !near(p.Y, c.Centroid[1], 1e-9) {
			t.Errorf("case %d centroid = %v, shapely %v", i, p, c.Centroid)
		}
		if a := PlanarArea(g.Polys); !near(a, c.Area, 1e-9) {
			t.Errorf("case %d area = %v, shapely %v", i, a, c.Area)
		}
	}
}

func TestHullMatchesShapely(t *testing.T) {
	r := loadRefs(t)
	for i, c := range r.Hulls {
		pts := make([]Point, len(c.Points))
		for j, p := range c.Points {
			pts[j] = Point{p[0], p[1]}
		}
		h := Hull(pts)
		want := c.Hull.geom(t)
		if len(want.Polys) == 0 {
			t.Fatalf("case %d: shapely hull is %s", i, c.Hull.Type)
		}
		if len(h) != len(want.Polys[0][0]) {
			t.Errorf("case %d: %d vertices, shapely %d", i, len(h), len(want.Polys[0][0]))
		}
		if a := ringArea(h); a <= 0 || !near(a, c.Area, 1e-9) {
			t.Errorf("case %d: area %v (must be > 0: counterclockwise), shapely %v", i, a, c.Area)
		}
		for _, p := range pts {
			if locateRing(h, p) < 0 {
				t.Errorf("case %d: %v outside the hull", i, p)
			}
		}
	}
	if h := Hull([]Point{{0, 0}, {1, 1}, {2, 2}, {1, 1}}); len(h) != 2 || h[0] != (Point{0, 0}) || h[1] != (Point{2, 2}) {
		t.Errorf("collinear hull = %v", h)
	}
	if h := Hull([]Point{{3, 3}, {3, 3}}); len(h) != 1 {
		t.Errorf("one point hull = %v", h)
	}
}

// GeodesicArea uses the authalic sphere; pyproj integrates on the ellipsoid
// with geodesic edges. The difference is the edge shape only.
func TestGeodesicAreaMatchesPyproj(t *testing.T) {
	r := loadRefs(t)
	for _, c := range r.Areas {
		var pgs []Polygon
		if c.Geom != nil {
			pgs = c.Geom.geom(t).Polys
		} else {
			ring := make(Ring, len(c.Ring))
			for i, p := range c.Ring {
				ring[i] = Point{p[0], p[1]}
			}
			pgs = []Polygon{{ring}}
		}
		a, err := GeodesicArea(pgs)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		rel := math.Abs(a-c.Area) / c.Area
		t.Logf("%-32s area %.6g m2, pyproj %.6g, relative difference %.2g", c.Name, a, c.Area, rel)
		// Edges hundreds of km long: great circle on the authalic sphere vs geodesic.
		limit := map[string]float64{"large: 60 degrees wide": 1e-3, "triangle 0,0 10,0 0,10": 1e-4,
			"high latitude": 5e-5, "Colorado (corners)": 1e-5, "southern, clockwise": 1e-5}[c.Name]
		if limit == 0 {
			limit = 1e-6
		}
		if rel > limit {
			t.Errorf("%s: area %v, pyproj %v (relative %g > %g)", c.Name, a, c.Area, rel, limit)
		}
	}
	// A ring around a pole needs an edge over the antimeridian; both are refused.
	for _, r := range []Ring{
		{{0, 80}, {120, 80}, {-120, 80}, {0, 80}},
		{{-180, 80}, {-90, 80}, {0, 80}, {90, 80}, {180, 80}, {-180, 80}},
		{{170, 0}, {-170, 0}, {-170, 5}, {170, 0}},
	} {
		if _, err := GeodesicArea([]Polygon{{r}}); err != ErrAntimeridian {
			t.Errorf("%v: %v", r, err)
		}
	}
}

func TestDistanceMatchesSklearn(t *testing.T) {
	r := loadRefs(t)
	for _, c := range r.Distances {
		d := Distance(Point{c.A.Lon, c.A.Lat}, Point{c.B.Lon, c.B.Lat})
		if !near(d, c.Dist, 1e-9) {
			t.Errorf("%v -> %v: %v, sklearn %v", c.A, c.B, d, c.Dist)
		}
	}
}

// Published: C. Veness, "Calculate distance, bearing and more between
// Latitude/Longitude points" (movable-type.co.uk/scripts/latlong.html), on
// R = 6371 km: Land's End 50 03 59N 005 42 53W to John o' Groats 58 38 38N
// 003 04 12W is 968.9 km at initial bearing 009 07 11; from 53 19 14N
// 001 43 47W, 124.8 km at 096 01 18 reaches 53 11 18N 000 08 00E.
func TestBearingAndDestinationPublished(t *testing.T) {
	dms := func(d, m, s float64) float64 { return d + m/60 + s/3600 }
	le := Point{-dms(5, 42, 53), dms(50, 3, 59)}
	jog := Point{-dms(3, 4, 12), dms(58, 38, 38)}
	if d := Distance(le, jog) * 6371 / (EarthRadius / 1000); math.Abs(d/1000-968.9) > 0.05 {
		t.Errorf("Land's End distance %v km", d/1000)
	}
	if b := Bearing(le, jog); math.Abs(b-dms(9, 7, 11)) > 1.0/3600 {
		t.Errorf("bearing %v", b)
	}
	p := Destination(Point{-dms(1, 43, 47), dms(53, 19, 14)}, dms(96, 1, 18), 124800*EarthRadius/6371000)
	if math.Abs(p.Y-dms(53, 11, 18)) > 1.0/3600 || math.Abs(p.X-dms(0, 8, 0)) > 1.0/3600 {
		t.Errorf("destination %v", p)
	}
	// Over a pole and the antimeridian.
	if b := Bearing(Point{0, 89}, Point{180, 89}); math.Abs(b) > 1e-9 {
		t.Errorf("bearing over the north pole %v", b)
	}
	if b := Bearing(Point{179.5, 0}, Point{-179.5, 0}); math.Abs(b-90) > 1e-9 {
		t.Errorf("bearing east over the antimeridian %v", b)
	}
	if p := Destination(Point{179.5, 0}, 90, Distance(Point{0, 0}, Point{1, 0})); math.Abs(p.X+179.5) > 1e-9 || math.Abs(p.Y) > 1e-9 {
		t.Errorf("destination over the antimeridian %v", p)
	}
	if p := Destination(Point{0, 89}, 0, Distance(Point{0, 89}, Point{0, 90})*2); math.Abs(p.Y-89) > 1e-9 || math.Abs(math.Abs(p.X)-180) > 1e-9 {
		t.Errorf("destination over the pole %v", p)
	}
}

func TestBounds(t *testing.T) {
	b := LonLatBounds([]Point{{179, 1}, {-179, 2}, {178, -1}})
	if b != (Box{178, -1, -179, 2}) || !b.CrossesAntimeridian() {
		t.Errorf("antimeridian box %v", b)
	}
	b = LonLatBounds([]Point{{-10, 0}, {10, 0}})
	if b != (Box{-10, 0, 10, 0}) || b.CrossesAntimeridian() {
		t.Errorf("plain box %v", b)
	}
	// RadiusBounds holds every point of the circle and touches it on each side.
	rng := rand.New(rand.NewPCG(1, 2))
	for _, c := range []struct {
		p Point
		r float64
	}{{Point{13.4, 52.5}, 5000}, {Point{179.99, 10}, 20000}, {Point{-179.99, -10}, 20000}, {Point{0, 89.9}, 50000}, {Point{40, -89.95}, 10000}, {Point{0, 0}, 3e6}} {
		bb := RadiusBounds(c.p, c.r)
		var seen []Point
		for range 4000 {
			q := Destination(c.p, rng.Float64()*360, c.r*math.Sqrt(rng.Float64()))
			seen = append(seen, q)
			if !inBox(bb, q, 1e-9) {
				t.Fatalf("%v r=%v: %v outside %v", c.p, c.r, q, bb)
			}
		}
		for i := range 3600 {
			seen = append(seen, Destination(c.p, float64(i)/10, c.r))
		}
		// A circle over a pole reaches it; sampling may not land on it exactly.
		for _, pole := range []Point{{0, 90}, {0, -90}} {
			if Distance(c.p, pole) <= c.r {
				seen = append(seen, pole)
			}
		}
		got := LonLatBounds(seen)
		if math.Abs(got.MinY-bb.MinY) > 1e-3 || math.Abs(got.MaxY-bb.MaxY) > 1e-3 {
			t.Errorf("%v r=%v: lat %v..%v, sampled %v..%v", c.p, c.r, bb.MinY, bb.MaxY, got.MinY, got.MaxY)
		}
		if bb.MinX != -180 && (math.Abs(NormLon(got.MinX-bb.MinX)) > 1e-3 || math.Abs(NormLon(got.MaxX-bb.MaxX)) > 1e-3) {
			t.Errorf("%v r=%v: lon %v..%v, sampled %v..%v", c.p, c.r, bb.MinX, bb.MaxX, got.MinX, got.MaxX)
		}
	}
	if b := RadiusBounds(Point{0, 89.9}, 50000); b.MinX != -180 || b.MaxX != 180 || b.MaxY != 90 {
		t.Errorf("over the pole %v", b)
	}
	if b := RadiusBounds(Point{179.99, 10}, 20000); !b.CrossesAntimeridian() {
		t.Errorf("over the antimeridian %v", b)
	}
}

func inBox(b Box, p Point, e float64) bool {
	if p.Y < b.MinY-e || p.Y > b.MaxY+e {
		return false
	}
	if b.CrossesAntimeridian() {
		return p.X >= b.MinX-e || p.X <= b.MaxX+e
	}
	return p.X >= b.MinX-e && p.X <= b.MaxX+e
}

func TestKMeansMatchesSklearn(t *testing.T) {
	k := loadRefs(t).Kmeans
	init := make([][]float64, len(k.Init))
	for i, j := range k.Init {
		init[i] = clone(k.Points[j])
	}
	res := KMeans(context.Background(), k.Points, init, 300, false)
	if !res.Converged {
		t.Fatal("not converged")
	}
	inertia := 0.0
	for i, l := range res.Labels {
		if l != k.Labels[i] {
			t.Fatalf("label %d = %d, sklearn %d", i, l, k.Labels[i])
		}
		inertia += sqDist(k.Points[i], res.Centers[l])
	}
	for c := range res.Centers {
		for j := range res.Centers[c] {
			if !near(res.Centers[c][j], k.Centers[c][j], 1e-9) {
				t.Errorf("center %d = %v, sklearn %v", c, res.Centers[c], k.Centers[c])
			}
		}
	}
	if !near(inertia, k.Inertia, 1e-9) {
		t.Errorf("inertia %v, sklearn %v", inertia, k.Inertia)
	}
	// k-means++ with a seed repeats.
	a, _ := KMeansPlusPlus(k.Points, 3, rand.New(rand.NewPCG(9, 9)))
	b, _ := KMeansPlusPlus(k.Points, 3, rand.New(rand.NewPCG(9, 9)))
	for i := range a {
		if a[i][0] != b[i][0] || a[i][1] != b[i][1] {
			t.Fatal("k-means++ is not repeatable")
		}
	}
	if _, err := KMeansPlusPlus([][]float64{{1, 1}, {1, 1}, {2, 2}}, 3, rand.New(rand.NewPCG(1, 1))); err == nil {
		t.Error("k > distinct points: want an error")
	}
}

func TestDBSCANMatchesSklearn(t *testing.T) {
	d := loadRefs(t).Dbscan
	check := func(name string, X [][]float64, eps float64, minPts int, labels, coreIdx []int) {
		got, core, err := DBSCAN(context.Background(), X, eps, minPts)
		if err != nil {
			t.Fatal(err)
		}
		for i := range got {
			if got[i] != labels[i] {
				t.Fatalf("%s: label %d = %d, sklearn %d", name, i, got[i], labels[i])
			}
		}
		n := 0
		for _, c := range core {
			if c {
				n++
			}
		}
		if n != len(coreIdx) {
			t.Fatalf("%s: %d core points, sklearn %d", name, n, len(coreIdx))
		}
		for _, i := range coreIdx {
			if !core[i] {
				t.Errorf("%s: %d is core in sklearn", name, i)
			}
		}
	}
	X := make([][]float64, len(d.Latlon))
	for i, p := range d.Latlon {
		u := Unit(Point{p[1], p[0]})
		X[i] = u[:]
	}
	chord := 2 * math.Sin(d.Eps/EarthRadius/2)
	check("haversine", X, chord, d.MinPts, d.Labels, d.Core)
	check("euclidean", d.XY, d.Eps2, d.MinPts2, d.Labels2, d.Core2)
}

func TestPlanarBasics(t *testing.T) {
	sq := Polygon{{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}}}
	for p, want := range map[Point]int{{2, 2}: 1, {0, 2}: 0, {4, 4}: 0, {5, 2}: -1} {
		if got := LocatePolygon(sq, p); got != want {
			t.Errorf("locate %v = %d, want %d", p, got, want)
		}
	}
	if a := PlanarArea([]Polygon{sq}); a != 16 {
		t.Errorf("area %v", a)
	}
	c := Circle(Point{0, 0}, 1, 4, true)
	if len(c) != 5 || ringArea(c) <= 0 {
		t.Errorf("circle %v is not a closed counterclockwise ring", c)
	}
	if b := PlanarBearing(Point{0, 0}, Point{1, 0}); b != 90 {
		t.Errorf("planar bearing %v", b)
	}
	if m, err := SphereMean([]Point{{179, 0}, {-179, 0}}); err != nil || math.Abs(math.Abs(m.X)-180) > 1e-9 {
		t.Errorf("mean over the antimeridian %v %v", m, err)
	}
	if _, err := SphereMean([]Point{{0, 0}, {180, 0}}); err != ErrSpread {
		t.Errorf("antipodal mean: %v", err)
	}
}
