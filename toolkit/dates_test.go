package toolkit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

type evalCase struct{ name, src, want string }

func runCases(t *testing.T, mk func() *runtime.Shell, cases []evalCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := mk().Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q): %v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("%s\n  got  %s\n  want %s", c.src, got, c.want)
			}
		})
	}
}

func runErrors(t *testing.T, mk func() *runtime.Shell, cases []evalCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := mk().Eval(c.src)
			if err == nil {
				t.Fatalf("eval(%q) = %s, want an error containing %q", c.src, v.Display(), c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("eval(%q) error = %q, want substring %q", c.src, err.Error(), c.want)
			}
		})
	}
}

func newCoreShell() *runtime.Shell { return toolkit.InstallCore(runtime.NewShell()) }

// The expectations are what Node prints with TZ=UTC.
func TestDateJSParity(t *testing.T) {
	runCases(t, newCoreShell, []evalCase{
		{"month is 0-based", `new Date(2026, 10, 7).toISOString()`, "2026-11-07T00:00:00.000Z"},
		{"getDay saturday", `new Date(2026, 10, 7).getDay()`, "6"},
		{"date-only iso is utc", `new Date("2026-11-07").toISOString()`, "2026-11-07T00:00:00.000Z"},
		{"iso with offset", `new Date("2026-11-07T10:00:00+02:00").toISOString()`, "2026-11-07T08:00:00.000Z"},
		{"iso with fraction", `new Date("2026-11-07T10:00:00.5Z").getMilliseconds()`, "500"},
		{"iso datetime without offset is local (utc)", `new Date("2026-11-07T10:00:00").toISOString()`, "2026-11-07T10:00:00.000Z"},
		{"epoch", `new Date(0).toISOString()`, "1970-01-01T00:00:00.000Z"},
		{"ms argument", `new Date(1794063845006).toISOString()`, "2026-11-07T15:04:05.006Z"},
		{"Date.UTC", `Date.UTC(2000, 0, 1)`, "946684800000"},
		{"Date.UTC end of year", `Date.UTC(2026, 11, 31, 23, 59, 59, 999)`, "1798761599999"},
		{"Date.UTC month default", `Date.UTC(2000)`, "946684800000"},
		{"two-digit year is 19xx", `new Date(99, 0, 1).getFullYear()`, "1999"},
		{"year 100 is itself", `new Date(100, 0, 1).getFullYear()`, "100"},
		{"month overflow carries", `new Date(2026, 12, 1).toISOString()`, "2027-01-01T00:00:00.000Z"},
		{"negative month carries", `new Date(2026, -1, 1).toISOString()`, "2025-12-01T00:00:00.000Z"},
		{"day 0 is last of previous month", `new Date(2026, 2, 0).getDate()`, "28"},
		{"feb 29 non-leap rolls", `new Date(2021, 1, 29).getMonth()`, "2"},
		{"leap day", `new Date(2024, 1, 29).getDate()`, "29"},
		{"max date", `new Date(8.64e15).toISOString()`, "+275760-09-13T00:00:00.000Z"},
		{"past max is invalid", `new Date(8.64e15 + 1).getTime()`, "NaN"},
		{"invalid string", `new Date("not a date").getTime()`, "NaN"},
		{"invalid prints", `String(new Date("nope"))`, "Invalid Date"},
		{"invalid getter is NaN", `new Date(NaN).getFullYear()`, "NaN"},
		{"invalid toString", `new Date(NaN).toString()`, "Invalid Date"},
		{"invalid toJSON is null", `new Date(NaN).toJSON()`, "null"},
		{"toString", `new Date(2026, 10, 7).toString()`, "Sat Nov 07 2026 00:00:00 GMT+0000 (Coordinated Universal Time)"},
		{"toDateString", `new Date(2026, 10, 7).toDateString()`, "Sat Nov 07 2026"},
		{"toUTCString", `new Date(2026, 10, 7).toUTCString()`, "Sat, 07 Nov 2026 00:00:00 GMT"},
		{"getTimezoneOffset", `new Date().getTimezoneOffset()`, "0"},
		{"getTime is valueOf", `new Date(5).getTime() == new Date(5).valueOf()`, "true"},
		{"getUTC getters", `let d = new Date(1794063845006); [d.getUTCHours(), d.getUTCMinutes(), d.getUTCSeconds(), d.getUTCMilliseconds(), d.getUTCDay()]`, "[15, 4, 5, 6, 6]"},
		{"subtraction is ms", `new Date(2026, 10, 7) - new Date(2026, 10, 6)`, "86400000"},
		{"comparison", `[new Date(1) < new Date(2), new Date(2) <= new Date(2), new Date(3) > new Date(2), new Date(2) >= new Date(3)]`, "[true, true, true, false]"},
		{"compare with number", `new Date(5) > 3`, "true"},
		{"invalid compares false", `[new Date(NaN) < new Date(1), new Date(NaN) >= new Date(1), new Date(1) > new Date(NaN)]`, "[false, false, false]"},
		{"plus is a string", `typeof (new Date(0) + 1)`, "string"},
		{"setMonth rolls day", `let d = new Date(2026, 0, 31); d.setMonth(1); d.toISOString()`, "2026-03-03T00:00:00.000Z"},
		{"setDate 0", `let d = new Date(2026, 2, 15); d.setDate(0); d.getDate()`, "28"},
		{"setHours with minutes", `let d = new Date(2026, 10, 7); d.setHours(13, 30); d.toISOString()`, "2026-11-07T13:30:00.000Z"},
		{"setFullYear returns the time", `new Date(0).setFullYear(1971)`, "31536000000"},
		{"setTime", `let d = new Date(0); d.setTime(1000); d.getTime()`, "1000"},
		{"setUTCDate", `let d = new Date(0); d.setUTCDate(15); d.getUTCDate()`, "15"},
		{"setter on invalid stays NaN", `let d = new Date(NaN); d.setHours(1); d.getTime()`, "NaN"},
		{"setFullYear revives invalid", `let d = new Date(NaN); d.setFullYear(2020); d.getFullYear()`, "2020"},
		{"copy constructor", `let a = new Date(5); let b = new Date(a); b.setTime(9); a.getTime()`, "5"},
		{"toJson", `toJson({d: new Date(0)})`, `{"d":"1970-01-01T00:00:00.000Z"}`},
		{"JSON.stringify", `JSON.stringify([new Date(0)])`, `["1970-01-01T00:00:00.000Z"]`},
		{"Number(d)", `Number(new Date(7))`, "7"},
		{"typeof", `typeof new Date()`, "object"},
		{"new without parens", `let d = new Date; d.getTime() > 0`, "true"},
		{"call chain on new", `new Date(2026, 10, 7).getFullYear()`, "2026"},
		{"Date.parse", `Date.parse("2026-11-07")`, "1794009600000"},
		{"Date.parse invalid", `Date.parse("zzz")`, "NaN"},
		{"Date.now is a number", `typeof Date.now()`, "number"},
		{"sort dates", `[new Date(3), new Date(1), new Date(2)].sort().map(d => d.getTime())`, "[1, 2, 3]"},
		{"month names", `new Date("Nov 7, 2026").toISOString()`, "2026-11-07T00:00:00.000Z"},
		{"long month name with time", `new Date("November 7, 2026 10:30:00").toISOString()`, "2026-11-07T10:30:00.000Z"},
		{"rfc 1123", `new Date("Sat, 07 Nov 2026 00:00:00 GMT").getTime()`, "1794009600000"},
		{"sql datetime is local", `new Date("2026-11-07 12:30:00").toISOString()`, "2026-11-07T12:30:00.000Z"},
		{"equality by time", `new Date(5) == new Date(5)`, "true"},
	})
}

