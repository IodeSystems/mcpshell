package geo

import (
	"math"
	"slices"
)

// Planar predicates. Edges are straight lines in the coordinates (for lon/lat:
// straight in degrees, as GeoJSON and PostGIS geometry treat them). A point on
// a boundary counts as covered: Covers is the "covers" of DE-9IM (shapely
// covers), so a stop on a territory line belongs to it.

func orient(a, b, c Point) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

// onSegment reports p on the segment a-b, with a tolerance relative to its length.
func onSegment(a, b, p Point) bool {
	s := math.Abs(b.X-a.X) + math.Abs(b.Y-a.Y)
	if s == 0 {
		return p == a
	}
	e := 1e-12 * s
	if p.X < math.Min(a.X, b.X)-e || p.X > math.Max(a.X, b.X)+e || p.Y < math.Min(a.Y, b.Y)-e || p.Y > math.Max(a.Y, b.Y)+e {
		return false
	}
	return math.Abs(orient(a, b, p)) <= 1e-12*s*s
}

// locateRing is 1 when p is inside the ring, 0 on it, -1 outside (even-odd rule).
func locateRing(r Ring, p Point) int {
	in := false
	for i := 0; i+1 < len(r); i++ {
		a, b := r[i], r[i+1]
		if onSegment(a, b, p) {
			return 0
		}
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < a.X+(p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y) {
			in = !in
		}
	}
	if in {
		return 1
	}
	return -1
}

// LocatePolygon is 1 inside the polygon, 0 on its boundary, -1 outside or in a hole.
func LocatePolygon(pg Polygon, p Point) int {
	l := locateRing(pg[0], p)
	if l <= 0 {
		return l
	}
	for _, h := range pg[1:] {
		switch locateRing(h, p) {
		case 0:
			return 0
		case 1:
			return -1
		}
	}
	return 1
}

// Locate is the best LocatePolygon over the polygons.
func Locate(pgs []Polygon, p Point) int {
	best := -1
	for _, pg := range pgs {
		if l := LocatePolygon(pg, p); l > best {
			best = l
			if best == 1 {
				break
			}
		}
	}
	return best
}

func segsIntersect(p1, p2, q1, q2 Point) bool {
	d1, d2 := orient(q1, q2, p1), orient(q1, q2, p2)
	d3, d4 := orient(p1, p2, q1), orient(p1, p2, q2)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return onSegment(q1, q2, p1) || onSegment(q1, q2, p2) || onSegment(p1, p2, q1) || onSegment(p1, p2, q2)
}

// segments lists every edge of the lines and polygon rings of g.
func segments(g Geometry) [][2]Point {
	var s [][2]Point
	add := func(l []Point) {
		for i := 0; i+1 < len(l); i++ {
			s = append(s, [2]Point{l[i], l[i+1]})
		}
	}
	for _, l := range g.Lines {
		add(l)
	}
	for _, pg := range g.Polys {
		for _, r := range pg {
			add(r)
		}
	}
	return s
}

// coversSegment reports every point of p1-p2 covered by the polygons: the
// segment is cut where it meets their edges, and each piece's midpoint and
// the ends are located.
func coversSegment(pgs []Polygon, edges [][2]Point, p1, p2 Point) bool {
	if Locate(pgs, p1) < 0 || Locate(pgs, p2) < 0 {
		return false
	}
	d := Point{p2.X - p1.X, p2.Y - p1.Y}
	dd := d.X*d.X + d.Y*d.Y
	if dd == 0 {
		return true
	}
	ts := []float64{0, 1}
	param := func(q Point) float64 { return ((q.X-p1.X)*d.X + (q.Y-p1.Y)*d.Y) / dd }
	for _, e := range edges {
		q1, q2 := e[0], e[1]
		if onSegment(p1, p2, q1) {
			ts = append(ts, param(q1))
		}
		if onSegment(p1, p2, q2) {
			ts = append(ts, param(q2))
		}
		r := Point{q2.X - q1.X, q2.Y - q1.Y}
		den := d.X*r.Y - d.Y*r.X
		if den == 0 {
			continue
		}
		t := ((q1.X-p1.X)*r.Y - (q1.Y-p1.Y)*r.X) / den
		u := ((q1.X-p1.X)*d.Y - (q1.Y-p1.Y)*d.X) / den
		if t > 0 && t < 1 && u >= 0 && u <= 1 {
			ts = append(ts, t)
		}
	}
	slices.Sort(ts)
	for i := 0; i+1 < len(ts); i++ {
		if ts[i+1]-ts[i] < 1e-12 {
			continue
		}
		m := (ts[i] + ts[i+1]) / 2
		if Locate(pgs, Point{p1.X + m*d.X, p1.Y + m*d.Y}) < 0 {
			return false
		}
	}
	return true
}

