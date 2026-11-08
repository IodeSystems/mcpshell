package runtime

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DateVal is a JS Date: milliseconds since the epoch (exact, an int64), or
// invalid (NaN in JS). Loc is the local zone for the get*/set* methods that
// are not UTC; it is UTC unless the operator sets a zone (docs/design.md).
// A Date is mutable, as in JS: the setters change it in place.
type DateVal struct {
	ms    int64
	valid bool
	loc   *time.Location
}

// maxDateMs is the JS limit: a time value beyond ±8.64e15 ms is NaN.
const maxDateMs = 8_640_000_000_000_000

func (*DateVal) isValue()         {}
func (*DateVal) TypeName() string { return "object" }
func (*DateVal) IsTruthy() bool   { return true }

// Display is the ISO form (or "Invalid Date"), not JS's long toString form:
// it is what the model reads, and what JSON and SQL carry. d.toString()
// gives the JS form.
func (d *DateVal) Display() string {
	if !d.valid {
		return "Invalid Date"
	}
	return d.ISO()
}
func (d *DateVal) Inspect() string { return d.Display() }

// NewDate returns a Date for ms milliseconds since the epoch, or an invalid
// Date when ms is outside the JS range. A nil loc is UTC.
func NewDate(ms int64, loc *time.Location) *DateVal {
	if loc == nil {
		loc = time.UTC
	}
	if ms > maxDateMs || ms < -maxDateMs {
		return &DateVal{loc: loc}
	}
	return &DateVal{ms: ms, valid: true, loc: loc}
}

// InvalidDate returns the Date that JS prints as "Invalid Date".
func InvalidDate(loc *time.Location) *DateVal {
	if loc == nil {
		loc = time.UTC
	}
	return &DateVal{loc: loc}
}

// Ms returns the time value and whether the Date is valid.
func (d *DateVal) Ms() (int64, bool) { return d.ms, d.valid }

// Valid reports whether the Date holds a time.
func (d *DateVal) Valid() bool { return d.valid }

// Loc is the Date's local zone.
func (d *DateVal) Loc() *time.Location { return d.loc }

// UTC returns the time in UTC; the zero time when the Date is invalid.
func (d *DateVal) UTC() time.Time {
	if !d.valid {
		return time.Time{}
	}
	return time.UnixMilli(d.ms).UTC()
}

func (d *DateVal) local() time.Time { return time.UnixMilli(d.ms).In(d.loc) }

// msNumber is the JS valueOf: ms, or NaN.
func (d *DateVal) msNumber() *NumberVal {
	if !d.valid {
		return Num(math.NaN())
	}
	return numInt(d.ms)
}

// DateMs is the number a Date stands for in arithmetic and Number(d).
func DateMs(d *DateVal) *NumberVal { return d.msNumber() }

// ISO is toISOString for a valid Date.
func (d *DateVal) ISO() string { return FormatISO(d.UTC()) }

// FormatISO renders t (taken in UTC) as 2026-11-07T00:00:00.000Z, with the
// ±YYYYYY year form outside 0..9999, as JS does.
func FormatISO(t time.Time) string {
	t = t.UTC()
	y := t.Year()
	var ys string
	switch {
	case y >= 0 && y <= 9999:
		ys = fmt.Sprintf("%04d", y)
	case y < 0:
		ys = fmt.Sprintf("-%06d", -y)
	default:
		ys = fmt.Sprintf("+%06d", y)
	}
	return fmt.Sprintf("%s-%02d-%02dT%02d:%02d:%02d.%03dZ", ys, int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1e6)
}

var (
	dayNames   = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	monthNames = []string{"January", "February", "March", "April", "May", "June", "July",
		"August", "September", "October", "November", "December"}
)

func zoneName(t time.Time) string {
	if t.Location() == time.UTC {
		return "Coordinated Universal Time"
	}
	n, _ := t.Zone()
	return n
}

func offsetText(t time.Time) string {
	_, off := t.Zone()
	sign := '+'
	if off < 0 {
		sign, off = '-', -off
	}
	return fmt.Sprintf("GMT%c%02d%02d", sign, off/3600, off%3600/60)
}

func yearText(y int) string {
	if y < 0 {
		return fmt.Sprintf("-%06d", -y)
	}
	return fmt.Sprintf("%04d", y)
}

func (d *DateVal) dateString(t time.Time) string {
	return fmt.Sprintf("%s %s %02d %s", dayNames[t.Weekday()][:3], monthNames[t.Month()-1][:3], t.Day(), yearText(t.Year()))
}