func TestDateLocaleStrings(t *testing.T) {
	runCases(t, newCoreShell, []evalCase{
		{"toLocaleDateString", `new Date(2026, 10, 7).toLocaleDateString()`, "11/7/2026"},
		{"toLocaleDateString en-US", `new Date(2026, 10, 7).toLocaleDateString("en-US")`, "11/7/2026"},
		{"long", `new Date(2026, 10, 7).toLocaleDateString("en-US", {year: "numeric", month: "long", day: "numeric"})`, "November 7, 2026"},
		{"weekday long", `new Date(2026, 10, 7).toLocaleDateString("en-US", {weekday: "long", year: "numeric", month: "long", day: "numeric"})`, "Saturday, November 7, 2026"},
		{"short month day", `new Date(2026, 10, 7).toLocaleDateString("en-US", {month: "short", day: "numeric"})`, "Nov 7"},
		{"2-digit", `new Date(2026, 10, 7).toLocaleDateString("en-US", {year: "numeric", month: "2-digit", day: "2-digit"})`, "11/07/2026"},
		{"month year", `new Date(2026, 10, 7).toLocaleDateString("en-US", {month: "long", year: "numeric"})`, "November 2026"},
		{"month only", `new Date(2026, 10, 7).toLocaleDateString("en-US", {month: "long"})`, "November"},
		{"weekday only", `new Date(2026, 10, 7).toLocaleDateString("en-US", {weekday: "short"})`, "Sat"},
		{"dateStyle medium", `new Date(2026, 10, 7).toLocaleDateString("en-US", {dateStyle: "medium"})`, "Nov 7, 2026"},
		{"dateStyle full", `new Date(2026, 10, 7).toLocaleDateString("en-US", {dateStyle: "full"})`, "Saturday, November 7, 2026"},
		{"dateStyle short", `new Date(2026, 10, 7).toLocaleDateString("en-US", {dateStyle: "short"})`, "11/7/26"},
		{"time", `new Date(2026, 10, 7, 15, 4, 5).toLocaleTimeString("en-US")`, "3:04:05 PM"},
		{"midnight", `new Date(2026, 10, 7).toLocaleTimeString("en-US")`, "12:00:00 AM"},
		{"hour minute", `new Date(2026, 10, 7, 9, 5).toLocaleTimeString("en-US", {hour: "2-digit", minute: "2-digit"})`, "09:05 AM"},
		{"24 hour", `new Date(2026, 10, 7, 15, 4, 5).toLocaleTimeString("en-US", {hour12: false})`, "15:04:05"},
		{"toLocaleString", `new Date(2026, 10, 7, 15, 4, 5).toLocaleString("en-US")`, "11/7/2026, 3:04:05 PM"},
		{"timeZone", `new Date(Date.UTC(2026, 10, 7, 15, 4, 5)).toLocaleTimeString("en-US", {timeZone: "America/New_York"})`, "10:04:05 AM"},
		{"Intl.DateTimeFormat", `new Intl.DateTimeFormat("en-US", {month: "short", day: "numeric", year: "numeric"}).format(new Date(2026, 10, 7))`, "Nov 7, 2026"},
	})
	runErrors(t, newCoreShell, []evalCase{
		{"other locale", `new Date(0).toLocaleDateString("de-DE")`, `only "en-US"`},
		{"unknown option", `new Date(0).toLocaleDateString("en-US", {era: "long"})`, "era is not supported"},
		{"bad value", `new Date(0).toLocaleDateString("en-US", {month: "huge"})`, "option month must be one of"},
		{"zone name", `new Date(0).toLocaleDateString("en-US", {timeZone: "+01:00"})`, "timeZone"},
	})
}