// Covers reports every point of b inside or on the boundary of the polygons a.
// Exact for valid polygons; a hole of a that only touches b's boundary at
// single points is the one degenerate case it may misjudge.
func Covers(a []Polygon, b Geometry) bool {
	if len(a) == 0 || b.Empty() {
		return false
	}
	for _, p := range b.Points {
		if Locate(a, p) < 0 {
			return false
		}
	}
	edges := segments(Geometry{Polys: a})
	for _, s := range segments(b) {
		if !coversSegment(a, edges, s[0], s[1]) {
			return false
		}
	}
	// b's boundary is covered. Its interior can still hold a hole of a.
	for _, pg := range a {
		for _, h := range pg[1:] {
			for _, v := range h {
				if Locate(b.Polys, v) == 1 {
					return false
				}
			}
			if q, ok := interiorPoint(h); ok && Locate(b.Polys, q) == 1 && Locate(a, q) < 0 {
				return false
			}
		}
	}
	return true
}

// Intersects reports a and b sharing at least one point (boundaries count).
func Intersects(a, b Geometry) bool {
	if a.Empty() || b.Empty() {
		return false
	}
	sa, sb := segments(a), segments(b)
	pointHits := func(pts []Point, g Geometry, segs [][2]Point) bool {
		for _, p := range pts {
			if slices.Contains(g.Points, p) || Locate(g.Polys, p) >= 0 {
				return true
			}
			for _, s := range segs {
				if onSegment(s[0], s[1], p) {
					return true
				}
			}
		}
		return false
	}
	if pointHits(a.Points, b, sb) || pointHits(b.Points, a, sa) {
		return true
	}
	for _, s := range sa {
		for _, t := range sb {
			if segsIntersect(s[0], s[1], t[0], t[1]) {
				return true
			}
		}
	}
	// No edges meet: a part of one is inside the other or apart. One vertex per part decides.
	firsts := func(g Geometry) []Point {
		var v []Point
		for _, l := range g.Lines {
			v = append(v, l[0])
		}
		for _, pg := range g.Polys {
			v = append(v, pg[0][0])
		}
		return v
	}
	for _, p := range firsts(a) {
		if Locate(b.Polys, p) >= 0 {
			return true
		}
	}
	for _, p := range firsts(b) {
		if Locate(a.Polys, p) >= 0 {
			return true
		}
	}
	return false
}

// interiorPoint is a point strictly inside the ring: the middle of the widest
// span of a horizontal line between two vertex heights.
func interiorPoint(r Ring) (Point, bool) {
	ys := make([]float64, 0, len(r))
	for _, p := range r {
		ys = append(ys, p.Y)
	}
	slices.Sort(ys)
	ys = slices.Compact(ys)
	if len(ys) < 2 {
		return Point{}, false
	}
	mid := len(ys) / 2
	y := (ys[mid-1] + ys[mid]) / 2
	var xs []float64
	for i := 0; i+1 < len(r); i++ {
		a, b := r[i], r[i+1]
		if (a.Y > y) != (b.Y > y) {
			xs = append(xs, a.X+(y-a.Y)*(b.X-a.X)/(b.Y-a.Y))
		}
	}
	slices.Sort(xs)
	best, at := -1.0, 0.0
	for i := 0; i+1 < len(xs); i += 2 {
		if w := xs[i+1] - xs[i]; w > best {
			best, at = w, (xs[i]+xs[i+1])/2
		}
	}
	if best <= 0 {
		return Point{}, false
	}
	return Point{at, y}, true
}