func (d *DateVal) timeString(t time.Time) string {
	return fmt.Sprintf("%02d:%02d:%02d %s (%s)", t.Hour(), t.Minute(), t.Second(), offsetText(t), zoneName(t))
}

// --- construction -----------------------------------------------------------

const maxField = 1e12

// makeTime builds a time value from fields in loc, as JS MakeDate(MakeDay,
// MakeTime): fields out of range carry into the next unit. ok is false when a
// field is not finite or the result is outside the Date range.
func makeTime(loc *time.Location, f [7]float64) (int64, bool) {
	var n [7]int
	for i, x := range f {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		x = math.Trunc(x)
		lim := maxField
		if i < 2 {
			lim = 1e9
		}
		if math.Abs(x) > lim {
			return 0, false
		}
		n[i] = int(x)
	}
	// time.Date carries out-of-range months, days and times into the next unit,
	// as JS MakeDay and MakeTime do.
	t := time.Date(n[0], time.Month(n[1]+1), n[2], n[3], n[4], n[5], n[6]*1e6, loc)
	ms := t.UnixMilli()
	if ms > maxDateMs || ms < -maxDateMs {
		return 0, false
	}
	return ms, true
}

// --- argument conversion -----------------------------------------------------

// dateArg converts a Date method argument to a float64 as JS ToNumber does
// for the cases that arise: numbers, numeric strings and booleans.
func dateArg(fn string, v Value) float64 {
	switch x := v.(type) {
	case *NumberVal:
		return x.V
	case *StringVal:
		if n, ok := ParseNumber(strings.TrimSpace(x.V)); ok {
			return n.V
		}
		return math.NaN()
	case *BoolVal:
		if x.V {
			return 1
		}
		return 0
	case *DateVal:
		if !x.valid {
			return math.NaN()
		}
		return float64(x.ms)
	}
	panic(TypeMismatch(fn, "number", v, "give a number; null stands for undefined here and is not accepted"))
}

func fnVal(name string, params []string, fn NativeFn) *FuncVal {
	return &FuncVal{Name: name, Params: params, Body: &NativeBody{Fn: fn}}
}

// tooMany is the error for extra arguments: the repo rule is that they are
// errors, not dropped.
func tooMany(fn string, args []Value, max int) {
	if len(args) > max {
		panic(Runtime(fmt.Sprintf("%s takes at most %d argument(s), got %d", fn, max, len(args))))
	}
}

// --- the Date namespace -------------------------------------------------------

// NewDateNamespace returns the global `Date`: new Date(...), Date.now(),
// Date.UTC(...), Date.parse(s). loc is the local zone ("local" = UTC by
// default). now is the clock; nil reads the host clock.
func NewDateNamespace(loc *time.Location, now func() time.Time) *ObjectVal {
	if loc == nil {
		loc = time.UTC
	}
	if now == nil {
		now = time.Now
	}
	ns := NewObject()
	ns.Set("now", fnVal("Date.now", nil, func(args []Value) Value {
		tooMany("Date.now", args, 0)
		return numInt(now().UnixMilli())
	}))
	ns.Set("UTC", fnVal("Date.UTC", []string{"year", "month", "day", "hours", "minutes", "seconds", "ms"}, func(args []Value) Value {
		tooMany("Date.UTC", args, 7)
		if len(args) == 0 {
			return Num(math.NaN())
		}
		f := fieldsFromArgs("Date.UTC", args, true)
		ms, ok := makeTime(time.UTC, f)
		if !ok {
			return Num(math.NaN())
		}
		return numInt(ms)
	}))
	ns.Set("parse", fnVal("Date.parse", []string{"string"}, func(args []Value) Value {
		tooMany("Date.parse", args, 1)
		s, ok := arg0(args).(*StringVal)
		if !ok {
			panic(TypeMismatch("Date.parse", "string", arg0(args), ""))
		}
		ms, ok := ParseDateString(s.V, loc)
		if !ok {
			return Num(math.NaN())
		}
		return numInt(ms)
	}))
	ns.Set("__new", fnVal("Date", nil, func(args []Value) Value {
		return constructDate(args, loc, now)
	}))
	// Date() without new is the current time as a string, whatever the arguments.
	ns.Set("__call", fnVal("Date", nil, func(args []Value) Value {
		return Str(constructDate(nil, loc, now).(*DateVal).toString())
	}))
	return ns
}

