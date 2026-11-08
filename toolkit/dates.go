package toolkit

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	. "github.com/iodesystems/mcpshell/runtime"
)

// InstallDates installs Date, Intl and Dates. "Local" time is loc (UTC unless
// the operator chose a zone: SetTimeZone). Date.now() reads the host clock.
func InstallDates(sh *Shell, loc *time.Location) *Shell {
	if loc == nil {
		loc = time.UTC
	}
	sh.SetBuiltin("Date", NewDateNamespace(loc, nil))
	intl := NewObject()
	intl.Set("NumberFormat", NewNumberFormat())
	intl.Set("DateTimeFormat", NewDateTimeFormat(loc))
	sh.SetBuiltin("Intl", intl)
	sh.SetBuiltin("Dates", newDatesNamespace(loc))
	sh.RegisterGuide("dates", datesGuide)
	return sh
}

// SetTimeZone makes loc the local zone of Date in sh. An operator choice:
// the sandbox has no ambient TZ, so the default is UTC.
func SetTimeZone(sh *Shell, loc *time.Location) *Shell { return InstallDates(sh, loc) }

// --- argument handling ----------------------------------------------------------

type dateShape int

const (
	shapeDate dateShape = iota // a Date (or a number of ms): results are Dates
	shapeDay                   // "2026-11-07"
	shapeSQL                   // "2026-11-07 12:00:00"
	shapeISO                   // "2026-11-07T12:00:00.000Z"
)

