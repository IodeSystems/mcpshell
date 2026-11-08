package runtime

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// dateFormat is the en-US subset of Intl.DateTimeFormat. Options it does not
// implement are errors that list what is supported, never ignored: an ignored
// option would change the text without a sign.
type dateFormat struct {
	weekday, year, month, day, hour, minute, second string
	hour12                                          bool
	dateStyle, timeStyle                            string
	loc                                             *time.Location
	utcName                                         bool
}

const supportedDateOptions = "weekday, year, month, day, hour, minute, second, hour12, timeZone, dateStyle, timeStyle"

func optionString(fn string, o *ObjectVal, key string, allowed ...string) string {
	v, ok := o.Get(key)
	if !ok || isNullValue(v) {
		return ""
	}
	s, ok := v.(*StringVal)
	if ok {
		for _, a := range allowed {
			if s.V == a {
				return s.V
			}
		}
	}
	panic(Runtime(fmt.Sprintf("%s: option %s must be one of %s, got %s", fn, key, strings.Join(allowed, ", "), v.Inspect())))
}

func isNullValue(v Value) bool { _, ok := v.(*NullVal); return ok }

// checkLocale accepts en-US and en (and no locale); anything else is an error.
func checkLocale(fn string, locale Value) {
	switch l := locale.(type) {
	case *NullVal:
		return
	case *StringVal:
		if l.V == "en-US" || l.V == "en" {
			return
		}
		panic(Runtime(fmt.Sprintf("%s: locale %q is not supported; only \"en-US\" (also \"en\", or none) is", fn, l.V)))
	}
	panic(Runtime(fn + ": the locale must be a string such as \"en-US\", got " + locale.Inspect()))
}