func TestDateErrors(t *testing.T) {
	runErrors(t, newCoreShell, []evalCase{
		{"toISOString invalid", `new Date(NaN).toISOString()`, "Invalid Date"},
		{"too many args", `new Date(1, 2, 3, 4, 5, 6, 7, 8)`, "at most 7"},
		{"extra to now", `Date.now(1)`, "at most 0"},
		{"null arg", `new Date(null)`, "new Date"},
		{"unknown member", `new Date(0).getYear2()`, "no member"},
		{"setter extra", `new Date(0).setDate(1, 2)`, "at most 1"},
		{"new unknown", `new Foo()`, "Foo is not defined"},
		{"new Promise", `new Promise(x => x)`, "no timers"},
		{"new arrow function", `let f = () => 1; new f()`, "not a constructor"},
	})
}

func TestDateLocalZone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	runCases(t, func() *runtime.Shell { return toolkit.SetTimeZone(newCoreShell(), loc) }, []evalCase{
		{"local constructor", `new Date(2026, 6, 1).toISOString()`, "2026-06-30T22:00:00.000Z"},
		{"offset", `new Date(2026, 6, 1).getTimezoneOffset()`, "-120"},
		{"local hours", `new Date("2026-07-01T00:00:00Z").getHours()`, "2"},
		{"utc hours", `new Date("2026-07-01T00:00:00Z").getUTCHours()`, "0"},
		{"date-only is still utc", `new Date("2026-07-01").toISOString()`, "2026-07-01T00:00:00.000Z"},
		{"winter offset", `new Date(2026, 0, 1).getTimezoneOffset()`, "-60"},
	})
}