var (
	dayOnlyRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	sqlDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}(:\d{2})?$`)
)

// asTime reads a Date, an ISO or SQL date string, or a number of ms. Strings
// without an offset are UTC here: the Dates helpers work on the UTC calendar.
func asTime(fn string, v Value) (time.Time, dateShape) {
	switch x := v.(type) {
	case *DateVal:
		if !x.Valid() {
			panic(Runtime(fn + ": Invalid Date"))
		}
		return x.UTC(), shapeDate
	case *NumberVal:
		if math.IsNaN(x.V) || math.IsInf(x.V, 0) {
			panic(Runtime(fn + ": " + x.Display() + " is not a time value"))
		}
		return time.UnixMilli(int64(x.V)).UTC(), shapeDate
	case *StringVal:
		ms, ok := ParseDateString(x.V, time.UTC)
		if !ok {
			panic(Runtime(fmt.Sprintf("%s: %q is not a date; use an ISO form such as \"2026-11-07\" or \"2026-11-07T12:00:00Z\"", fn, x.V)))
		}
		shape := shapeISO
		switch {
		case dayOnlyRe.MatchString(strings.TrimSpace(x.V)):
			shape = shapeDay
		case sqlDateRe.MatchString(strings.TrimSpace(x.V)):
			shape = shapeSQL
		}
		return time.UnixMilli(ms).UTC(), shape
	}
	panic(Runtime(fn + ": expected a Date or an ISO date string, got " + v.TypeName() + " " + v.Inspect()))
}

// fromTime returns t in the shape of the argument it was computed from.
func fromTime(t time.Time, shape dateShape, loc *time.Location) Value {
	switch shape {
	case shapeDay:
		return Str(FormatISO(t)[:10])
	case shapeSQL:
		s := FormatISO(t)
		return Str(s[:10] + " " + s[11:19])
	case shapeISO:
		return Str(FormatISO(t))
	}
	return NewDate(t.UnixMilli(), loc)
}

func maxArgs(fn string, args []Value, n int) {
	if len(args) > n {
		panic(Runtime(fmt.Sprintf("%s takes at most %d argument(s), got %d", fn, n, len(args))))
	}
}

func needArgs(fn string, args []Value, n int, usage string) {
	if len(args) < n {
		panic(Runtime(fmt.Sprintf("%s needs %d argument(s): %s", fn, n, usage)))
	}
}

func daysIn(y int, m time.Month) int { return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day() }

// addMonths adds n calendar months and clamps the day to the target month's
// length: Jan 31 + 1 month is Feb 28 (or 29).
func addMonths(t time.Time, n int) time.Time {
	total := t.Year()*12 + int(t.Month()) - 1 + n
	y, m := total/12, total%12
	if m < 0 {
		m += 12
		y--
	}
	d := min(t.Day(), daysIn(y, time.Month(m+1)))
	return time.Date(y, time.Month(m+1), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

var addUnits = map[string]float64{ // milliseconds
	"weeks": 7 * 86400000, "days": 86400000, "hours": 3600000, "minutes": 60000, "seconds": 1000,
	"ms": 1, "milliseconds": 1,
}

const addKeys = "years, months, weeks, days, hours, minutes, seconds, ms"

// addSpec applies {years, months, weeks, days, hours, minutes, seconds, ms}.
// Years and months go first, clamped to the month end; the rest are exact.
func addSpec(fn string, t time.Time, spec Value, sign int) time.Time {
	o, ok := spec.(*ObjectVal)
	if !ok {
		panic(Runtime(fn + ": the amount must be an object such as {days: 3} or {months: 1} (keys: " + addKeys + ")"))
	}
	months := 0
	var ms float64
	for _, k := range o.Keys() {
		v, _ := o.Get(k)
		n, isNum := v.(*NumberVal)
		if !isNum || math.IsNaN(n.V) || math.IsInf(n.V, 0) {
			panic(Runtime(fmt.Sprintf("%s: %s must be a number, got %s", fn, k, v.Inspect())))
		}
		switch k {
		case "years", "months":
			if n.V != math.Trunc(n.V) {
				panic(Runtime(fmt.Sprintf("%s: %s must be a whole number, got %s", fn, k, n.Display())))
			}
			if math.Abs(n.V) > 1e6 {
				panic(Runtime(fmt.Sprintf("%s: %s is out of range", fn, k)))
			}
			if k == "years" {
				months += int(n.V) * 12
			} else {
				months += int(n.V)
			}
		default:
			unit, known := addUnits[k]
			if !known {
				panic(Runtime(fmt.Sprintf("%s: unknown key %q (keys: %s)", fn, k, addKeys)))
			}
			ms += n.V * unit
		}
	}
	if math.Abs(ms) > 1e17 {
		panic(Runtime(fn + ": amount is out of range"))
	}
	t = addMonths(t, sign*months)
	return t.Add(time.Duration(float64(sign)*ms) * time.Millisecond)
}

func checkRange(fn string, t time.Time) time.Time {
	if ms := t.UnixMilli(); ms > 8_640_000_000_000_000 || ms < -8_640_000_000_000_000 {
		panic(Runtime(fn + ": the result is outside the Date range"))
	}
	return t
}

func diffMonths(a, b time.Time) int {
	if a.Before(b) {
		return -diffMonths(b, a)
	}
	n := (a.Year()-b.Year())*12 + int(a.Month()) - int(b.Month())
	for n > 0 && addMonths(b, n).After(a) {
		n--
	}
	return n
}

func startOf(fn string, t time.Time, unit string, weekStart int) time.Time {
	y, m, d := t.Date()
	switch unit {
	case "year":
		return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	case "quarter":
		return time.Date(y, time.Month((int(m)-1)/3*3+1), 1, 0, 0, 0, 0, time.UTC)
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	case "week":
		back := (int(t.Weekday()) - weekStart + 7) % 7
		return time.Date(y, m, d-back, 0, 0, 0, 0, time.UTC)
	case "day":
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	case "hour":
		return t.Truncate(time.Hour)
	case "minute":
		return t.Truncate(time.Minute)
	case "second":
		return t.Truncate(time.Second)
	}
	panic(Runtime(fmt.Sprintf("%s: unit %q is not supported (units: year, quarter, month, week, day, hour, minute, second)", fn, unit)))
}

func unitArg(fn string, v Value) string {
	s, ok := v.(*StringVal)
	if !ok {
		panic(Runtime(fn + ": the unit must be a string such as \"month\""))
	}
	return strings.TrimSuffix(s.V, "s")
}

// --- business days ---------------------------------------------------------------

type businessOpts struct{ holidays map[string]bool }

func parseBusinessOpts(fn string, v Value) businessOpts {
	b := businessOpts{holidays: map[string]bool{}}
	switch o := v.(type) {
	case *NullVal:
		return b
	case *ObjectVal:
		for _, k := range o.Keys() {
			if k != "holidays" {
				panic(Runtime(fmt.Sprintf("%s: option %q is not supported (supported: holidays; weekends are Saturday and Sunday)", fn, k)))
			}
		}
		h, _ := o.Get("holidays")
		arr, ok := h.(*ArrayVal)
		if h != nil && !ok {
			if _, isNull := h.(*NullVal); !isNull {
				panic(Runtime(fn + ": holidays must be an array of dates"))
			}
		}
		if ok {
			for _, e := range arr.Elements {
				t, _ := asTime(fn+" holidays", e)
				b.holidays[FormatISO(t)[:10]] = true
			}
		}
		return b
	}
	panic(Runtime(fn + ": the options must be an object such as {holidays: [\"2026-12-25\"]}"))
}

func (b businessOpts) isBusiness(t time.Time) bool {
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	return !b.holidays[FormatISO(t)[:10]]
}

func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// businessBetween counts business days in [a, b): a counts, b does not.
func (b businessOpts) between(a, z time.Time) int {
	a, z = civilDay(a), civilDay(z)
	if z.Before(a) {
		return -b.between(z, a)
	}
	days := int(z.Sub(a).Hours() / 24)
	count := days / 7 * 5
	cur := a.AddDate(0, 0, days/7*7)
	for cur.Before(z) {
		if wd := cur.Weekday(); wd != time.Saturday && wd != time.Sunday {
			count++
		}
		cur = cur.AddDate(0, 0, 1)
	}
	for h := range b.holidays {
		ht, _ := time.Parse("2006-01-02", h)
		if wd := ht.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		if !ht.Before(a) && ht.Before(z) {
			count--
		}
	}
	return count
}

// --- the namespace -----------------------------------------------------------------

func newDatesNamespace(loc *time.Location) *ObjectVal {
	ns := NewObject()
	def := func(name string, params []string, fn NativeFn) {
		ns.Set(name, &FuncVal{Name: "Dates." + name, Params: params, Body: &NativeBody{Fn: fn}})
	}

	def("add", []string{"date", "amount"}, func(args []Value) Value {
		const fn = "Dates.add"
		maxArgs(fn, args, 2)
		needArgs(fn, args, 2, "Dates.add(date, {days: 3, months: 1})")
		t, shape := asTime(fn, args[0])
		return fromTime(checkRange(fn, addSpec(fn, t, args[1], 1)), shape, loc)
	})
	def("subtract", []string{"date", "amount"}, func(args []Value) Value {
		const fn = "Dates.subtract"
		maxArgs(fn, args, 2)
		needArgs(fn, args, 2, "Dates.subtract(date, {days: 3})")
		t, shape := asTime(fn, args[0])
		return fromTime(checkRange(fn, addSpec(fn, t, args[1], -1)), shape, loc)
	})
	def("diff", []string{"a", "b", "unit"}, func(args []Value) Value {
		const fn = "Dates.diff"
		maxArgs(fn, args, 3)
		needArgs(fn, args, 2, "Dates.diff(a, b, unit?) is a - b, in whole units; default \"ms\"")
		a, _ := asTime(fn, args[0])
		b, _ := asTime(fn, args[1])
		unit := "ms"
		if len(args) > 2 {
			unit = unitArg(fn, args[2])
		}
		ms := a.UnixMilli() - b.UnixMilli()
		switch unit {
		case "ms", "millisecond", "milliseconds":
			return Num(float64(ms))
		case "second":
			return Num(float64(ms / 1000))
		case "minute":
			return Num(float64(ms / 60000))
		case "hour":
			return Num(float64(ms / 3600000))
		case "day":
			return Num(float64(ms / 86400000))
		case "week":
			return Num(float64(ms / (7 * 86400000)))
		case "month":
			return Num(float64(diffMonths(a, b)))
		case "year":
			return Num(float64(diffMonths(a, b) / 12))
		}
		panic(Runtime(fmt.Sprintf("%s: unit %q is not supported (units: ms, second, minute, hour, day, week, month, year)", fn, unit)))
	})
	boundary := func(name string, end bool) {
		fn := "Dates." + name
		def(name, []string{"date", "unit", "weekStartsOn"}, func(args []Value) Value {
			maxArgs(fn, args, 3)
			needArgs(fn, args, 2, fn+"(date, \"week\"|\"month\"|\"quarter\"|\"year\"|\"day\"|\"hour\"|\"minute\"|\"second\")")
			t, shape := asTime(fn, args[0])
			unit := unitArg(fn, args[1])
			weekStart := 1
			if len(args) > 2 {
				n, ok := args[2].(*NumberVal)
				if !ok || n.V != math.Trunc(n.V) || n.V < 0 || n.V > 6 {
					panic(Runtime(fn + ": weekStartsOn must be 0 (Sunday) to 6 (Saturday); the default is 1 (Monday)"))
				}
				weekStart = int(n.V)
			}
			s := startOf(fn, t, unit, weekStart)
			if !end {
				return fromTime(s, shape, loc)
			}
			var next time.Time
			switch unit {
			case "year":
				next = s.AddDate(1, 0, 0)
			case "quarter":
				next = addMonths(s, 3)
			case "month":
				next = addMonths(s, 1)
			case "week":
				next = s.AddDate(0, 0, 7)
			case "day":
				next = s.AddDate(0, 0, 1)
			case "hour":
				next = s.Add(time.Hour)
			case "minute":
				next = s.Add(time.Minute)
			default:
				next = s.Add(time.Second)
			}
			return fromTime(next.Add(-time.Millisecond), shape, loc)
		})
	}
	boundary("startOf", false)
	boundary("endOf", true)

	def("isoWeek", []string{"date"}, func(args []Value) Value {
		maxArgs("Dates.isoWeek", args, 1)
		t, _ := asTime("Dates.isoWeek", arg(args, 0))
		_, w := t.ISOWeek()
		return Num(float64(w))
	})
	def("isoWeekYear", []string{"date"}, func(args []Value) Value {
		maxArgs("Dates.isoWeekYear", args, 1)
		t, _ := asTime("Dates.isoWeekYear", arg(args, 0))
		y, _ := t.ISOWeek()
		return Num(float64(y))
	})
	def("daysInMonth", []string{"date"}, func(args []Value) Value {
		maxArgs("Dates.daysInMonth", args, 1)
		t, _ := asTime("Dates.daysInMonth", arg(args, 0))
		return Num(float64(daysIn(t.Year(), t.Month())))
	})

	def("isBusinessDay", []string{"date", "options"}, func(args []Value) Value {
		const fn = "Dates.isBusinessDay"
		maxArgs(fn, args, 2)
		t, _ := asTime(fn, arg(args, 0))
		return Bln(parseBusinessOpts(fn, arg(args, 1)).isBusiness(t))
	})
	def("businessDaysBetween", []string{"a", "b", "options"}, func(args []Value) Value {
		const fn = "Dates.businessDaysBetween"
		maxArgs(fn, args, 3)
		needArgs(fn, args, 2, "Dates.businessDaysBetween(a, b, {holidays: [...]?}) counts business days in [a, b)")
		a, _ := asTime(fn, args[0])
		b, _ := asTime(fn, args[1])
		return Num(float64(parseBusinessOpts(fn, arg(args, 2)).between(a, b)))
	})
	def("addBusinessDays", []string{"date", "n", "options"}, func(args []Value) Value {
		const fn = "Dates.addBusinessDays"
		maxArgs(fn, args, 3)
		needArgs(fn, args, 2, "Dates.addBusinessDays(date, n, {holidays: [...]?})")
		t, shape := asTime(fn, args[0])
		nv, ok := args[1].(*NumberVal)
		if !ok || nv.V != math.Trunc(nv.V) || math.Abs(nv.V) > 1e6 {
			panic(Runtime(fn + ": n must be a whole number (negative goes back), at most 1000000"))
		}
		b := parseBusinessOpts(fn, arg(args, 2))
		n := int(nv.V)
		step := 1
		if n < 0 {
			step, n = -1, -n
		}
		for n > 0 {
			t = t.AddDate(0, 0, step)
			if b.isBusiness(t) {
				n--
			}
		}
		return fromTime(checkRange(fn, t), shape, loc)
	})

	def("range", []string{"start", "end", "step"}, func(args []Value) Value {
		const fn = "Dates.range"
		maxArgs(fn, args, 3)
		needArgs(fn, args, 2, "Dates.range(start, end, step?) with step like {days: 7} or \"week\"; both ends are included")
		start, shape := asTime(fn, args[0])
		end, _ := asTime(fn, args[1])
		var step Value = func() Value { o := NewObject(); o.Set("days", Num(1)); return o }()
		if len(args) > 2 {
			switch s := args[2].(type) {
			case *StringVal:
				key := map[string]string{"year": "years", "month": "months", "week": "weeks", "day": "days", "hour": "hours", "minute": "minutes", "second": "seconds"}[strings.TrimSuffix(s.V, "s")]
				if key == "" {
					panic(Runtime(fmt.Sprintf("%s: step %q is not supported (use \"day\", \"week\", \"month\", \"year\", \"hour\", \"minute\", \"second\", or {days: 2})", fn, s.V)))
				}
				o := NewObject()
				o.Set(key, Num(1))
				step = o
			case *NumberVal:
				o := NewObject()
				o.Set("days", s)
				step = o
			default:
				step = args[2]
			}
		}
		// Each element is start + k steps, not the previous element + a step, so a
		// month step from Jan 31 gives Jan 31, Feb 28, Mar 31 and not Feb 28, Mar 28.
		if !addSpec(fn, start, step, 1).After(start) {
			panic(Runtime(fn + ": the step must move forward in time"))
		}
		const maxItems = 10000
		var out []Value
		for k := 1; ; k++ {
			if len(out) > maxItems {
				panic(Runtime(fmt.Sprintf("%s: more than %d dates; use a larger step", fn, maxItems)))
			}
			t := start
			if k > 1 {
				t = scaledStep(fn, start, step, k-1)
			}
			if t.After(end) {
				break
			}
			out = append(out, fromTime(t, shape, loc))
		}
		return &ArrayVal{Elements: out}
	})

	def("format", []string{"date", "pattern"}, func(args []Value) Value {
		const fn = "Dates.format"
		maxArgs(fn, args, 2)
		needArgs(fn, args, 2, "Dates.format(date, \"yyyy-MM-dd HH:mm\"); help(\"dates\") lists the letters")
		t, _ := asTime(fn, args[0])
		p, ok := args[1].(*StringVal)
		if !ok {
			panic(Runtime(fn + ": the pattern must be a string such as \"yyyy-MM-dd\""))
		}
		return Str(formatPattern(fn, t, p.V))
	})
	def("parse", []string{"text", "pattern"}, func(args []Value) Value {
		const fn = "Dates.parse"
		maxArgs(fn, args, 2)
		needArgs(fn, args, 2, "Dates.parse(\"07/11/2026\", \"dd/MM/yyyy\")")
		s, ok1 := args[0].(*StringVal)
		p, ok2 := args[1].(*StringVal)
		if !ok1 || !ok2 {
			panic(Runtime(fn + ": the text and the pattern must be strings"))
		}
		t := parsePattern(fn, s.V, p.V)
		return checkRangeDate(fn, t, loc)
	})
	return ns
}

func checkRangeDate(fn string, t time.Time, loc *time.Location) Value {
	checkRange(fn, t)
	return NewDate(t.UnixMilli(), loc)
}

// scaledStep is start plus k times the step spec.
func scaledStep(fn string, start time.Time, step Value, k int) time.Time {
	o := step.(*ObjectVal)
	scaled := NewObject()
	for _, key := range o.Keys() {
		v, _ := o.Get(key)
		n, ok := v.(*NumberVal)
		if !ok {
			return addSpec(fn, start, step, 1) // addSpec names the bad value
		}
		scaled.Set(key, Num(n.V*float64(k)))
	}
	return checkRange(fn, addSpec(fn, start, scaled, 1))
}

// --- patterns ------------------------------------------------------------------------

type patToken struct {
	letter byte // 0: literal
	n      int
	lit    string
}

// tokenize splits a Java-style pattern: runs of one letter, 'quoted' text, other characters.
func tokenize(fn, pat string) []patToken {
	var out []patToken
	for i := 0; i < len(pat); {
		c := pat[i]
		switch {
		case c == '\'':
			j := i + 1
			var lit strings.Builder
			for {
				if j >= len(pat) {
					panic(Runtime(fn + ": the pattern has an unclosed ' quote"))
				}
				if pat[j] == '\'' {
					if j+1 < len(pat) && pat[j+1] == '\'' {
						lit.WriteByte('\'')
						j += 2
						continue
					}
					break
				}
				lit.WriteByte(pat[j])
				j++
			}
			if j == i+1 {
				lit.WriteByte('\'')
			}
			out = append(out, patToken{lit: lit.String()})
			i = j + 1
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(pat) && pat[j] == c {
				j++
			}
			if !strings.ContainsRune("yYMdHhmsSaEwQZX", rune(c)) {
				panic(Runtime(fmt.Sprintf("%s: pattern letter %q is not supported (letters: yyyy YYYY MM MMM MMMM dd HH hh mm ss SSS a EEE EEEE ww Q Z XXX; quote text with 'single quotes')", fn, string(c))))
			}
			out = append(out, patToken{letter: c, n: j - i})
			i = j
		default:
			out = append(out, patToken{lit: string(c)})
			i++
		}
	}
	return out
}

func formatPattern(fn string, t time.Time, pat string) string {
	var b strings.Builder
	for _, tk := range tokenize(fn, pat) {
		if tk.letter == 0 {
			b.WriteString(tk.lit)
			continue
		}
		pad := func(v int) string { return fmt.Sprintf("%0*d", tk.n, v) }
		switch tk.letter {
		case 'y':
			if tk.n == 2 {
				b.WriteString(fmt.Sprintf("%02d", ((t.Year()%100)+100)%100))
			} else {
				b.WriteString(pad(t.Year()))
			}
		case 'Y':
			y, _ := t.ISOWeek()
			if tk.n == 2 {
				b.WriteString(fmt.Sprintf("%02d", ((y%100)+100)%100))
			} else {
				b.WriteString(pad(y))
			}
		case 'M':
			switch {
			case tk.n >= 4:
				b.WriteString(t.Month().String())
			case tk.n == 3:
				b.WriteString(t.Month().String()[:3])
			default:
				b.WriteString(pad(int(t.Month())))
			}
		case 'd':
			b.WriteString(pad(t.Day()))
		case 'H':
			b.WriteString(pad(t.Hour()))
		case 'h':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			b.WriteString(pad(h))
		case 'm':
			b.WriteString(pad(t.Minute()))
		case 's':
			b.WriteString(pad(t.Second()))
		case 'S':
			b.WriteString(fmt.Sprintf("%03d", t.Nanosecond()/1e6)[:min(tk.n, 3)])
		case 'a':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'E':
			if tk.n >= 4 {
				b.WriteString(t.Weekday().String())
			} else {
				b.WriteString(t.Weekday().String()[:3])
			}
		case 'w':
			_, w := t.ISOWeek()
			b.WriteString(pad(w))
		case 'Q':
			b.WriteString(pad((int(t.Month())-1)/3 + 1))
		case 'Z':
			b.WriteString("+0000")
		case 'X':
			b.WriteString("Z")
		}
	}
	return b.String()
}

func parsePattern(fn, text, pat string) time.Time {
	fail := func(why string) {
		panic(Runtime(fmt.Sprintf("%s: %q does not match the pattern %q (%s)", fn, text, pat, why)))
	}
	year, month, day := 1970, 1, 1
	hour, minute, sec, ms := 0, 0, 0, 0
	pm, hasAmPm := false, false
	var offset time.Duration
	pos := 0
	number := func(lo, hi int) int {
		start := pos
		for pos < len(text) && text[pos] >= '0' && text[pos] <= '9' && pos-start < hi {
			pos++
		}
		if pos-start < lo {
			fail(fmt.Sprintf("expected a number at position %d", start+1))
		}
		n := 0
		for _, c := range text[start:pos] {
			n = n*10 + int(c-'0')
		}
		return n
	}
	// width: a two-letter field or one next to another number has a fixed width.
	width := func(tk patToken, adjacent bool) (int, int) {
		if adjacent || tk.n >= 2 {
			return 2, 2
		}
		return 1, 2
	}
	name := func(names []string, abbr bool) int {
		for i, n := range names {
			if abbr {
				n = n[:3]
			}
			if len(text)-pos >= len(n) && strings.EqualFold(text[pos:pos+len(n)], n) {
				pos += len(n)
				return i
			}
		}
		fail(fmt.Sprintf("expected a name at position %d", pos+1))
		return 0
	}
	months := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	toks := tokenize(fn, pat)
	for i, tk := range toks {
		// A number token followed by another number token has a fixed width.
		adjacent := i+1 < len(toks) && toks[i+1].letter != 0 && strings.ContainsRune("yYMdHhmsS", rune(toks[i+1].letter))
		if tk.letter == 0 {
			if !strings.HasPrefix(text[pos:], tk.lit) {
				fail(fmt.Sprintf("expected %q at position %d", tk.lit, pos+1))
			}
			pos += len(tk.lit)
			continue
		}
		switch tk.letter {
		case 'y', 'Y':
			switch {
			case tk.n == 2:
				year = number(2, 2)
				if year < 70 {
					year += 2000
				} else {
					year += 1900
				}
			default:
				year = number(max(tk.n, 4), max(tk.n, 4))
			}
		case 'M':
			switch {
			case tk.n >= 4:
				month = name(months, false) + 1
			case tk.n == 3:
				month = name(months, true) + 1
			default:
				month = number(width(tk, adjacent))
			}
		case 'd':
			day = number(width(tk, adjacent))
		case 'H':
			hour = number(width(tk, adjacent))
		case 'h':
			hour = number(width(tk, adjacent))
		case 'm':
			minute = number(width(tk, adjacent))
		case 's':
			sec = number(width(tk, adjacent))
		case 'S':
			start := pos
			f := number(1, 9)
			digits := pos - start
			for ; digits < 3; digits++ {
				f *= 10
			}
			for ; digits > 3; digits-- {
				f /= 10
			}
			ms = f
		case 'a':
			switch {
			case len(text)-pos >= 2 && strings.EqualFold(text[pos:pos+2], "AM"):
			case len(text)-pos >= 2 && strings.EqualFold(text[pos:pos+2], "PM"):
				pm = true
			default:
				fail(fmt.Sprintf("expected AM or PM at position %d", pos+1))
			}
			hasAmPm = true
			pos += 2
		case 'E':
			name(days, tk.n < 4)
		case 'w', 'Q':
			number(1, 2)
		case 'Z', 'X':
			switch {
			case pos < len(text) && (text[pos] == 'Z' || text[pos] == 'z'):
				pos++
			case pos < len(text) && (text[pos] == '+' || text[pos] == '-'):
				sign := time.Duration(1)
				if text[pos] == '-' {
					sign = -1
				}
				pos++
				oh := number(2, 2)
				if pos < len(text) && text[pos] == ':' {
					pos++
				}
				om := number(2, 2)
				offset = sign * (time.Duration(oh)*time.Hour + time.Duration(om)*time.Minute)
			default:
				fail(fmt.Sprintf("expected Z or +hh:mm at position %d", pos+1))
			}
		}
	}
	if pos != len(text) {
		fail(fmt.Sprintf("unexpected %q at position %d", text[pos:], pos+1))
	}
	if hasAmPm {
		if hour < 1 || hour > 12 {
			fail("the hour must be 1 to 12 with a")
		}
		hour %= 12
		if pm {
			hour += 12
		}
	}
	if month < 1 || month > 12 || day < 1 || day > daysIn(year, time.Month(month)) || hour > 24 || minute > 59 || sec > 59 {
		fail("a field is out of range")
	}
	return time.Date(year, time.Month(month), day, hour, minute, sec, ms*1e6, time.UTC).Add(-offset)
}

const datesGuide = `Dates — Date (JS subset) and the Dates helpers