func arg0(args []Value) Value {
	if len(args) == 0 {
		return Null
	}
	return args[0]
}

func fieldsFromArgs(fn string, args []Value, twoDigitYear bool) [7]float64 {
	f := [7]float64{0, 0, 1, 0, 0, 0, 0}
	for i, a := range args {
		f[i] = dateArg(fn, a)
	}
	if twoDigitYear && !math.IsNaN(f[0]) {
		if y := math.Trunc(f[0]); y >= 0 && y <= 99 {
			f[0] = 1900 + y
		}
	}
	return f
}

func constructDate(args []Value, loc *time.Location, now func() time.Time) Value {
	switch len(args) {
	case 0:
		return NewDate(now().UnixMilli(), loc)
	case 1:
		switch x := args[0].(type) {
		case *DateVal:
			c := *x
			c.loc = loc
			return &c
		case *StringVal:
			ms, ok := ParseDateString(x.V, loc)
			if !ok {
				return InvalidDate(loc)
			}
			return NewDate(ms, loc)
		case *NumberVal:
			if math.IsNaN(x.V) || math.IsInf(x.V, 0) {
				return InvalidDate(loc)
			}
			return NewDate(int64(math.Trunc(x.V)), loc)
		}
		panic(TypeMismatch("new Date", "number, string or Date", args[0], "null stands for undefined here and is not accepted"))
	}
	tooMany("new Date", args, 7)
	ms, ok := makeTime(loc, fieldsFromArgs("new Date", args, true))
	if !ok {
		return InvalidDate(loc)
	}
	return NewDate(ms, loc)
}

// --- parsing ------------------------------------------------------------------

var isoRe = regexp.MustCompile(`^([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?)?` +
	`(?:[Tt ](\d{2}):(\d{2})(?::(\d{2})(?:[.,](\d+))?)?\s*(Z|z|[+-]\d{2}(?::?\d{2})?)?)?$`)

