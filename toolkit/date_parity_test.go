package toolkit_test

import "testing"

// Expectations are Node 24.12 output with TZ=UTC (generated 2026-11-08),
// compared through toJson. mcpshell's local time zone is UTC by default.
func TestDateAndFormatNodeParity(t *testing.T) {
	cases := []struct{ code, want string }{
		{`new Date(0).toISOString()`, `"1970-01-01T00:00:00.000Z"`},
		{`new Date(2024, 0, 31).getMonth()`, `0`},
		{`new Date(2024, 1, 30).toISOString()`, `"2024-03-01T00:00:00.000Z"`},
		{`new Date("2024-02-30").getTime()`, `1709251200000`},
		{`new Date("2024-02-31T00:00:00Z").toISOString()`, `"2024-03-02T00:00:00.000Z"`},
		{`new Date("nope").getTime()`, `null`},
		{`String(new Date("nope"))`, `"Invalid Date"`},
		{`new Date(Date.UTC(2024, 11, 31, 23, 59, 59, 999)).toISOString()`, `"2024-12-31T23:59:59.999Z"`},
		{`new Date(2026, 10, 7).getDay()`, `6`},
		{`new Date(0).toString()`, `"Thu Jan 01 1970 00:00:00 GMT+0000 (Coordinated Universal Time)"`},
		{`new Date(0).toUTCString()`, `"Thu, 01 Jan 1970 00:00:00 GMT"`},
		{`new Date(0).toLocaleDateString("en-US")`, `"1/1/1970"`},
		{`new Date(0).toLocaleTimeString("en-US")`, `"12:00:00 AM"`},
		{`new Date(0).toLocaleString("en-US")`, `"1/1/1970, 12:00:00 AM"`},
		{`new Date(0).toLocaleTimeString("en-US", {hour12: false})`, `"00:00:00"`},
		{`new Date(0).toLocaleDateString("en-US", {year: "numeric", month: "long", day: "numeric"})`, `"January 1, 1970"`},
		{`new Date(0).toLocaleDateString("en-US", {weekday: "short", month: "short", day: "2-digit"})`, `"Thu, Jan 01"`},
		{`new Date("2024-03-10").getTime() - new Date("2024-03-09").getTime()`, `86400000`},
		{`new Date(2024, 0, 1, 25).toISOString()`, `"2024-01-02T01:00:00.000Z"`},
		{`new Date(1e15).getTime()`, `1000000000000000`},
		{`new Date(8.64e15 + 1).getTime()`, `null`},
		{`new Date("2024-01-15T10:30:00+02:00").toISOString()`, `"2024-01-15T08:30:00.000Z"`},
		{`new Date("2024-01-15 10:30").toISOString()`, `"2024-01-15T10:30:00.000Z"`},
		{`new Date("January 5, 2024").toISOString()`, `"2024-01-05T00:00:00.000Z"`},
		{`new Date("2024/01/05").toISOString()`, `"2024-01-05T00:00:00.000Z"`},
		{`(1234567.891).toLocaleString()`, `"1,234,567.891"`},
		{`(1234567.891).toLocaleString("en-US", {maximumFractionDigits: 1})`, `"1,234,567.9"`},
		{`(0.1 + 0.2).toLocaleString("en-US", {minimumFractionDigits: 2, maximumFractionDigits: 2})`, `"0.30"`},
		{`(1234.5).toLocaleString("en-US", {style: "currency", currency: "USD"})`, `"$1,234.50"`},
		{`(-1234.5).toLocaleString("en-US", {style: "currency", currency: "EUR"})`, `"-€1,234.50"`},
		{`(0.256).toLocaleString("en-US", {style: "percent"})`, `"26%"`},
		{`(0.2567).toLocaleString("en-US", {style: "percent", minimumFractionDigits: 1})`, `"25.7%"`},
		{`(1234567).toLocaleString("en-US", {notation: "compact"})`, `"1.2M"`},
		{`(1500).toLocaleString("en-US", {notation: "compact"})`, `"1.5K"`},
		{`(2.5).toLocaleString("en-US", {maximumFractionDigits: 0})`, `"3"`},
		{`(-2.5).toLocaleString("en-US", {maximumFractionDigits: 0})`, `"-3"`},
		{`(1.005).toLocaleString("en-US", {maximumFractionDigits: 2})`, `"1.01"`},
		{`(1e21).toLocaleString()`, `"1,000,000,000,000,000,000,000"`},
		{`(0).toLocaleString("en-US", {style: "currency", currency: "JPY"})`, `"¥0"`},
		{`new Intl.NumberFormat("en-US", {style: "currency", currency: "USD"}).format(99.999)`, `"$100.00"`},
		{`(123.456).toLocaleString("en-US", {minimumIntegerDigits: 5})`, `"00,123.456"`},
		{`(5).toLocaleString("en-US", {useGrouping: false, minimumFractionDigits: 3})`, `"5.000"`},
		{`new Date(Date.UTC(2024,0,1,0,5)).toLocaleTimeString("en-US", {hour12: false})`, `"00:05:00"`},
	}
	for _, c := range cases {
		got := run(t, "toJson("+c.code+")")
		if got != c.want {
			t.Errorf("%s\n  got  %s\n  node %s", c.code, got, c.want)
		}
	}
}
