package runtime

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// numFormat is the en-US subset of Intl.NumberFormat. It formats from the
// exact decimal value (big.Rat), never through float64, and rounds a half
// away from zero as Intl does. An option it does not implement is an error
// that lists what is supported: an ignored option would change the text
// without a sign.
type numFormat struct {
	style           string // decimal, currency, percent
	currency        string
	currencyDisplay string // symbol, code
	minFD, maxFD    int
	minID           int
	minSD, maxSD    int // 0: unset
	grouping        bool
	compact         bool
	digitsGiven     bool
	signDisplay     string
}

const supportedNumberOptions = "style, currency, currencyDisplay, minimumFractionDigits, maximumFractionDigits, " +
	"minimumIntegerDigits, minimumSignificantDigits, maximumSignificantDigits, useGrouping, notation, compactDisplay, signDisplay"

var currencySymbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "CN¥", "INR": "₹", "CAD": "CA$", "AUD": "A$",
	"KRW": "₩", "MXN": "MX$", "BRL": "R$", "NZD": "NZ$", "HKD": "HK$", "ILS": "₪", "VND": "₫", "TWD": "NT$", "PHP": "₱",
}

func currencyDigits(code string) int {
	switch code {
	case "JPY", "KRW", "VND", "CLP", "ISK", "UGX", "XAF", "XOF", "PYG", "RWF":
		return 0
	case "BHD", "KWD", "JOD", "OMR", "TND", "IQD", "LYD":
		return 3
	}
	return 2
}

func optionInt(fn string, o *ObjectVal, key string, lo, hi int) (int, bool) {
	v, ok := o.Get(key)
	if !ok || isNullValue(v) {
		return 0, false
	}
	n, isNum := v.(*NumberVal)
	if !isNum || n.V != math.Trunc(n.V) || n.V < float64(lo) || n.V > float64(hi) {
		panic(Runtime(fmt.Sprintf("%s: option %s must be an integer from %d to %d, got %s", fn, key, lo, hi, v.Inspect())))
	}
	return int(n.V), true
}

func parseNumberFormat(fn string, locale, options Value) *numFormat {
	checkLocale(fn, locale)
	o := optionsObject(fn, options)
	checkOptionKeys(fn, o, supportedNumberOptions)
	f := &numFormat{style: "decimal", currencyDisplay: "symbol", grouping: true, signDisplay: "auto"}
	if s := optionString(fn, o, "style", "decimal", "currency", "percent", "unit"); s != "" {
		if s == "unit" {
			panic(Runtime(fn + ": style \"unit\" is not supported (supported: decimal, currency, percent); write the unit after the number: n.toLocaleString() + \" km\""))
		}
		f.style = s
	}
	if v, ok := o.Get("currency"); ok && !isNullValue(v) {
		s, isStr := v.(*StringVal)
		if !isStr || len(s.V) != 3 || strings.ToUpper(s.V) != s.V && strings.ToLower(s.V) != s.V {
			panic(Runtime(fn + ": option currency must be a 3-letter code such as \"USD\", got " + v.Inspect()))
		}
		f.currency = strings.ToUpper(s.V)
	}
	if f.style == "currency" && f.currency == "" {
		panic(Runtime(fn + ": style \"currency\" needs a currency code: {style: \"currency\", currency: \"USD\"}"))
	}
	if f.style != "currency" && f.currency != "" {
		panic(Runtime(fn + ": option currency needs style \"currency\""))
	}
	if s := optionString(fn, o, "currencyDisplay", "symbol", "code", "name", "narrowSymbol"); s != "" {
		if s == "name" || s == "narrowSymbol" {
			panic(Runtime(fn + ": currencyDisplay \"" + s + "\" is not supported (supported: symbol, code)"))
		}
		f.currencyDisplay = s
	}
	switch optionString(fn, o, "notation", "standard", "compact", "scientific", "engineering") {
	case "compact":
		f.compact = true
	case "scientific", "engineering":
		panic(Runtime(fn + ": notation scientific and engineering are not supported (supported: standard, compact); use n.toExponential(d)"))
	}
	if s := optionString(fn, o, "compactDisplay", "short", "long"); s == "long" {
		panic(Runtime(fn + ": compactDisplay \"long\" is not supported (supported: short: K, M, B, T)"))
	}
	f.signDisplay = "auto"
	if s := optionString(fn, o, "signDisplay", "auto", "always", "never", "exceptZero", "negative"); s != "" {
		f.signDisplay = s
	}
	if v, ok := o.Get("useGrouping"); ok && !isNullValue(v) {
		switch g := v.(type) {
		case *BoolVal:
			f.grouping = g.V
		case *StringVal:
			if g.V != "auto" && g.V != "always" {
				panic(Runtime(fn + ": option useGrouping must be true, false, \"auto\" or \"always\", got " + v.Inspect()))
			}
		default:
			panic(Runtime(fn + ": option useGrouping must be true, false, \"auto\" or \"always\", got " + v.Inspect()))
		}
	}
	f.minID = 1
	if n, ok := optionInt(fn, o, "minimumIntegerDigits", 1, 21); ok {
		f.minID = n
	}
	defMin, defMax := 0, 3
	switch f.style {
	case "currency":
		defMin = currencyDigits(f.currency)
		defMax = defMin
	case "percent":
		defMin, defMax = 0, 0
	}
	mnfd, hasMn := optionInt(fn, o, "minimumFractionDigits", 0, 100)
	mxfd, hasMx := optionInt(fn, o, "maximumFractionDigits", 0, 100)
	mnsd, hasMnSD := optionInt(fn, o, "minimumSignificantDigits", 1, 21)
	mxsd, hasMxSD := optionInt(fn, o, "maximumSignificantDigits", 1, 21)
	switch {
	case hasMnSD || hasMxSD:
		f.minSD, f.maxSD = 1, 21
		if hasMnSD {
			f.minSD = mnsd
		}
		if hasMxSD {
			f.maxSD = mxsd
		}
		if f.minSD > f.maxSD {
			panic(Runtime(fn + ": minimumSignificantDigits is greater than maximumSignificantDigits"))
		}
		f.digitsGiven = true
	case hasMn || hasMx:
		switch {
		case !hasMn:
			mnfd = min(defMin, mxfd)
		case !hasMx:
			mxfd = max(defMax, mnfd)
		case mnfd > mxfd:
			panic(Runtime(fn + ": minimumFractionDigits is greater than maximumFractionDigits"))
		}
		f.minFD, f.maxFD = mnfd, mxfd
		f.digitsGiven = true
	default:
		f.minFD, f.maxFD = defMin, defMax
	}
	return f
}