// ParseDateString parses the ISO 8601 forms and the common text forms that
// JS accepts. A date-only ISO string is UTC, one with a time and no offset is
// in loc, as in JS. Anything else is not a date (ok false), where V8's legacy
// parser would guess.
func ParseDateString(s string, loc *time.Location) (int64, bool) {
	s = strings.TrimSpace(s)
	if m := isoRe.FindStringSubmatch(s); m != nil {
		return parseISO(m, loc)
	}
	return parseLegacy(s, loc)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func parseISO(m []string, loc *time.Location) (int64, bool) {
	if m[1] == "-000000" {
		return 0, false
	}
	year := atoi(m[1])
	month, day := 1, 1
	if m[2] != "" {
		month = atoi(m[2])
	}
	if m[3] != "" {
		day = atoi(m[3])
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, false
	}
	hasTime := m[4] != ""
	var h, mi, sec, ms int
	if hasTime {
		h, mi = atoi(m[4]), atoi(m[5])
		sec = atoi(m[6])
		if m[7] != "" {
			frac := (m[7] + "00")[:3]
			ms = atoi(frac)
		}
		if h > 24 || mi > 59 || sec > 59 || (h == 24 && (mi != 0 || sec != 0 || ms != 0)) {
			return 0, false
		}
	}
	zone := time.UTC
	var offset *time.Duration
	if hasTime {
		switch z := m[8]; {
		case z == "":
			zone = loc
		case z == "Z" || z == "z":
		default:
			digits := strings.ReplaceAll(z[1:], ":", "")
			oh := atoi(digits[:2])
			om := 0
			if len(digits) >= 4 {
				om = atoi(digits[2:4])
			}
			if oh > 23 || om > 59 {
				return 0, false
			}
			d := time.Duration(oh)*time.Hour + time.Duration(om)*time.Minute
			if z[0] == '-' {
				d = -d
			}
			offset = &d
		}
	}
	v, ok := makeTime(zone, [7]float64{float64(year), float64(month - 1), float64(day), float64(h), float64(mi), float64(sec), float64(ms)})
	if !ok {
		return 0, false
	}
	if offset != nil {
		v -= offset.Milliseconds()
	}
	if v > maxDateMs || v < -maxDateMs {
		return 0, false
	}
	return v, true
}

var (
	legacyDates = []string{"Jan 2, 2006", "January 2, 2006", "2 Jan 2006", "2 January 2006", "Jan 2 2006", "January 2 2006",
		"Mon Jan 2 2006", "Mon, Jan 2, 2006", "Mon, 2 Jan 2006", "Mon Jan 02 2006", "Monday, January 2, 2006", "1/2/2006", "2006/1/2"}
	legacyTimes = []string{"", " 15:04", " 15:04:05", " 3:04 PM", " 3:04:05 PM", " 15:04:05 GMT", " 15:04:05 MST",
		" 15:04:05 GMT-0700", " 15:04:05 -0700", " 15:04 MST", " 15:04:05 UTC", " 15:04:05.000 GMT"}
	parenRe = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
)

func parseLegacy(s string, loc *time.Location) (int64, bool) {
	s = parenRe.ReplaceAllString(s, "")
	if s == "" {
		return 0, false
	}
	for _, dl := range legacyDates {
		for _, tl := range legacyTimes {
			t, err := time.ParseInLocation(dl+tl, s, loc)
			if err != nil {
				continue
			}
			if strings.Contains(tl, "GMT") || strings.Contains(tl, "UTC") {
				// "GMT" and "UTC" parse as a zone of that name: the instant is right.
			}
			ms := t.UnixMilli()
			if ms > maxDateMs || ms < -maxDateMs {
				return 0, false
			}
			return ms, true
		}
	}
	return 0, false
}

// --- members -------------------------------------------------------------------

type dateField int

const (
	fYear dateField = iota
	fMonth
	fDay
	fHour
	fMinute
	fSecond
	fMs
)

func fieldsOf(t time.Time) [7]float64 {
	return [7]float64{float64(t.Year()), float64(int(t.Month()) - 1), float64(t.Day()),
		float64(t.Hour()), float64(t.Minute()), float64(t.Second()), float64(t.Nanosecond() / 1e6)}
}

var getters = map[string]dateField{
	"FullYear": fYear, "Month": fMonth, "Date": fDay, "Hours": fHour, "Minutes": fMinute, "Seconds": fSecond, "Milliseconds": fMs,
}

// setter name → first field and how many arguments it takes.
var setters = map[string]struct {
	first dateField
	max   int
}{
	"FullYear": {fYear, 3}, "Month": {fMonth, 2}, "Date": {fDay, 1},
	"Hours": {fHour, 4}, "Minutes": {fMinute, 3}, "Seconds": {fSecond, 2}, "Milliseconds": {fMs, 1},
}

func nan() Value { return Num(math.NaN()) }

func (d *DateVal) tzFor(utc bool) *time.Location {
	if utc {
		return time.UTC
	}
	return d.loc
}

func (d *DateVal) toString() string {
	if !d.valid {
		return "Invalid Date"
	}
	t := d.local()
	return d.dateString(t) + " " + d.timeString(t)
}

// DateMember returns the method or value for d.field, or ok false when Date
// has no such member.
func DateMember(d *DateVal, field string) (Value, bool) {
	fn := func(params []string, f NativeFn) (Value, bool) { return fnVal(field, params, f), true }
	switch field {
	case "getTime", "valueOf":
		return fn(nil, func(args []Value) Value { tooMany(field, args, 0); return d.msNumber() })
	case "getDay", "getUTCDay":
		utc := field == "getUTCDay"
		return fn(nil, func(args []Value) Value {
			tooMany(field, args, 0)
			if !d.valid {
				return nan()
			}
			return numInt(int64(time.UnixMilli(d.ms).In(d.tzFor(utc)).Weekday()))
		})
	case "getTimezoneOffset":
		return fn(nil, func(args []Value) Value {
			tooMany(field, args, 0)
			if !d.valid {
				return nan()
			}
			_, off := d.local().Zone()
			return numInt(int64(-off / 60))
		})
	case "setTime":
		return fn([]string{"ms"}, func(args []Value) Value {
			tooMany(field, args, 1)
			x := dateArg(field, arg0(args))
			if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(math.Trunc(x)) > maxDateMs {
				d.valid = false
				return nan()
			}
			d.ms, d.valid = int64(math.Trunc(x)), true
			return numInt(d.ms)
		})
	case "toISOString":
		return fn(nil, func(args []Value) Value {
			tooMany(field, args, 0)
			if !d.valid {
				panic(Runtime("toISOString: Invalid Date (JS raises RangeError: Invalid time value)"))
			}
			return Str(d.ISO())
		})
	case "toJSON":
		return fn(nil, func(args []Value) Value {
			if !d.valid {
				return Null
			}
			return Str(d.ISO())
		})
	case "toString":
		return fn(nil, func(args []Value) Value { return Str(d.toString()) })
	case "toDateString":
		return fn(nil, func(args []Value) Value {
			if !d.valid {
				return Str("Invalid Date")
			}
			return Str(d.dateString(d.local()))
		})
	case "toTimeString":
		return fn(nil, func(args []Value) Value {
			if !d.valid {
				return Str("Invalid Date")
			}
			return Str(d.timeString(d.local()))
		})
	case "toUTCString", "toGMTString":
		return fn(nil, func(args []Value) Value {
			if !d.valid {
				return Str("Invalid Date")
			}
			t := d.UTC()
			return Str(fmt.Sprintf("%s, %02d %s %s %02d:%02d:%02d GMT", dayNames[t.Weekday()][:3], t.Day(),
				monthNames[t.Month()-1][:3], yearText(t.Year()), t.Hour(), t.Minute(), t.Second()))
		})
	case "toLocaleDateString", "toLocaleTimeString", "toLocaleString":
		kind := map[string]string{"toLocaleDateString": "date", "toLocaleTimeString": "time", "toLocaleString": "all"}[field]
		return fn([]string{"locale", "options"}, func(args []Value) Value {
			tooMany(field, args, 2)
			spec := parseDateFormat(field, arg0(args), argAt(args, 1), kind, d.loc)
			return Str(spec.format(d))
		})
	}
	name := strings.TrimPrefix(field, "get")
	utc := false
	if strings.HasPrefix(field, "getUTC") {
		name, utc = strings.TrimPrefix(field, "getUTC"), true
	}
	if strings.HasPrefix(field, "get") {
		if f, ok := getters[name]; ok {
			return fn(nil, func(args []Value) Value {
				tooMany(field, args, 0)
				if !d.valid {
					return nan()
				}
				return numInt(int64(fieldsOf(time.UnixMilli(d.ms).In(d.tzFor(utc)))[f]))
			})
		}
	}
	if strings.HasPrefix(field, "set") {
		name, utc = strings.TrimPrefix(field, "set"), false
		if strings.HasPrefix(field, "setUTC") {
			name, utc = strings.TrimPrefix(field, "setUTC"), true
		}
		if s, ok := setters[name]; ok {
			return fn(nil, func(args []Value) Value {
				if len(args) == 0 {
					d.valid = false
					return nan()
				}
				tooMany(field, args, s.max)
				var base [7]float64
				switch {
				case d.valid:
					base = fieldsOf(time.UnixMilli(d.ms).In(d.tzFor(utc)))
				case s.first == fYear:
					base = fieldsOf(time.UnixMilli(0).In(d.tzFor(utc)))
				default:
					for _, a := range args {
						dateArg(field, a)
					}
					return nan()
				}
				for i, a := range args {
					base[int(s.first)+i] = dateArg(field, a)
				}
				ms, ok := makeTime(d.tzFor(utc), base)
				if !ok {
					d.valid = false
					return nan()
				}
				d.ms, d.valid = ms, true
				return numInt(ms)
			})
		}
	}
	return nil, false
}

func argAt(args []Value, i int) Value {
	if i < len(args) {
		return args[i]
	}
	return Null
}

// dateMemberOrPanic is accessMember for a Date.
func dateMemberOrPanic(d *DateVal, field string) Value {
	if v, ok := DateMember(d, field); ok {
		return v
	}
	panic(Runtime("a Date has no member '" + field + "' — it has getFullYear/getMonth/getDate/getDay/getHours/getMinutes/getSeconds/getMilliseconds (and getUTC…), the same set… set…, getTime, valueOf, toISOString, toJSON, toString, toDateString, toUTCString, toLocaleDateString, toLocaleTimeString, toLocaleString; help(\"dates\")"))
}

// isDateOperand reports whether a comparison has a Date on one side and a
// Date or number on the other.
func isDateOperand(a, b Value) bool {
	da, aok := a.(*DateVal)
	db, bok := b.(*DateVal)
	switch {
	case aok && bok:
		return true
	case aok:
		_, n := b.(*NumberVal)
		return n && da != nil
	case bok:
		_, n := a.(*NumberVal)
		return n && db != nil
	}
	return false
}

// dateCompared is false for an invalid Date or a NaN.
func dateCompared(v Value) bool {
	switch x := v.(type) {
	case *DateVal:
		return x.valid
	case *NumberVal:
		return !math.IsNaN(x.V)
	}
	return true
}