// ringArea is the signed shoelace area (counterclockwise > 0), computed
// relative to the first point for precision.
func ringArea(r Ring) float64 {
	if len(r) < 4 {
		return 0
	}
	o := r[0]
	s := 0.0
	for i := 1; i+2 < len(r); i++ {
		s += orient(o, r[i], r[i+1])
	}
	return s / 2
}

// PlanarArea is the area of the polygons, holes subtracted.
func PlanarArea(pgs []Polygon) float64 {
	s := 0.0
	for _, pg := range pgs {
		s += math.Abs(ringArea(pg[0]))
		for _, h := range pg[1:] {
			s -= math.Abs(ringArea(h))
		}
	}
	return s
}

// Centroid is the planar center of mass of g's highest dimension with a size:
// polygons by area, else lines by length, else the mean of the points (shapely
// centroid). ok is false for an empty geometry.
func Centroid(g Geometry) (Point, bool) {
	var o Point // shift to the first coordinate for precision
	switch {
	case len(g.Polys) > 0:
		o = g.Polys[0][0][0]
	case len(g.Lines) > 0:
		o = g.Lines[0][0]
	case len(g.Points) > 0:
		o = g.Points[0]
	default:
		return Point{}, false
	}
	var sx, sy, sw float64
	for _, pg := range g.Polys {
		for k, r := range pg {
			a := ringArea(r)
			if a == 0 {
				continue
			}
			var cx, cy float64
			for i := 0; i+1 < len(r); i++ {
				x0, y0, x1, y1 := r[i].X-o.X, r[i].Y-o.Y, r[i+1].X-o.X, r[i+1].Y-o.Y
				c := x0*y1 - x1*y0
				cx += (x0 + x1) * c
				cy += (y0 + y1) * c
			}
			cx, cy = cx/(6*a), cy/(6*a)
			w := math.Abs(a)
			if k > 0 {
				w = -w
			}
			sx, sy, sw = sx+w*cx, sy+w*cy, sw+w
		}
	}
	if sw > 0 {
		return Point{o.X + sx/sw, o.Y + sy/sw}, true
	}
	sx, sy, sw = 0, 0, 0
	lines := g.Lines
	for _, pg := range g.Polys {
		for _, r := range pg {
			lines = append(lines, r)
		}
	}
	for _, l := range lines {
		for i := 0; i+1 < len(l); i++ {
			w := PlanarDistance(l[i], l[i+1])
			sx += w * ((l[i].X+l[i+1].X)/2 - o.X)
			sy += w * ((l[i].Y+l[i+1].Y)/2 - o.Y)
			sw += w
		}
	}
	if sw > 0 {
		return Point{o.X + sx/sw, o.Y + sy/sw}, true
	}
	pts := g.Points
	for _, l := range lines {
		pts = append(pts, l...)
	}
	for _, p := range pts {
		sx, sy = sx+p.X-o.X, sy+p.Y-o.Y
	}
	n := float64(len(pts))
	return Point{o.X + sx/n, o.Y + sy/n}, true
}

// Hull is the convex hull of the points (Andrew's monotone chain): a closed
// counterclockwise ring, or 1 point, or the 2 ends of a line when the points
// are all equal or collinear.
func Hull(pts []Point) []Point {
	p := slices.Clone(pts)
	slices.SortFunc(p, func(a, b Point) int {
		if a.X != b.X {
			return cmpF(a.X, b.X)
		}
		return cmpF(a.Y, b.Y)
	})
	p = slices.Compact(p)
	if len(p) < 3 {
		return p
	}
	h := make([]Point, 0, 2*len(p))
	for _, q := range p {
		for len(h) >= 2 && orient(h[len(h)-2], h[len(h)-1], q) <= 0 {
			h = h[:len(h)-1]
		}
		h = append(h, q)
	}
	lower := len(h) + 1
	for i := len(p) - 2; i >= 0; i-- {
		q := p[i]
		for len(h) >= lower && orient(h[len(h)-2], h[len(h)-1], q) <= 0 {
			h = h[:len(h)-1]
		}
		h = append(h, q)
	}
	if len(h) < 4 { // all collinear: the chain went out and back
		return []Point{p[0], p[len(p)-1]}
	}
	return h // closed: the last point is p[0]
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