// formatRat is the exact value of n, or ok false for NaN and the infinities.
func ratOf(n *NumberVal) (*big.Rat, bool) {
	if r := n.asRat(); r != nil {
		return r, true
	}
	if math.IsNaN(n.V) || math.IsInf(n.V, 0) {
		return nil, false
	}
	// A float-only value (sqrt, sin, …) is its shortest decimal, as ICU reads a double.
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(n.V, 'g', -1, 64))
	if !ok {
		return nil, false
	}
	return r, true
}

var bigTen = big.NewInt(10)

func pow10(n int) *big.Int { return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil) }

// roundHalfUp returns round(r * 10^f) for r >= 0, a half rounding up. f may be negative.
func roundHalfUp(r *big.Rat, f int) *big.Int {
	x := new(big.Rat).Set(r)
	if f >= 0 {
		x.Mul(x, new(big.Rat).SetInt(pow10(f)))
	} else {
		x.Quo(x, new(big.Rat).SetInt(pow10(-f)))
	}
	x.Add(x, big.NewRat(1, 2))
	return new(big.Int).Div(x.Num(), x.Denom()) // floor for a positive value
}

// digitsFixed renders r (>= 0) with at most maxFD and at least minFD fraction digits.
func digitsFixed(r *big.Rat, minFD, maxFD int) (string, string) {
	q := roundHalfUp(r, maxFD).String()
	if len(q) <= maxFD {
		q = strings.Repeat("0", maxFD-len(q)+1) + q
	}
	ip, fp := q[:len(q)-maxFD], q[len(q)-maxFD:]
	for len(fp) > minFD && strings.HasSuffix(fp, "0") {
		fp = fp[:len(fp)-1]
	}
	return ip, fp
}

// magnitude is floor(log10(r)) for r > 0.
func magnitude(r *big.Rat) int {
	e := 0
	one := big.NewRat(1, 1)
	x := new(big.Rat).Set(r)
	ten := big.NewRat(10, 1)
	for x.Cmp(ten) >= 0 {
		x.Quo(x, ten)
		e++
	}
	for x.Cmp(one) < 0 {
		x.Mul(x, ten)
		e--
	}
	return e
}

// digitsSignificant renders r (>= 0) with between minSD and maxSD significant digits.
func digitsSignificant(r *big.Rat, minSD, maxSD int) (string, string) {
	if r.Sign() == 0 {
		return "0", strings.Repeat("0", minSD-1)
	}
	e := magnitude(r)
	f := maxSD - 1 - e
	q := roundHalfUp(r, f).String()
	var ip, fp string
	switch {
	case f <= 0:
		ip = q + strings.Repeat("0", -f)
	case len(q) > f:
		ip, fp = q[:len(q)-f], q[len(q)-f:]
	default:
		ip, fp = "0", strings.Repeat("0", f-len(q))+q
	}
	sig := func() int {
		all := strings.TrimLeft(ip+fp, "0")
		return len(all)
	}
	for len(fp) > 0 && strings.HasSuffix(fp, "0") && sig() > minSD {
		fp = fp[:len(fp)-1]
	}
	for sig() < minSD {
		fp += "0"
	}
	return ip, fp
}

