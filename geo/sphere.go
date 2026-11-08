package geo

import (
	"math"
	"slices"

	"github.com/iodesystems/mcpshell/routing"
)

// EarthRadius is the mean Earth radius in meters (IUGG R1), the sphere of
// Distance, Bearing, Destination, RadiusBounds and clustering.
const EarthRadius = routing.EarthRadius

const deg = math.Pi / 180

// Distance is the great-circle (haversine) distance in meters between two lon/lat points.
func Distance(a, b Point) float64 { return routing.Haversine(a.Y, a.X, b.Y, b.X) }

// PlanarDistance is the straight-line distance in the plane.
func PlanarDistance(a, b Point) float64 { return routing.Euclidean(a.X, a.Y, b.X, b.Y) }

// Bearing is the initial great-circle bearing from a to b in degrees,
// clockwise from north, in [0, 360).
func Bearing(a, b Point) float64 {
	p1, p2 := a.Y*deg, b.Y*deg
	dl := (b.X - a.X) * deg
	y := math.Sin(dl) * math.Cos(p2)
	x := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dl)
	return norm360(math.Atan2(y, x) / deg)
}

// PlanarBearing is the angle from a to b in degrees, clockwise from +y, in [0, 360).
func PlanarBearing(a, b Point) float64 { return norm360(math.Atan2(b.X-a.X, b.Y-a.Y) / deg) }

// Destination is the point dist meters from a along the great circle that
// starts at bearing degrees.
func Destination(a Point, bearing, dist float64) Point {
	d := dist / EarthRadius
	t := bearing * deg
	p1, l1 := a.Y*deg, a.X*deg
	sp2 := math.Sin(p1)*math.Cos(d) + math.Cos(p1)*math.Sin(d)*math.Cos(t)
	sp2 = math.Max(-1, math.Min(1, sp2))
	l2 := l1 + math.Atan2(math.Sin(t)*math.Sin(d)*math.Cos(p1), math.Cos(d)-math.Sin(p1)*sp2)
	return Point{NormLon(l2 / deg), math.Asin(sp2) / deg}
}

// PlanarDestination is the point dist from a at bearing degrees clockwise from +y.
func PlanarDestination(a Point, bearing, dist float64) Point {
	t := bearing * deg
	return Point{a.X + dist*math.Sin(t), a.Y + dist*math.Cos(t)}
}

func norm360(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	if d >= 360 {
		d = 0
	}
	return d
}

// NormLon maps a longitude to [-180, 180).
func NormLon(l float64) float64 {
	l = math.Mod(l+180, 360)
	if l < 0 {
		l += 360
	}
	return l - 180
}

// PlanarBounds is the box of the points.
func PlanarBounds(pts []Point) Box {
	b := Box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range pts {
		b.MinX, b.MaxX = math.Min(b.MinX, p.X), math.Max(b.MaxX, p.X)
		b.MinY, b.MaxY = math.Min(b.MinY, p.Y), math.Max(b.MaxY, p.Y)
	}
	return b
}

// LonLatBounds is the smallest lon/lat box of the points: its longitude
// range is the shortest arc that holds every longitude, so it crosses the
// antimeridian (MinX > MaxX) when that arc does. A tie keeps the box that
// does not cross.
func LonLatBounds(pts []Point) Box {
	b := PlanarBounds(pts)
	lons := make([]float64, len(pts))
	for i, p := range pts {
		lons[i] = p.X
	}
	slices.Sort(lons)
	n := len(lons)
	gap := lons[0] + 360 - lons[n-1] // the gap the plain box leaves out
	for i := 0; i+1 < n; i++ {
		if g := lons[i+1] - lons[i]; g > gap {
			gap, b.MinX, b.MaxX = g, lons[i+1], lons[i]
		}
	}
	return b
}

// RadiusBounds is the lon/lat box that holds every point within r meters of
// c on the sphere (J. P. Matuschek, "Finding Points Within a Distance of a
// Latitude/Longitude Using Bounding Coordinates"). A circle over a pole
// spans every longitude; one over the antimeridian gives MinX > MaxX.
func RadiusBounds(c Point, r float64) Box {
	d := r / EarthRadius
	lat, lon := c.Y*deg, c.X*deg
	if d >= math.Pi {
		return Box{-180, -90, 180, 90}
	}
	minLat, maxLat := lat-d, lat+d
	if minLat <= -math.Pi/2 || maxLat >= math.Pi/2 {
		return Box{-180, math.Max(minLat, -math.Pi/2) / deg, 180, math.Min(maxLat, math.Pi/2) / deg}
	}
	dl := math.Asin(math.Sin(d) / math.Cos(lat))
	minLon, maxLon := lon-dl, lon+dl
	if minLon < -math.Pi {
		minLon += 2 * math.Pi
	}
	if maxLon > math.Pi {
		maxLon -= 2 * math.Pi
	}
	return Box{minLon / deg, minLat / deg, maxLon / deg, maxLat / deg}
}

// PlanarRadiusBounds is the box of the circle of radius r around c.
func PlanarRadiusBounds(c Point, r float64) Box { return Box{c.X - r, c.Y - r, c.X + r, c.Y + r} }

// Length is the great-circle length in meters of the lines.
func Length(lines [][]Point) float64 {
	s := 0.0
	for _, l := range lines {
		for i := 0; i+1 < len(l); i++ {
			s += Distance(l[i], l[i+1])
		}
	}
	return s
}

// PlanarLength is the length of the lines in the plane.
func PlanarLength(lines [][]Point) float64 {
	s := 0.0
	for _, l := range lines {
		for i := 0; i+1 < len(l); i++ {
			s += PlanarDistance(l[i], l[i+1])
		}
	}
	return s
}

// Unit is the unit vector of a lon/lat point.
func Unit(p Point) [3]float64 {
	la, lo := p.Y*deg, p.X*deg
	return [3]float64{math.Cos(la) * math.Cos(lo), math.Cos(la) * math.Sin(lo), math.Sin(la)}
}

// FromUnit is the lon/lat point of a vector (any length > 0).
func FromUnit(v [3]float64) Point {
	return Point{math.Atan2(v[1], v[0]) / deg, math.Atan2(v[2], math.Hypot(v[0], v[1])) / deg}
}

// SphereMean is the normalized mean of the unit vectors of the points: the
// center of mass projected to the sphere. Unlike the mean of lon and lat it
// holds across the antimeridian and near the poles.
func SphereMean(pts []Point) (Point, error) {
	var s [3]float64
	for _, p := range pts {
		u := Unit(p)
		s[0], s[1], s[2] = s[0]+u[0], s[1]+u[1], s[2]+u[2]
	}
	if math.Sqrt(s[0]*s[0]+s[1]*s[1]+s[2]*s[2]) < 1e-9*float64(len(pts)) {
		return Point{}, ErrSpread
	}
	return FromUnit(s), nil
}

// Circle is a closed counterclockwise ring of steps points at r meters (or
// plane units) from c: the vertices of a buffer around a point.
func Circle(c Point, r float64, steps int, planar bool) Ring {
	ring := make(Ring, 0, steps+1)
	for i := range steps {
		b := -360 * float64(i) / float64(steps) // counterclockwise: bearings decrease
		if planar {
			ring = append(ring, PlanarDestination(c, b, r))
		} else {
			ring = append(ring, Destination(c, b, r))
		}
	}
	return append(ring, ring[0])
}
