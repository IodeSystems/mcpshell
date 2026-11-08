package geo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
)

// Clustering works on vectors: plane points as [x, y], lon/lat points as unit
// vectors [x, y, z] on the sphere, so distances and means hold across the
// antimeridian and at the poles. On unit vectors the straight (chord)
// distance orders points as the great-circle distance does.

func sqDist(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return s
}

// KMeansResult is the outcome of Lloyd's algorithm: a local optimum, not a
// proven best.
type KMeansResult struct {
	Labels     []int       // cluster of each point
	Centers    [][]float64 // the mean of each cluster (renormalized on the sphere)
	Iterations int         // update steps run
	Converged  bool        // an update left every label unchanged
	Stopped    bool        // the context ended first
}

// KMeansPlusPlus picks k initial centers by k-means++ (Arthur and Vassilvitskii
// 2007): the first uniformly, each next one with probability proportional to
// the squared distance to the nearest center picked. rng makes it repeatable.
func KMeansPlusPlus(X [][]float64, k int, rng *rand.Rand) ([][]float64, error) {
	n := len(X)
	centers := [][]float64{clone(X[rng.IntN(n)])}
	d := make([]float64, n)
	for i := range X {
		d[i] = sqDist(X[i], centers[0])
	}
	for len(centers) < k {
		sum := 0.0
		for _, v := range d {
			sum += v
		}
		if sum == 0 {
			return nil, fmt.Errorf("only %d distinct points, fewer than k = %d", len(centers), k)
		}
		r := rng.Float64() * sum
		pick := n - 1
		for i, v := range d {
			if r < v {
				pick = i
				break
			}
			r -= v
		}
		for d[pick] == 0 { // rounding landed on a picked point
			pick = (pick + n - 1) % n
		}
		c := clone(X[pick])
		centers = append(centers, c)
		for i := range X {
			d[i] = math.Min(d[i], sqDist(X[i], c))
		}
	}
	return centers, nil
}

func clone(v []float64) []float64 { return append([]float64(nil), v...) }

// KMeans runs Lloyd's algorithm from the given centers until no label
// changes or maxIter updates. With sphere set, each center is the mean
// renormalized to unit length (spherical k-means). A cluster left empty takes
// the point farthest from its center, as scikit-learn does.
func KMeans(ctx context.Context, X [][]float64, centers [][]float64, maxIter int, sphere bool) KMeansResult {
	n := len(X)
	dim := len(X[0])
	labels := make([]int, n)
	assign := func() bool {
		changed := false
		for i, x := range X {
			best, bd := 0, math.Inf(1)
			for c, ctr := range centers {
				if d := sqDist(x, ctr); d < bd {
					best, bd = c, d
				}
			}
			if labels[i] != best {
				labels[i], changed = best, true
			}
		}
		return changed
	}
	for i := range labels {
		labels[i] = -1
	}
	assign()
	res := KMeansResult{Labels: labels, Centers: centers}
	for res.Iterations < maxIter {
		if ctx.Err() != nil {
			res.Stopped = true
			return res
		}
		res.Iterations++
		update(X, labels, centers, dim, sphere)
		if !assign() {
			res.Converged = true
			break
		}
	}
	return res
}

func update(X [][]float64, labels []int, centers [][]float64, dim int, sphere bool) {
	k := len(centers)
	sums := make([][]float64, k)
	counts := make([]int, k)
	for c := range sums {
		sums[c] = make([]float64, dim)
	}
	for i, x := range X {
		c := labels[i]
		counts[c]++
		for j := range x {
			sums[c][j] += x[j]
		}
	}
	for c := range k {
		if counts[c] > 0 {
			continue
		}
		// Empty: move the point farthest from its center (in a cluster of more than one) here.
		far, fd := -1, -1.0
		for i, x := range X {
			if counts[labels[i]] > 1 {
				if d := sqDist(x, centers[labels[i]]); d > fd {
					far, fd = i, d
				}
			}
		}
		if far < 0 {
			continue
		}
		old := labels[far]
		counts[old]--
		for j := range X[far] {
			sums[old][j] -= X[far][j]
			sums[c][j] = X[far][j]
		}
		labels[far], counts[c] = c, 1
	}
	for c := range k {
		if counts[c] == 0 {
			continue
		}
		norm := 0.0
		for j := range dim {
			centers[c][j] = sums[c][j] / float64(counts[c])
			norm += centers[c][j] * centers[c][j]
		}
		if sphere && norm > 0 {
			norm = math.Sqrt(norm)
			for j := range dim {
				centers[c][j] /= norm
			}
		}
	}
}

// Noise is the DBSCAN label of a point in no cluster.
const Noise = -1

// ErrEpsSmall is returned when eps is too small for the coordinates' grid.
var ErrEpsSmall = errors.New("eps is too small for these coordinates")

// DBSCAN clusters X (Ester et al. 1996): a point with at least minPts points
// (itself included) within eps is a core point; clusters are the core points
// linked by eps, plus the non-core points within eps of one (border points,
// which join the first cluster that reaches them, in input order). Others are
// Noise. Labels match scikit-learn's DBSCAN for the same eps and min_samples.
// Neighbors come from a grid of eps-sized cells.
func DBSCAN(ctx context.Context, X [][]float64, eps float64, minPts int) (labels []int, core []bool, err error) {
	n := len(X)
	type key [3]int64
	cells := map[key][]int{}
	cellOf := func(x []float64) (key, bool) {
		var k key
		for j, v := range x {
			c := math.Floor(v / eps)
			if math.Abs(c) > 1e15 {
				return k, false
			}
			k[j] = int64(c)
		}
		return k, true
	}
	keys := make([]key, n)
	for i, x := range X {
		k, ok := cellOf(x)
		if !ok {
			return nil, nil, ErrEpsSmall
		}
		keys[i] = k
		cells[k] = append(cells[k], i)
	}
	dim := len(X[0])
	eps2 := eps * eps
	// neighbors calls f for every point within eps of i (i included), in input order per cell.
	neighbors := func(i int, f func(j int) bool) {
		var off key
		var walk func(d int) bool
		walk = func(d int) bool {
			if d == dim {
				var k key
				for j := range dim {
					k[j] = keys[i][j] + off[j]
				}
				for _, j := range cells[k] {
					if sqDist(X[i], X[j]) <= eps2 && !f(j) {
						return false
					}
				}
				return true
			}
			for o := int64(-1); o <= 1; o++ {
				off[d] = o
				if !walk(d + 1) {
					return false
				}
			}
			return true
		}
		walk(0)
	}
	core = make([]bool, n)
	for i := range X {
		if i&1023 == 0 && ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		cnt := 0
		neighbors(i, func(int) bool { cnt++; return cnt < minPts })
		core[i] = cnt >= minPts
	}
	labels = make([]int, n)
	for i := range labels {
		labels[i] = Noise
	}
	next := 0
	var stack []int
	for i := range X {
		if labels[i] != Noise || !core[i] {
			continue
		}
		labels[i] = next
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			neighbors(p, func(j int) bool {
				if labels[j] == Noise {
					labels[j] = next
					if core[j] {
						stack = append(stack, j)
					}
				}
				return true
			})
		}
		next++
	}
	return labels, core, nil
}
