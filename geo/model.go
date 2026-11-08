// Package geo is the spatial computation behind the Geo toolkit: distance,
// bearing and boxes on the sphere, planar polygon predicates, geodesic area,
// centroid, convex hull, k-means and DBSCAN. It does not import the runtime;
// toolkit/geo*.go reads and writes the language values.
//
// Coordinates are a Point{X, Y}: X = longitude and Y = latitude in degrees
// for geographic data, or plane x and y. Polygon predicates, centroid and hull
// treat both as planar coordinates (edges are straight in lon/lat, as in
// GeoJSON and PostGIS geometry). Distance, bearing, boxes, length, area and
// clustering of geographic data are on the sphere or the WGS84 ellipsoid.
package geo

import "errors"

// Point is a position: lon/lat in degrees, or plane x/y.
type Point struct{ X, Y float64 }

// Ring is a closed ring: the first and last points are equal.
type Ring []Point

// Polygon is an exterior ring followed by its holes.
type Polygon []Ring

// Geometry is a GeoJSON geometry flattened to its parts: points (Point,
// MultiPoint), lines (LineString, MultiLineString) and polygons (Polygon,
// MultiPolygon). A GeometryCollection or FeatureCollection is the union of
// its members' parts.
type Geometry struct {
	Points []Point
	Lines  [][]Point
	Polys  []Polygon
}

// Empty reports a geometry with no parts.
func (g *Geometry) Empty() bool { return len(g.Points) == 0 && len(g.Lines) == 0 && len(g.Polys) == 0 }

// Add appends the parts of h.
func (g *Geometry) Add(h Geometry) {
	g.Points = append(g.Points, h.Points...)
	g.Lines = append(g.Lines, h.Lines...)
	g.Polys = append(g.Polys, h.Polys...)
}

// Box is an axis-aligned box. For geographic data MinX is the west edge and
// MaxX the east edge; MinX > MaxX when the box crosses the antimeridian
// (RFC 7946 section 5.2).
type Box struct{ MinX, MinY, MaxX, MaxY float64 }

// CrossesAntimeridian reports a geographic box whose west edge is east of its east edge.
func (b Box) CrossesAntimeridian() bool { return b.MinX > b.MaxX }

// Limits.
const (
	// MaxClusterPoints caps the rows of Geo.kmeans and Geo.dbscan.
	MaxClusterPoints = 200000
	// MaxK caps the clusters of k-means.
	MaxK = 10000
	// MaxIterations caps the Lloyd iterations of k-means.
	MaxIterations = 10000
)

// Errors a caller can name.
var (
	ErrPole         = errors.New("the ring goes around a pole")
	ErrAntimeridian = errors.New("an edge crosses the antimeridian")
	ErrSpread       = errors.New("the points are spread evenly around the globe: their mean direction is undefined")
)
