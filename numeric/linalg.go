package numeric

import (
	"math"
	"sort"
)

// Matrices are row-major [][]float64; the caller checks that they are
// rectangular and the shapes agree.

// MatMul returns a*b (a is m x k, b is k x n).
func MatMul(a, b [][]float64) [][]float64 {
	m, k, n := len(a), len(b), len(b[0])
	out := make([][]float64, m)
	for i := range m {
		row := make([]float64, n)
		for t := range k {
			ait := a[i][t]
			if ait == 0 {
				continue
			}
			bt := b[t]
			for j := range n {
				row[j] += ait * bt[j]
			}
		}
		out[i] = row
	}
	return out
}

// Transpose returns a^T.
func Transpose(a [][]float64) [][]float64 {
	m, n := len(a), len(a[0])
	out := make([][]float64, n)
	for j := range n {
		out[j] = make([]float64, m)
		for i := range m {
			out[j][i] = a[i][j]
		}
	}
	return out
}

func clone(a [][]float64) [][]float64 {
	out := make([][]float64, len(a))
	for i, r := range a {
		out[i] = append([]float64(nil), r...)
	}
	return out
}

func maxAbs(a [][]float64) float64 {
	m := 0.0
	for _, r := range a {
		for _, v := range r {
			m = math.Max(m, math.Abs(v))
		}
	}
	return m
}

// LU is an LU factorization with partial pivoting of a square matrix.
type LU struct {
	lu   [][]float64
	piv  []int
	sign float64
	// Singular: a pivot is at most n*eps*max|A|. Solve and Inverse refuse.
	Singular bool
	// PivotRatio is min|u_ii| / max|u_ii|: a cheap sign of ill
	// conditioning (it is not the condition number).
	PivotRatio float64
}

// FactorLU factors the square matrix a (not modified).
func FactorLU(a [][]float64) *LU {
	n := len(a)
	lu := clone(a)
	piv := make([]int, n)
	for i := range piv {
		piv[i] = i
	}
	sign := 1.0
	tol := float64(n) * eps * maxAbs(a)
	f := &LU{lu: lu, piv: piv}
	minP, maxP := math.Inf(1), 0.0
	for k := range n {
		p := k
		for i := k + 1; i < n; i++ {
			if math.Abs(lu[i][k]) > math.Abs(lu[p][k]) {
				p = i
			}
		}
		if p != k {
			lu[p], lu[k] = lu[k], lu[p]
			piv[p], piv[k] = piv[k], piv[p]
			sign = -sign
		}
		d := lu[k][k]
		ad := math.Abs(d)
		minP, maxP = math.Min(minP, ad), math.Max(maxP, ad)
		if ad <= tol {
			f.Singular = true
		}
		if d == 0 {
			continue
		}
		for i := k + 1; i < n; i++ {
			lu[i][k] /= d
			l := lu[i][k]
			if l == 0 {
				continue
			}
			for j := k + 1; j < n; j++ {
				lu[i][j] -= l * lu[k][j]
			}
		}
	}
	f.sign = sign
	if maxP > 0 {
		f.PivotRatio = minP / maxP
	}
	return f
}

// Det is the determinant (the product of the pivots).
func (f *LU) Det() float64 {
	d := f.sign
	for i := range f.lu {
		d *= f.lu[i][i]
	}
	return d
}

// Solve returns x with A x = b for each column of b (n x k).
func (f *LU) Solve(b [][]float64) ([][]float64, error) {
	if f.Singular {
		return nil, ErrSingular
	}
	n := len(f.lu)
	k := len(b[0])
	x := make([][]float64, n)
	for i := range n {
		x[i] = append([]float64(nil), b[f.piv[i]]...)
	}
	for c := range k {
		for i := range n {
			s := x[i][c]
			for j := range i {
				s -= f.lu[i][j] * x[j][c]
			}
			x[i][c] = s
		}
		for i := n - 1; i >= 0; i-- {
			s := x[i][c]
			for j := i + 1; j < n; j++ {
				s -= f.lu[i][j] * x[j][c]
			}
			x[i][c] = s / f.lu[i][i]
		}
	}
	return x, nil
}

// Inverse returns A^-1.
func (f *LU) Inverse() ([][]float64, error) {
	n := len(f.lu)
	id := make([][]float64, n)
	for i := range n {
		id[i] = make([]float64, n)
		id[i][i] = 1
	}
	return f.Solve(id)
}

// QR is a Householder QR factorization of an m x n matrix, m >= n.
type QR struct {
	qr    [][]float64
	rdiag []float64
	m, n  int
}

// FactorQR factors a (not modified); len(a) >= len(a[0]).
func FactorQR(a [][]float64) *QR {
	m, n := len(a), len(a[0])
	qr := clone(a)
	rdiag := make([]float64, n)
	for k := range n {
		nrm := 0.0
		for i := k; i < m; i++ {
			nrm = math.Hypot(nrm, qr[i][k])
		}
		if nrm != 0 {
			if qr[k][k] < 0 {
				nrm = -nrm
			}
			for i := k; i < m; i++ {
				qr[i][k] /= nrm
			}
			qr[k][k]++
			for j := k + 1; j < n; j++ {
				s := 0.0
				for i := k; i < m; i++ {
					s += qr[i][k] * qr[i][j]
				}
				s = -s / qr[k][k]
				for i := k; i < m; i++ {
					qr[i][j] += s * qr[i][k]
				}
			}
		}
		rdiag[k] = -nrm
	}
	return &QR{qr: qr, rdiag: rdiag, m: m, n: n}
}