func groupDigits(s string) string {
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

var compactSuffix = []string{"", "K", "M", "B", "T"}

// format renders n; the number is exact up to the digits asked for.
func (f *numFormat) format(n *NumberVal) string {
	r, finite := ratOf(n)
	neg := false
	var ip, fp, suffix string
	if !finite {
		neg = n.V < 0
		ip = "NaN"
		if math.IsInf(n.V, 0) {
			ip = "∞"
		}
	} else {
		neg = r.Sign() < 0 || (r.Sign() == 0 && math.Signbit(n.V) && n.kind == kFloat)
		r = new(big.Rat).Abs(r)
		if f.style == "percent" {
			r.Mul(r, big.NewRat(100, 1))
		}
		switch {
		case f.compact && !f.digitsGiven:
			ip, fp, suffix = compactDigits(r)
		case f.compact:
			k := 0
			for k < 4 && r.Cmp(new(big.Rat).SetInt(pow10(3*(k+1)))) >= 0 {
				k++
			}
			s := new(big.Rat).Quo(r, new(big.Rat).SetInt(pow10(3*k)))
			ip, fp = f.digits(s)
			suffix = compactSuffix[k]
		default:
			ip, fp = f.digits(r)
		}
		if len(ip) < f.minID {
			ip = strings.Repeat("0", f.minID-len(ip)) + ip
		}
		if f.grouping {
			ip = groupDigits(ip)
		}
	}
	body := ip
	if fp != "" {
		body += "." + fp
	}
	body += suffix
	zero := finite && strings.Trim(ip+fp, "0,") == ""
	sign := ""
	switch f.signDisplay {
	case "auto":
		if neg {
			sign = "-"
		}
	case "always":
		sign = "+"
		if neg {
			sign = "-"
		}
	case "exceptZero":
		if !zero && finite || !finite && ip != "NaN" {
			sign = "+"
			if neg {
				sign = "-"
			}
		}
	case "negative":
		if neg && !zero {
			sign = "-"
		}
	}
	switch f.style {
	case "percent":
		return sign + body + "%"
	case "currency":
		if f.currencyDisplay == "symbol" {
			if sym, ok := currencySymbols[f.currency]; ok {
				return sign + sym + body
			}
		}
		return sign + f.currency + " " + body
	}
	return sign + body
}

func (f *numFormat) digits(r *big.Rat) (string, string) {
	if f.maxSD > 0 {
		return digitsSignificant(r, f.minSD, f.maxSD)
	}
	return digitsFixed(r, f.minFD, f.maxFD)
}

// compactDigits is the default compact rounding: a value of two integer
// digits or more rounds to an integer, a smaller one to two significant
// digits (1234 → 1.2K, 12345 → 12K), and a result that reaches 1000 moves to
// the next unit (999999 → 1M).
func compactDigits(r *big.Rat) (ip, fp, suffix string) {
	k := 0
	for k < 4 && r.Cmp(new(big.Rat).SetInt(pow10(3*(k+1)))) >= 0 {
		k++
	}
	for {
		s := new(big.Rat).Quo(r, new(big.Rat).SetInt(pow10(3*k)))
		if s.Cmp(big.NewRat(10, 1)) >= 0 {
			ip, fp = digitsFixed(s, 0, 0)
		} else {
			ip, fp = digitsSignificant(s, 1, 2)
		}
		if k < 4 && len(ip) > 3 {
			k++
			continue
		}
		return ip, fp, compactSuffix[k]
	}
}

func numberArg(fn string, v Value) *NumberVal {
	switch x := v.(type) {
	case *NumberVal:
		return x
	case *StringVal:
		if n, ok := ParseNumber(strings.TrimSpace(x.V)); ok {
			return n
		}
		return Num(math.NaN())
	}
	panic(TypeMismatch(fn, "number", v, ""))
}

// FormatNumberLocale is n.toLocaleString(locale, options).
func FormatNumberLocale(n *NumberVal, locale, options Value) string {
	return parseNumberFormat("toLocaleString", locale, options).format(n)
}

func numberToLocaleString(n *NumberVal) *FuncVal {
	return fnVal("toLocaleString", []string{"locale", "options"}, func(args []Value) Value {
		tooMany("toLocaleString", args, 2)
		return Str(FormatNumberLocale(n, arg0(args), argAt(args, 1)))
	})
}

// NewNumberFormat is Intl.NumberFormat(locale?, options?): an object with format(n).
func NewNumberFormat() *FuncVal {
	return fnVal("Intl.NumberFormat", []string{"locale", "options"}, func(args []Value) Value {
		tooMany("Intl.NumberFormat", args, 2)
		f := parseNumberFormat("Intl.NumberFormat", arg0(args), argAt(args, 1))
		o := NewObject()
		o.Set("format", fnVal("format", []string{"number"}, func(a []Value) Value {
			tooMany("format", a, 1)
			return Str(f.format(numberArg("format", arg0(a))))
		}))
		return o
	})
}