func checkOptionKeys(fn string, o *ObjectVal, supported string) {
	ok := map[string]bool{}
	for _, k := range strings.Split(supported, ", ") {
		ok[k] = true
	}
	var bad []string
	for _, k := range o.Keys() {
		if !ok[k] {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		panic(Runtime(fmt.Sprintf("%s: option %s is not supported (supported: %s)", fn, strings.Join(bad, ", "), supported)))
	}
}

func optionsObject(fn string, v Value) *ObjectVal {
	switch o := v.(type) {
	case *NullVal:
		return NewObject()
	case *ObjectVal:
		return o
	}
	panic(Runtime(fn + ": options must be an object, got " + v.Inspect()))
}

// parseDateFormat reads (locale, options) for toLocaleDateString (kind
// "date"), toLocaleTimeString ("time") and toLocaleString ("all"); "dtf" is Intl.DateTimeFormat.
func parseDateFormat(fn string, locale, options Value, kind string, local *time.Location) *dateFormat {
	checkLocale(fn, locale)
	o := optionsObject(fn, options)
	checkOptionKeys(fn, o, supportedDateOptions)
	f := &dateFormat{loc: local, hour12: true}
	f.weekday = optionString(fn, o, "weekday", "long", "short", "narrow")
	f.year = optionString(fn, o, "year", "numeric", "2-digit")
	f.month = optionString(fn, o, "month", "numeric", "2-digit", "long", "short", "narrow")
	f.day = optionString(fn, o, "day", "numeric", "2-digit")
	f.hour = optionString(fn, o, "hour", "numeric", "2-digit")
	f.minute = optionString(fn, o, "minute", "numeric", "2-digit")
	f.second = optionString(fn, o, "second", "numeric", "2-digit")
	f.dateStyle = optionString(fn, o, "dateStyle", "full", "long", "medium", "short")
	f.timeStyle = optionString(fn, o, "timeStyle", "full", "long", "medium", "short")
	if v, ok := o.Get("hour12"); ok && !isNullValue(v) {
		b, isBool := v.(*BoolVal)
		if !isBool {
			panic(Runtime(fn + ": option hour12 must be true or false, got " + v.Inspect()))
		}
		f.hour12 = b.V
	}
	if v, ok := o.Get("timeZone"); ok && !isNullValue(v) {
		s, isStr := v.(*StringVal)
		if !isStr {
			panic(Runtime(fn + ": option timeZone must be a string such as \"UTC\", got " + v.Inspect()))
		}
		if s.V == "UTC" || s.V == "GMT" || s.V == "Etc/UTC" {
			f.loc, f.utcName = time.UTC, true
		} else if loc, err := time.LoadLocation(s.V); err == nil && strings.Contains(s.V, "/") {
			f.loc = loc
		} else {
			panic(Runtime(fmt.Sprintf("%s: timeZone %q is not supported; use \"UTC\" or an IANA name such as \"America/New_York\"", fn, s.V)))
		}
	}
	if f.loc == time.UTC {
		f.utcName = true
	}
	if f.dateStyle != "" || f.timeStyle != "" {
		if f.weekday+f.year+f.month+f.day+f.hour+f.minute+f.second != "" {
			panic(Runtime(fn + ": dateStyle/timeStyle cannot be combined with weekday, year, month, day, hour, minute or second"))
		}
		if kind == "date" && f.timeStyle != "" {
			panic(Runtime(fn + ": timeStyle is not allowed in toLocaleDateString"))
		}
		if kind == "time" && f.dateStyle != "" {
			panic(Runtime(fn + ": dateStyle is not allowed in toLocaleTimeString"))
		}
		return f
	}
	needDefaults := true
	if kind != "time" && f.weekday+f.year+f.month+f.day != "" {
		needDefaults = false
	}
	if kind != "date" && f.hour+f.minute+f.second != "" {
		needDefaults = false
	}
	if needDefaults {
		if kind != "time" {
			f.year, f.month, f.day = "numeric", "numeric", "numeric"
		}
		if kind == "time" || kind == "all" {
			f.hour, f.minute, f.second = "numeric", "numeric", "numeric"
		}
	}
	return f
}

func (f *dateFormat) format(d *DateVal) string {
	if !d.valid {
		return "Invalid Date"
	}
	t := time.UnixMilli(d.ms).In(f.loc)
	if f.dateStyle != "" || f.timeStyle != "" {
		return f.formatStyles(t)
	}
	date := f.datePart(t)
	tm := f.timePart(t)
	switch {
	case date != "" && tm != "":
		return date + ", " + tm
	case date != "":
		return date
	}
	return tm
}

func (f *dateFormat) formatStyles(t time.Time) string {
	var date, tm string
	g := *f
	switch f.dateStyle {
	case "full":
		g.weekday, g.month, g.day, g.year = "long", "long", "numeric", "numeric"
	case "long":
		g.month, g.day, g.year = "long", "numeric", "numeric"
	case "medium":
		g.month, g.day, g.year = "short", "numeric", "numeric"
	case "short":
		g.month, g.day, g.year = "numeric", "numeric", "2-digit"
	}
	if f.dateStyle != "" {
		date = g.datePart(t)
	}
	if f.timeStyle != "" {
		g.hour, g.minute, g.second = "numeric", "2-digit", "2-digit"
		if f.timeStyle == "short" {
			g.second = ""
		}
		tm = g.timePart(t)
		switch f.timeStyle {
		case "long", "full":
			if !f.utcName {
				panic(Runtime("timeStyle long and full need a zone name; they are supported only for UTC (use \"short\" or \"medium\", or timeZone: \"UTC\")"))
			}
			if f.timeStyle == "long" {
				tm += " UTC"
			} else {
				tm += " Coordinated Universal Time"
			}
		}
	}
	switch {
	case date != "" && tm != "":
		if f.dateStyle == "full" || f.dateStyle == "long" {
			return date + " at " + tm
		}
		return date + ", " + tm
	case date != "":
		return date
	}
	return tm
}

func (f *dateFormat) datePart(t time.Time) string {
	var wd, y, d string
	if f.weekday != "" {
		wd = dayNames[t.Weekday()]
		switch f.weekday {
		case "short":
			wd = wd[:3]
		case "narrow":
			wd = wd[:1]
		}
	}
	if f.year != "" {
		y = fmt.Sprint(t.Year())
		if f.year == "2-digit" {
			y = fmt.Sprintf("%02d", ((t.Year()%100)+100)%100)
		}
	}
	if f.day != "" {
		d = fmt.Sprint(t.Day())
		if f.day == "2-digit" {
			d = fmt.Sprintf("%02d", t.Day())
		}
	}
	var body string
	switch f.month {
	case "long", "short", "narrow":
		m := monthNames[t.Month()-1]
		switch f.month {
		case "short":
			m = m[:3]
		case "narrow":
			m = m[:1]
		}
		body = m
		if d != "" {
			body += " " + d
		}
		if y != "" {
			if d != "" {
				body += ","
			}
			body += " " + y
		}
	case "numeric", "2-digit":
		m := fmt.Sprint(int(t.Month()))
		if f.month == "2-digit" {
			m = fmt.Sprintf("%02d", int(t.Month()))
		}
		parts := []string{m}
		if d != "" {
			parts = append(parts, d)
		}
		if y != "" {
			parts = append(parts, y)
		}
		body = strings.Join(parts, "/")
	default:
		var parts []string
		for _, p := range []string{d, y} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		body = strings.Join(parts, " ")
	}
	switch {
	case wd != "" && body != "":
		return wd + ", " + body
	case wd != "":
		return wd
	}
	return body
}

func (f *dateFormat) timePart(t time.Time) string {
	if f.hour == "" && f.minute == "" && f.second == "" {
		return ""
	}
	var parts []string
	suffix := ""
	if f.hour != "" {
		h := t.Hour()
		if f.hour12 {
			suffix = " AM"
			if h >= 12 {
				suffix = " PM"
			}
			h %= 12
			if h == 0 {
				h = 12
			}
			if f.hour == "2-digit" {
				parts = append(parts, fmt.Sprintf("%02d", h))
			} else {
				parts = append(parts, fmt.Sprint(h))
			}
		} else {
			// Node 24 (ICU) with hour12:false pads the hour and prints
			// midnight as 00 (checked: 00:05:00, 09:05).
			parts = append(parts, fmt.Sprintf("%02d", h))
		}
	}
	pad := f.hour != ""
	if f.minute != "" {
		if pad || f.second != "" || f.minute == "2-digit" {
			parts = append(parts, fmt.Sprintf("%02d", t.Minute()))
		} else {
			parts = append(parts, fmt.Sprint(t.Minute()))
		}
		pad = true
	}
	if f.second != "" {
		if pad || f.second == "2-digit" {
			parts = append(parts, fmt.Sprintf("%02d", t.Second()))
		} else {
			parts = append(parts, fmt.Sprint(t.Second()))
		}
	}
	return strings.Join(parts, ":") + suffix
}

// NewDateTimeFormat is Intl.DateTimeFormat(locale?, options?): an object with
// format(date).
func NewDateTimeFormat(local *time.Location) *FuncVal {
	return fnVal("Intl.DateTimeFormat", []string{"locale", "options"}, func(args []Value) Value {
		tooMany("Intl.DateTimeFormat", args, 2)
		f := parseDateFormat("Intl.DateTimeFormat", arg0(args), argAt(args, 1), "dtf", local)
		o := NewObject()
		o.Set("format", fnVal("format", []string{"date"}, func(a []Value) Value {
			tooMany("format", a, 1)
			switch x := arg0(a).(type) {
			case *DateVal:
				return Str(f.format(x))
			case *NumberVal:
				return Str(f.format(NewDate(int64(x.V), local)))
			case *NullVal:
				panic(Runtime("format: give a Date or a number of milliseconds"))
			}
			panic(TypeMismatch("format", "Date or number", arg0(a), ""))
		}))
		return o
	})
}