func TestDatesHelpers(t *testing.T) {
	runCases(t, newCoreShell, []evalCase{
		// month-end clamping
		{"clamp jan 31", `Dates.add("2026-01-31", {months: 1})`, "2026-02-28"},
		{"clamp leap", `Dates.add("2024-01-31", {months: 1})`, "2024-02-29"},
		{"clamp back", `Dates.add("2026-03-31", {months: -1})`, "2026-02-28"},
		{"clamp year", `Dates.add("2024-02-29", {years: 1})`, "2025-02-28"},
		{"12 months", `Dates.add("2026-05-31", {months: 12})`, "2027-05-31"},
		{"days and months", `Dates.add("2026-01-31", {months: 1, days: 1})`, "2026-03-01"},
		{"subtract", `Dates.subtract("2026-03-01", {days: 1})`, "2026-02-28"},
		{"weeks", `Dates.add("2026-11-07", {weeks: 2})`, "2026-11-21"},
		{"hours on sql string", `Dates.add("2026-11-07 23:00:00", {hours: 2})`, "2026-11-08 01:00:00"},
		{"iso string keeps iso", `Dates.add("2026-11-07T10:00:00.000Z", {minutes: 30})`, "2026-11-07T10:30:00.000Z"},
		{"date in date out", `Dates.add(new Date(Date.UTC(2026, 0, 31)), {months: 1}).toISOString()`, "2026-02-28T00:00:00.000Z"},
		{"ms in date out", `Dates.add(0, {days: 1}).getTime()`, "86400000"},
		// diff
		{"diff days", `Dates.diff("2026-12-25", "2026-11-07", "day")`, "48"},
		{"diff negative", `Dates.diff("2026-11-07", "2026-12-25", "day")`, "-48"},
		{"diff hours", `Dates.diff("2026-11-07 12:00:00", "2026-11-07 00:00:00", "hour")`, "12"},
		{"diff ms default", `Dates.diff(new Date(5), new Date(2))`, "3"},
		{"diff weeks truncates", `Dates.diff("2026-11-20", "2026-11-07", "week")`, "1"},
		{"diff months whole", `Dates.diff("2026-03-30", "2026-01-31", "month")`, "1"},
		{"diff months end", `Dates.diff("2026-03-31", "2026-02-28", "month")`, "1"},
		{"diff months negative", `Dates.diff("2026-01-31", "2026-03-30", "month")`, "-1"},
		{"diff years", `Dates.diff("2026-11-07", "2020-11-08", "year")`, "5"},
		// startOf / endOf
		{"startOf week monday", `Dates.startOf("2026-11-07", "week")`, "2026-11-02"},
		{"startOf week sunday", `Dates.startOf("2026-11-07", "week", 0)`, "2026-11-01"},
		{"startOf month", `Dates.startOf("2026-11-07 10:00:00", "month")`, "2026-11-01 00:00:00"},
		{"startOf quarter", `Dates.startOf("2026-11-07", "quarter")`, "2026-10-01"},
		{"startOf year", `Dates.startOf("2026-11-07", "year")`, "2026-01-01"},
		{"startOf day Date", `Dates.startOf(new Date(Date.UTC(2026, 10, 7, 15)), "day").toISOString()`, "2026-11-07T00:00:00.000Z"},
		{"endOf month day-only", `Dates.endOf("2026-02-10", "month")`, "2026-02-28"},
		{"endOf month Date", `Dates.endOf(new Date(Date.UTC(2026, 1, 10)), "month").toISOString()`, "2026-02-28T23:59:59.999Z"},
		{"endOf year iso", `Dates.endOf("2026-06-01T00:00:00.000Z", "year")`, "2026-12-31T23:59:59.999Z"},
		{"endOf quarter", `Dates.endOf("2026-11-07", "quarter")`, "2026-12-31"},
		{"endOf week", `Dates.endOf("2026-11-07", "week")`, "2026-11-08"},
		// ISO weeks across year boundaries
		{"2020-12-31 week", `[Dates.isoWeek("2020-12-31"), Dates.isoWeekYear("2020-12-31")]`, "[53, 2020]"},
		{"2021-01-01 week", `[Dates.isoWeek("2021-01-01"), Dates.isoWeekYear("2021-01-01")]`, "[53, 2020]"},
		{"2021-01-04 week", `[Dates.isoWeek("2021-01-04"), Dates.isoWeekYear("2021-01-04")]`, "[1, 2021]"},
		{"2018-12-31 week", `[Dates.isoWeek("2018-12-31"), Dates.isoWeekYear("2018-12-31")]`, "[1, 2019]"},
		{"2024-12-30 week", `[Dates.isoWeek("2024-12-30"), Dates.isoWeekYear("2024-12-30")]`, "[1, 2025]"},
		{"2026-01-01 week", `Dates.isoWeek("2026-01-01")`, "1"},
		{"week format", `Dates.format("2021-01-01", "YYYY-'W'ww")`, "2020-W53"},
		{"daysInMonth", `[Dates.daysInMonth("2024-02-10"), Dates.daysInMonth("2026-02-10"), Dates.daysInMonth("2026-12-01")]`, "[29, 28, 31]"},
		// business days
		{"week of business days", `Dates.businessDaysBetween("2026-11-02", "2026-11-09")`, "5"},
		{"with holiday", `Dates.businessDaysBetween("2026-11-02", "2026-11-09", {holidays: ["2026-11-04"]})`, "4"},
		{"holiday on weekend ignored", `Dates.businessDaysBetween("2026-11-02", "2026-11-09", {holidays: ["2026-11-07"]})`, "5"},
		{"fri to mon", `Dates.businessDaysBetween("2026-11-06", "2026-11-09")`, "1"},
		{"reverse is negative", `Dates.businessDaysBetween("2026-11-09", "2026-11-02")`, "-5"},
		{"four weeks", `Dates.businessDaysBetween("2026-11-02", "2026-11-30")`, "20"},
		{"same day", `Dates.businessDaysBetween("2026-11-02", "2026-11-02")`, "0"},
		{"add 1 from friday", `Dates.addBusinessDays("2026-11-06", 1)`, "2026-11-09"},
		{"add -1 from monday", `Dates.addBusinessDays("2026-11-09", -1)`, "2026-11-06"},
		{"add 5", `Dates.addBusinessDays("2026-11-02", 5)`, "2026-11-09"},
		{"add skips holiday", `Dates.addBusinessDays("2026-11-03", 1, {holidays: ["2026-11-04"]})`, "2026-11-05"},
		{"isBusinessDay", `[Dates.isBusinessDay("2026-11-06"), Dates.isBusinessDay("2026-11-07"), Dates.isBusinessDay("2026-11-06", {holidays: ["2026-11-06"]})]`, "[true, false, false]"},
		// range
		{"range days", `Dates.range("2026-11-01", "2026-11-03")`, `["2026-11-01", "2026-11-02", "2026-11-03"]`},
		{"range month clamp", `Dates.range("2026-01-31", "2026-04-30", "month")`, `["2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"]`},
		{"range step object", `Dates.range("2026-11-01", "2026-11-15", {days: 7})`, `["2026-11-01", "2026-11-08", "2026-11-15"]`},
		{"range number step", `Dates.range("2026-11-01", "2026-11-05", 2)`, `["2026-11-01", "2026-11-03", "2026-11-05"]`},
		{"range empty", `Dates.range("2026-11-05", "2026-11-01")`, `[]`},
		{"range dates", `Dates.range(new Date(0), new Date(2 * 86400000)).length`, "3"},
		// format
		{"format iso-like", `Dates.format(new Date(1794063845006), "yyyy-MM-dd HH:mm:ss.SSS")`, "2026-11-07 15:04:05.006"},
		{"format names", `Dates.format("2026-11-07", "EEE, d MMM yyyy")`, "Sat, 7 Nov 2026"},
		{"format long names", `Dates.format("2026-11-07", "EEEE MMMM dd, yy")`, "Saturday November 07, 26"},
		{"format 12 hour", `Dates.format("2026-11-07 15:04:05", "h:mm a")`, "3:04 PM"},
		{"format midnight 12 hour", `Dates.format("2026-11-07 00:04:05", "hh:mm a")`, "12:04 AM"},
		{"format quoted", `Dates.format("2026-11-07", "yyyy 'Q'Q")`, "2026 Q4"},
		{"format escaped quote", `Dates.format("2026-11-07", "yyyy''MM")`, "2026'11"},
		// parse
		{"parse dmy", `Dates.parse("07/11/2026", "dd/MM/yyyy").toISOString()`, "2026-11-07T00:00:00.000Z"},
		{"parse text", `Dates.parse("Nov 7, 2026 3:04 PM", "MMM d, yyyy h:mm a").toISOString()`, "2026-11-07T15:04:00.000Z"},
		{"parse compact", `Dates.parse("20261107", "yyyyMMdd").toISOString()`, "2026-11-07T00:00:00.000Z"},
		{"parse offset", `Dates.parse("2026-11-07 10:00 +02:00", "yyyy-MM-dd HH:mm XXX").toISOString()`, "2026-11-07T08:00:00.000Z"},
		{"parse ms", `Dates.parse("10:00:00.5", "HH:mm:ss.S").getMilliseconds()`, "500"},
		{"parse full month", `Dates.parse("November 7 2026", "MMMM d yyyy").getMonth()`, "10"},
		{"parse 12am", `Dates.parse("12:30 AM", "h:mm a").getHours()`, "0"},
		{"parse 2 digit year", `Dates.parse("11/07/26", "MM/dd/yy").getFullYear()`, "2026"},
		{"round trip", `Dates.format(Dates.parse("2026-11-07 15:04:05", "yyyy-MM-dd HH:mm:ss"), "yyyy-MM-dd'T'HH:mm:ss")`, "2026-11-07T15:04:05"},
	})
	runErrors(t, newCoreShell, []evalCase{
		{"parse mismatch", `Dates.parse("2026/11/07", "yyyy-MM-dd")`, "does not match"},
		{"parse out of range", `Dates.parse("2026-13-01", "yyyy-MM-dd")`, "out of range"},
		{"parse trailing", `Dates.parse("2026-11-07x", "yyyy-MM-dd")`, "unexpected"},
		{"bad letter", `Dates.format("2026-11-07", "yyyy-MM-dd z")`, `letter "z"`},
		{"bad unit", `Dates.startOf("2026-11-07", "fortnight")`, "not supported"},
		{"bad add key", `Dates.add("2026-11-07", {fortnights: 1})`, "unknown key"},
		{"fractional month", `Dates.add("2026-11-07", {months: 1.5})`, "whole number"},
		{"bad date", `Dates.add("yesterday", {days: 1})`, "not a date"},
		{"invalid Date", `Dates.add(new Date(NaN), {days: 1})`, "Invalid Date"},
		{"extra arg", `Dates.isoWeek("2026-01-01", 2)`, "at most 1"},
		{"holiday option typo", `Dates.isBusinessDay("2026-11-06", {holydays: []})`, "not supported"},
		{"range backwards step", `Dates.range("2026-11-01", "2026-11-05", -1)`, "forward"},
		{"range too long", `Dates.range("1000-01-01", "2026-01-01")`, "more than 10000"},
		{"range bad step", `Dates.range("2026-11-01", "2026-11-05", "fortnight")`, "not supported"},
	})
}
