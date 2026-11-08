package numeric

import (
	"math"
	"math/bits"
	"math/rand/v2"
)

// RNG is a seeded generator: PCG-DXSM (math/rand/v2's PCG, a fixed
// algorithm) with the conversions to floats, integers and normals written
// here, so a seed gives the same stream in every mcpshell version.
type RNG struct {
	src *rand.PCG
	// spare normal from the polar method
	spare    float64
	hasSpare bool
}

// NewRNG seeds a generator.
func NewRNG(seed uint64) *RNG {
	return &RNG{src: rand.NewPCG(seed, 0x9e3779b97f4a7c15^seed)}
}

// Float64 is uniform in [0, 1) with 53 random bits.
func (r *RNG) Float64() float64 { return float64(r.src.Uint64()>>11) * 0x1p-53 }

// open is uniform in (0, 1).
func (r *RNG) open() float64 {
	for {
		if u := r.Float64(); u > 0 {
			return u
		}
	}
}

// Below returns a uniform integer in [0, n), n > 0 (Lemire's method).
func (r *RNG) Below(n uint64) uint64 {
	hi, lo := bits.Mul64(r.src.Uint64(), n)
	if lo < n {
		thresh := -n % n
		for lo < thresh {
			hi, lo = bits.Mul64(r.src.Uint64(), n)
		}
	}
	return hi
}

// Normal is a standard normal draw (Marsaglia polar method).
func (r *RNG) Normal() float64 {
	if r.hasSpare {
		r.hasSpare = false
		return r.spare
	}
	for {
		u := 2*r.Float64() - 1
		v := 2*r.Float64() - 1
		s := u*u + v*v
		if s > 0 && s < 1 {
			m := math.Sqrt(-2 * math.Log(s) / s)
			r.spare, r.hasSpare = v*m, true
			return u * m
		}
	}
}

// Draw samples d by inversion: PPF of a uniform in (0, 1).
func (r *RNG) Draw(d Dist) float64 { return d.PPF(r.open()) }

// Perm returns a random permutation of 0..n-1 (Fisher-Yates).
func (r *RNG) Perm(n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := int(r.Below(uint64(i + 1)))
		p[i], p[j] = p[j], p[i]
	}
	return p
}

// Sample returns k distinct indexes of 0..n-1 in random order (a partial
// Fisher-Yates shuffle).
func (r *RNG) Sample(n, k int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	for i := range k {
		j := i + int(r.Below(uint64(n-i)))
		p[i], p[j] = p[j], p[i]
	}
	return p[:k]
}