// Deficient returns the index of the first column whose |R_jj| is at most
// max(m, n)*eps*max|R_ii| (a column that is, to working precision, a
// linear combination of the ones before it), or -1 for full rank.
func (q *QR) Deficient() int {
	mx := 0.0
	for _, d := range q.rdiag {
		mx = math.Max(mx, math.Abs(d))
	}
	tol := float64(max(q.m, q.n)) * eps * mx * 10
	for j, d := range q.rdiag {
		if math.Abs(d) <= tol {
			return j
		}
	}
	return -1
}

// Solve returns the least-squares x minimizing |A x - b| (full rank only).
func (q *QR) Solve(b []float64) ([]float64, error) {
	if q.Deficient() >= 0 {
		return nil, ErrSingular
	}
	m, n := q.m, q.n
	y := append([]float64(nil), b...)
	for k := range n {
		s := 0.0
		for i := k; i < m; i++ {
			s += q.qr[i][k] * y[i]
		}
		s = -s / q.qr[k][k]
		for i := k; i < m; i++ {
			y[i] += s * q.qr[i][k]
		}
	}
	x := make([]float64, n)
	for k := n - 1; k >= 0; k-- {
		s := y[k]
		for j := k + 1; j < n; j++ {
			s -= q.qr[k][j] * x[j]
		}
		x[k] = s / q.rdiag[k]
	}
	return x, nil
}

// RtRInverse returns (R^T R)^-1 = (A^T A)^-1, for the covariance of a
// least-squares fit.
func (q *QR) RtRInverse() [][]float64 {
	n := q.n
	// Rinv is upper triangular: solve R Rinv = I.
	rinv := make([][]float64, n)
	for i := range rinv {
		rinv[i] = make([]float64, n)
	}
	r := func(i, j int) float64 {
		if i == j {
			return q.rdiag[i]
		}
		return q.qr[i][j]
	}
	for c := range n {
		for i := c; i >= 0; i-- {
			s := 0.0
			if i == c {
				s = 1
			}
			for j := i + 1; j <= c; j++ {
				s -= r(i, j) * rinv[j][c]
			}
			rinv[i][c] = s / r(i, i)
		}
	}
	out := make([][]float64, n)
	for i := range n {
		out[i] = make([]float64, n)
		for j := range n {
			s := 0.0
			for k := max(i, j); k < n; k++ {
				s += rinv[i][k] * rinv[j][k]
			}
			out[i][j] = s
		}
	}
	return out
}

// Lstsq solves min |A x - b| by QR. It returns ErrSingular with the first
// dependent column when A does not have full column rank.
func Lstsq(a [][]float64, b []float64) (x []float64, dependent int, err error) {
	q := FactorQR(a)
	if j := q.Deficient(); j >= 0 {
		return nil, j, ErrSingular
	}
	x, err = q.Solve(b)
	return x, -1, err
}

// SymEig returns the eigenvalues (ascending) and unit eigenvectors of the
// symmetric matrix a by the cyclic Jacobi method; vectors[i] belongs to
// values[i], with its largest-magnitude component positive.
func SymEig(a [][]float64) (values []float64, vectors [][]float64, err error) {
	n := len(a)
	scale := maxAbs(a)
	for i := range n {
		for j := i + 1; j < n; j++ {
			if math.Abs(a[i][j]-a[j][i]) > 1e-10*math.Max(scale, 1e-300) {
				return nil, nil, ErrNotSymmetric
			}
		}
	}
	m := clone(a)
	v := make([][]float64, n)
	for i := range v {
		v[i] = make([]float64, n)
		v[i][i] = 1
	}
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := range n {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off <= (eps*scale)*(eps*scale) || off == 0 {
			break
		}
		for p := range n {
			for q := p + 1; q < n; q++ {
				apq := m[p][q]
				if apq == 0 {
					continue
				}
				theta := (m[q][q] - m[p][p]) / (2 * apq)
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := range n {
					mkp, mkq := m[k][p], m[k][q]
					m[k][p] = c*mkp - s*mkq
					m[k][q] = s*mkp + c*mkq
				}
				for k := range n {
					mpk, mqk := m[p][k], m[q][k]
					m[p][k] = c*mpk - s*mqk
					m[q][k] = s*mpk + c*mqk
				}
				for k := range n {
					vkp, vkq := v[k][p], v[k][q]
					v[k][p] = c*vkp - s*vkq
					v[k][q] = s*vkp + c*vkq
				}
			}
		}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool { return m[idx[x]][idx[x]] < m[idx[y]][idx[y]] })
	values = make([]float64, n)
	vectors = make([][]float64, n)
	for r, i := range idx {
		values[r] = m[i][i]
		vec := make([]float64, n)
		big := 0
		for k := range n {
			vec[k] = v[k][i]
			if math.Abs(vec[k]) > math.Abs(vec[big])+1e-12 {
				big = k
			}
		}
		if vec[big] < 0 {
			for k := range vec {
				vec[k] = -vec[k]
			}
		}
		vectors[r] = vec
	}
	return values, vectors, nil
}