TIME ZONE: local time is UTC, unless the operator sets --tz. Month is 0-based in Date (new Date(2026, 10, 7) is Nov 7), 1-based in strings.
  Date.now() reads the host clock (not I/O). An invalid date is NaN in arithmetic and prints "Invalid Date".

DATE (as JS):
  new Date()  new Date(ms)  new Date("2026-11-07")  new Date(y, m, d?, h?, mi?, s?, ms?)  Date.now()  Date.UTC(y, m, ...)  Date.parse(s)
  getFullYear getMonth getDate getDay getHours getMinutes getSeconds getMilliseconds (and getUTC…)  same with set…  getTime valueOf
  toISOString toJSON toString toDateString toUTCString toLocaleDateString toLocaleTimeString toLocaleString
  d2 - d1 is ms; d1 < d2 compares; toJson(d) is the ISO string. Printing a Date gives the ISO form, not JS's long form.
  Strings: ISO forms, "2026-11-07 12:00:00", "Nov 7, 2026", "7 November 2026 10:00". Date-only ISO is UTC; with a time and no offset it is local.
  toLocale*: en-US only; options weekday year month day hour minute second hour12 timeZone dateStyle timeStyle. Others are errors.

Dates.* take a Date, an ISO string ("2026-11-07", "2026-11-07 12:00:00", "…T…Z") or ms. A string gives a string of the same shape; a Date or ms gives a Date.
  They work on the UTC calendar.
  Dates.add(d, {years, months, weeks, days, hours, minutes, seconds, ms})   months clamp to the month end: Jan 31 + 1 month = Feb 28
  Dates.subtract(d, {…})   Dates.diff(a, b, unit?)  a - b in whole units: ms second minute hour day week month year
  Dates.startOf(d, unit, weekStartsOn?)  Dates.endOf(d, unit, …)   unit: year quarter month week day hour minute second; the week starts on Monday (1)
  Dates.isoWeek(d)  Dates.isoWeekYear(d)  Dates.daysInMonth(d)
  Dates.isBusinessDay(d, opts?)  Dates.businessDaysBetween(a, b, opts?) counts [a, b)  Dates.addBusinessDays(d, n, opts?)   opts: {holidays: [dates]}; weekend = Sat, Sun
  Dates.range(start, end, step?)  both ends included; step {days: 7}, "week", "month", or a number of days (default 1); at most 10000 dates
  Dates.format(d, pattern)  Dates.parse(text, pattern)   pattern letters: yyyy yy YYYY (ISO week year) MM MMM MMMM dd HH hh mm ss SSS a EEE EEEE ww (ISO week) Q Z XXX; 'quoted text'
  Dates.parse fails with an error when the text does not match, and returns a Date.

EXAMPLES
  new Date("2026-11-07").getDay()                      // 6
  Dates.add("2026-01-31", {months: 1})                 // "2026-02-28"
  Dates.diff("2026-12-25", "2026-11-07", "day")        // 48
  Dates.format(new Date(2026, 10, 7), "EEE, d MMM yyyy")  // "Sat, 7 Nov 2026"
  Dates.businessDaysBetween("2026-11-02", "2026-11-09")   // 5
  Dates.range("2026-01-31", "2026-04-30", "month")     // ["2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"]

NUMBER AND TABLE FORMATS: help("format")`
