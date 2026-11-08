package numeric

import "math"

// Peak shapes are parameterized by center, height (the value at the
// center) and width (full width at half maximum), so the guesses read off
// a plot. Pseudo-Voigt adds eta in [0, 1], the Lorentzian fraction.

// PeakShape is a named line shape.
type PeakShape struct {
	Name string
	// NParams is 3 (center, height, width) or 4 (+ eta).
	NParams int
	At      func(x float64, p []float64) float64
	// Area is the integral over the real line.
	Area func(p []float64) float64
}

const ln2x4 = 4 * math.Ln2

var gaussArea = math.Sqrt(math.Pi / ln2x4)

func gaussAt(x float64, p []float64) float64 {
	d := (x - p[0]) / p[2]
	return p[1] * math.Exp(-ln2x4*d*d)
}

func lorentzAt(x float64, p []float64) float64 {
	d := (x - p[0]) / p[2]
	return p[1] / (1 + 4*d*d)
}

// PeakShapes lists the supported shapes.
var PeakShapes = []PeakShape{
	{"gaussian", 3, gaussAt, func(p []float64) float64 { return p[1] * p[2] * gaussArea }},
	{"lorentzian", 3, lorentzAt, func(p []float64) float64 { return p[1] * p[2] * math.Pi / 2 }},
	{"pseudovoigt", 4,
		func(x float64, p []float64) float64 { return p[3]*lorentzAt(x, p) + (1-p[3])*gaussAt(x, p) },
		func(p []float64) float64 { return p[1] * p[2] * (p[3]*math.Pi/2 + (1-p[3])*gaussArea) }},
}

// FindPeakShape returns the named shape.
func FindPeakShape(name string) (PeakShape, bool) {
	for _, s := range PeakShapes {
		if s.Name == name {
			return s, true
		}
	}
	return PeakShape{}, false
}
