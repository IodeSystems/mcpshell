package routing

import (
	"context"
	"math"
	"slices"
)

// EarthRadius is the mean Earth radius in meters (IUGG R1).
const EarthRadius = 6371008.8

// Haversine is the great-circle distance in meters between two points given
// in degrees.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const rad = math.Pi / 180
	p1, p2 := lat1*rad, lat2*rad
	dp, dl := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * EarthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Euclidean is the straight-line distance in the plane.
func Euclidean(x1, y1, x2, y2 float64) float64 { return math.Hypot(x2-x1, y2-y1) }

// Manhattan is |dx| + |dy|.
func Manhattan(x1, y1, x2, y2 float64) float64 { return math.Abs(x2-x1) + math.Abs(y2-y1) }

// Closure replaces each Inf entry (i, j) by the shortest path cost from i to
// j over the finite arcs. Rows without an Inf are not searched. It returns
// the first pair with no path, or (-1, -1).
func Closure(ctx context.Context, m *Matrix) (int, int, error) {
	orig := &Matrix{N: m.N, C: slices.Clone(m.C)}
	for i := 0; i < m.N; i++ {
		row := m.C[i*m.N : (i+1)*m.N]
		if !slices.Contains(row, Inf) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return -1, -1, err
		}
		dist := DenseShortest(orig, i, false)
		for j := range row {
			if math.IsInf(row[j], 1) {
				if math.IsInf(dist[j], 1) {
					return i, j, nil
				}
				row[j] = dist[j]
			}
		}
	}
	return -1, -1, nil
}

// DenseShortest is O(n^2) Dijkstra on a dense matrix: the shortest path cost
// from src to every node, or from every node to src when reverse is set.
// Inf entries are missing arcs.
func DenseShortest(m *Matrix, src int, reverse bool) []float64 {
	n := m.N
	d := make([]float64, n)
	done := make([]bool, n)
	for i := range d {
		d[i] = Inf
	}
	d[src] = 0
	for range n {
		u := -1
		for v := 0; v < n; v++ {
			if !done[v] && (u < 0 || d[v] < d[u]) {
				u = v
			}
		}
		if math.IsInf(d[u], 1) {
			break
		}
		done[u] = true
		for v := 0; v < n; v++ {
			w := m.C[u*n+v]
			if reverse {
				w = m.C[v*n+u]
			}
			if c := d[u] + w; c < d[v] {
				d[v] = c
			}
		}
	}
	return d
}
