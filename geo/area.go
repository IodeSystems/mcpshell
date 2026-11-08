package geo

import "math"

// Geodesic area on the WGS84 ellipsoid, by an equal-area map to the authalic
// sphere: each latitude becomes its authalic latitude, and the ring's area is
// the spherical excess of its great-circle polygon on the sphere of radius
// Rq (Snyder, "Map Projections: A Working Manual", USGS PP 1395, eq. 3-11,
// 3-12, 3-18). The map preserves area exactly; the only error is the edge
// shape: a great circle on that sphere, not an ellipsoid geodesic. Against
// pyproj.Geod(ellps="WGS84") (geo_test.go): 5e-11 relative for a city block,
// 2e-7 for a 1 degree square, 5e-6 to 4e-5 for polygons 5 to 20 degrees
// across with long edges, 5e-4 for one 60 degrees wide. Dense vertices
// (short edges) shrink the error.

const (
	wgsA  = 6378137.0
	wgsF  = 1 / 298.257223563
	wgsE2 = wgsF * (2 - wgsF)
)

var (
	wgsE    = math.Sqrt(wgsE2)
	authQp  = authQ(1)
	authRq2 = wgsA * wgsA * authQp / 2 // Rq^2
)

func authQ(s float64) float64 {
	return (1 - wgsE2) * (s/(1-wgsE2*s*s) - 1/(2*wgsE)*math.Log((1-wgsE*s)/(1+wgsE*s)))
}

// authalic is the authalic latitude in radians of a geodetic latitude in degrees.
func authalic(lat float64) float64 {
	return math.Asin(math.Max(-1, math.Min(1, authQ(math.Sin(lat*deg))/authQp)))
}

// ringExcess is the signed spherical excess (unit sphere) of a closed lon/lat
// ring with great-circle edges: the sum over edges of the signed area between
// the edge and the equator, 2 atan(tan(dl/2) (t1 + t2) / (1 + t1 t2)) with
// t = tan(beta/2). An edge must span at most 180 degrees of longitude.
func ringExcess(r Ring) (float64, error) {
	s := 0.0
	for i := 0; i+1 < len(r); i++ {
		dl := r[i+1].X - r[i].X
		if math.Abs(dl) > 180 {
			return 0, ErrAntimeridian
		}
		t1, t2 := math.Tan(authalic(r[i].Y)/2), math.Tan(authalic(r[i+1].Y)/2)
		s += 2 * math.Atan2(math.Tan(dl*deg/2)*(t1+t2), 1+t1*t2)
	}
	return s, nil
}

// GeodesicArea is the area in square meters of lon/lat polygons on the WGS84
// ellipsoid, holes subtracted.
func GeodesicArea(pgs []Polygon) (float64, error) {
	total := 0.0
	for _, pg := range pgs {
		for k, r := range pg {
			e, err := ringExcess(r)
			if err != nil {
				return 0, err
			}
			a := math.Abs(e) * authRq2
			if k > 0 {
				a = -a
			}
			total += a
		}
	}
	return total, nil
}
