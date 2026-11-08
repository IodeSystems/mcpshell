// Package numeric is the float64 numerical methods behind load("num"):
// dense linear algebra, least squares and regression, Levenberg-Marquardt
// curve fitting, Nelder-Mead minimization, Brent root finding,
// interpolation, adaptive quadrature, probability distributions and a
// seeded random number generator.
//
// Everything here is float64. mcpshell numbers are exact elsewhere, but
// these methods are iterative or use irrational functions (exp, sqrt,
// incomplete gamma), so exact arithmetic is not possible for them. The
// package has no runtime dependency: the toolkit converts values in and out
// and checks shapes, and this package reports numeric conditions
// (singular, not symmetric, no sign change) as errors.
package numeric

import (
	"errors"
	"math"
)

// Status is how an iterative method ended.
type Status int

const (
	// Converged: a stopping test of the method was met (see Reason on the
	// result). For a local method this is a local result, not a proof of a
	// global one.
	Converged Status = iota
	// MaxIterations: the iteration or evaluation limit ended the run first.
	MaxIterations
	// Stopped: the context (time limit) ended the run first.
	Stopped
	// Stalled: no trial step improved the objective although no stopping
	// test was met.
	Stalled
)

func (s Status) String() string {
	switch s {
	case Converged:
		return "converged"
	case MaxIterations:
		return "max_iterations"
	case Stalled:
		return "stalled"
	}
	return "stopped"
}

// ErrSingular is returned for a matrix that is singular to working
// precision.
var ErrSingular = errors.New("matrix is singular to working precision")

// ErrNotSymmetric is returned by SymEig for a non-symmetric matrix.
var ErrNotSymmetric = errors.New("matrix is not symmetric")

// eps is the float64 machine epsilon.
const eps = 0x1p-52

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
