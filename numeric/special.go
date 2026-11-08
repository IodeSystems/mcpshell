package numeric

import "math"

// lbeta is ln B(a, b).
func lbeta(a, b float64) float64 {
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	lab, _ := math.Lgamma(a + b)
	return la + lb - lab
}

// GammaPQ returns the regularized incomplete gamma functions P(a, x) and
// Q(a, x) = 1 - P(a, x). The one computed directly (P by the series for
// x < a+1, Q by the continued fraction otherwise) keeps its relative
// accuracy in the tail; the other is 1 minus it.
func GammaPQ(a, x float64) (p, q float64) {
	switch {
	case math.IsNaN(a) || math.IsNaN(x) || a <= 0 || x < 0:
		return math.NaN(), math.NaN()
	case x == 0:
		return 0, 1
	case math.IsInf(x, 1):
		return 1, 0
	}
	lg, _ := math.Lgamma(a)
	lpre := a*math.Log(x) - x - lg
	maxIter := 1000 + int(20*math.Sqrt(a))
	if x < a+1 {
		// Series: P = e^-x x^a / Gamma(a+1) * sum x^n / ((a+1)...(a+n)).
		ap, sum, del := a, 1/a, 1/a
		for range maxIter {
			ap++
			del *= x / ap
			sum += del
			if math.Abs(del) < math.Abs(sum)*1e-16 {
				break
			}
		}
		p = sum * math.Exp(lpre)
		return p, 1 - p
	}
	// Continued fraction for Q (modified Lentz).
	const tiny = 1e-300
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i <= maxIter; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-16 {
			break
		}
	}
	q = math.Exp(lpre) * h
	return 1 - q, q
}

// BetaI returns the regularized incomplete beta function I_x(a, b) and its
// complement 1 - I_x(a, b), each with relative accuracy in its own tail.
func BetaI(x, a, b float64) (i, ic float64) {
	switch {
	case math.IsNaN(x) || a <= 0 || b <= 0:
		return math.NaN(), math.NaN()
	case x <= 0:
		return 0, 1
	case x >= 1:
		return 1, 0
	}
	lfront := a*math.Log(x) + b*math.Log1p(-x) - lbeta(a, b)
	if x < (a+1)/(a+b+2) {
		i = math.Exp(lfront) * betacf(x, a, b) / a
		return i, 1 - i
	}
	ic = math.Exp(lfront) * betacf(1-x, b, a) / b
	return 1 - ic, ic
}

// betacf is the continued fraction for the incomplete beta function
// (modified Lentz).
func betacf(x, a, b float64) float64 {
	const tiny = 1e-300
	qab, qap, qam := a+b, a+1, a-1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	maxIter := 1000 + int(20*math.Sqrt(math.Max(a, b)))
	for m := 1; m <= maxIter; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-16 {
			break
		}
	}
	return h
}

// NormPPF is the standard normal quantile (Wichura, AS 241, PPND16:
// about 1e-16 relative accuracy).
func NormPPF(p float64) float64 {
	switch {
	case math.IsNaN(p) || p < 0 || p > 1:
		return math.NaN()
	case p == 0:
		return math.Inf(-1)
	case p == 1:
		return math.Inf(1)
	}
	q := p - 0.5
	if math.Abs(q) <= 0.425 {
		r := 0.180625 - q*q
		return q * (((((((2509.0809287301226727*r+33430.575583588128105)*r+67265.770927008700853)*r+
			45921.953931549871457)*r+13731.693765509461125)*r+1971.5909503065514427)*r+133.14166789178437745)*r +
			3.387132872796366608) /
			(((((((5226.495278852545925*r+28729.085735721942674)*r+39307.89580009271061)*r+
				21213.794301586595867)*r+5394.1960214247511077)*r+687.1870074920579083)*r+42.313330701600911252)*r + 1)
	}
	r := p
	if q > 0 {
		r = 1 - p
	}
	r = math.Sqrt(-math.Log(r))
	var x float64
	if r <= 5 {
		r -= 1.6
		x = (((((((7.7454501427834140764e-4*r+0.0227238449892691845833)*r+0.24178072517745061177)*r+
			1.27045825245236838258)*r+3.64784832476320460504)*r+5.7694972214606914055)*r+4.6303378461565452959)*r +
			1.42343711074968357734) /
			(((((((1.05075007164441684324e-9*r+5.475938084995344946e-4)*r+0.0151986665636164571966)*r+
				0.14810397642748007459)*r+0.68976733498510000455)*r+1.6763848301838038494)*r+2.05319162663775882187)*r + 1)
	} else {
		r -= 5
		x = (((((((2.01033439929228813265e-7*r+2.71155556874348757815e-5)*r+0.0012426609473880784386)*r+
			0.026532189526576123093)*r+0.29656057182850489123)*r+1.7848265399172913358)*r+5.4637849111641143699)*r +
			6.6579046435011037772) /
			(((((((2.04426310338993978564e-15*r+1.4215117583164458887e-7)*r+1.8463183175100546818e-5)*r+
				7.868691311456132591e-4)*r+0.0148753612908506148525)*r+0.13692988092273580531)*r+0.59983220655588793769)*r + 1)
	}
	if q < 0 {
		x = -x
	}
	return x
}
