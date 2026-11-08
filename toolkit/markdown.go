package toolkit

import (
	"fmt"
	"strings"

	. "github.com/iodesystems/mcpshell/runtime"
)

// toMarkdown renders rows as a GitHub markdown table.
func toMarkdown(args []Value) Value {
	const fn = "toMarkdown"
	maxArgs(fn, args, 2)
	rows, ok := arg(args, 0).(*ArrayVal)
	if !ok {
		panic(TypeMismatch(fn, "array of objects or array of arrays", arg(args, 0), "pipe an array in: rows |> toMarkdown()"))
	}
	var columns []string
	var header []string // arrays of arrays
	headerRow := false
	if o, isObj := arg(args, 1).(*ObjectVal); isObj {
		for _, k := range o.Keys() {
			v, _ := o.Get(k)
			switch k {
			case "columns":
				arr, isArr := v.(*ArrayVal)
				if !isArr {
					panic(Runtime(fn + ": columns must be an array of names"))
				}
				for _, e := range arr.Elements {
					columns = append(columns, e.Display())
				}
			case "header":
				switch h := v.(type) {
				case *BoolVal:
					headerRow = h.V
				case *ArrayVal:
					for _, e := range h.Elements {
						header = append(header, e.Display())
					}
				default:
					panic(Runtime(fn + ": header must be true (the first row) or an array of names"))
				}
			default:
				panic(Runtime(fmt.Sprintf("%s: option %q is not supported (supported: columns, header)", fn, k)))
			}
		}
	} else if _, isNull := arg(args, 1).(*NullVal); !isNull && len(args) > 1 {
		panic(Runtime(fn + ": the options must be an object such as {columns: [\"a\", \"b\"]}"))
	}

	// Arrays of arrays.
	if len(rows.Elements) > 0 {
		if _, isArr := rows.Elements[0].(*ArrayVal); isArr {
			return markdownArrays(fn, rows.Elements, header, headerRow, columns)
		}
	}
	if header != nil || headerRow {
		panic(Runtime(fn + ": header applies to an array of arrays; objects use their keys (pick with columns)"))
	}
	var objs []*ObjectVal
	seen := map[string]bool{}
	var keys []string
	for i, r := range rows.Elements {
		o, isObj := r.(*ObjectVal)
		if !isObj {
			panic(Runtime(fmt.Sprintf("%s: row %d is %s, not an object; rows must all be objects or all be arrays", fn, i, r.TypeName())))
		}
		objs = append(objs, o)
		for _, k := range o.Keys() {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	if columns == nil {
		columns = keys
	}
	if len(columns) == 0 {
		return Str("")
	}
	cells := make([][]Value, len(objs))
	for i, o := range objs {
		cells[i] = make([]Value, len(columns))
		for j, c := range columns {
			v, ok := o.Get(c)
			if !ok {
				v = Null
			}
			cells[i][j] = v
		}
	}
	return Str(renderTable(columns, cells))
}

func markdownArrays(fn string, rows []Value, header []string, headerRow bool, columns []string) Value {
	var cells [][]Value
	for i, r := range rows {
		a, ok := r.(*ArrayVal)
		if !ok {
			panic(Runtime(fmt.Sprintf("%s: row %d is %s, not an array; rows must all be objects or all be arrays", fn, i, r.TypeName())))
		}
		cells = append(cells, a.Elements)
	}
	switch {
	case header != nil:
	case headerRow:
		for _, v := range cells[0] {
			header = append(header, v.Display())
		}
		cells = cells[1:]
	case columns != nil:
		header = columns
	default:
		panic(Runtime(fn + ": an array of arrays needs a header: {header: true} uses the first row, {header: [\"a\", \"b\"]} names the columns"))
	}
	width := len(header)
	out := make([][]Value, len(cells))
	for i, row := range cells {
		if len(row) > width {
			panic(Runtime(fmt.Sprintf("%s: row %d has %d cells and the header has %d", fn, i, len(row), width)))
		}
		out[i] = append(append([]Value(nil), row...), make([]Value, width-len(row))...)
		for j := range out[i] {
			if out[i][j] == nil {
				out[i][j] = Null
			}
		}
	}
	return Str(renderTable(header, out))
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r\n", "<br>")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return strings.ReplaceAll(s, "\r", "<br>")
}

func mdCell(v Value) string {
	switch x := v.(type) {
	case *NullVal:
		return ""
	case *StringVal:
		return mdEscape(x.V)
	case *ArrayVal, *ObjectVal:
		return mdEscape(ToJSON(v))
	}
	return mdEscape(v.Display())
}

func renderTable(header []string, rows [][]Value) string {
	var b strings.Builder
	b.WriteString("|")
	for _, h := range header {
		b.WriteString(" " + mdEscape(h) + " |")
	}
	b.WriteString("\n|")
	for j := range header {
		numeric, any := true, false
		for _, r := range rows {
			switch r[j].(type) {
			case *NullVal:
			case *NumberVal:
				any = true
			default:
				numeric = false
			}
		}
		if numeric && any {
			b.WriteString(" ---: |")
		} else {
			b.WriteString(" --- |")
		}
	}
	for _, r := range rows {
		b.WriteString("\n|")
		for _, c := range r {
			if s := mdCell(c); s != "" {
				b.WriteString(" " + s + " |")
			} else {
				b.WriteString("  |")
			}
		}
	}
	return b.String()
}

const formatGuide = `Formats — numbers, currency, tables

NUMBERS (en-US only; an unsupported locale or option is an error that lists what is supported). Formatted from the exact decimal, never via float64.
  (1234567.891).toLocaleString()                       // "1,234,567.891"
  (0.1 + 0.2).toLocaleString("en-US", {minimumFractionDigits: 2, maximumFractionDigits: 2})   // "0.30"
  (1234.5).toLocaleString("en-US", {style: "currency", currency: "USD"})   // "$1,234.50"
  (0.256).toLocaleString("en-US", {style: "percent", maximumFractionDigits: 1})   // "25.6%"
  (1234567).toLocaleString("en-US", {notation: "compact"})   // "1.2M"
  new Intl.NumberFormat("en-US", {style: "currency", currency: "EUR"}).format(n)
  Options: style (decimal currency percent) currency currencyDisplay (symbol code) minimumFractionDigits maximumFractionDigits
    minimumIntegerDigits minimumSignificantDigits maximumSignificantDigits useGrouping notation (standard compact) signDisplay.
  Not supported: style "unit", other locales, notation scientific, currencyDisplay "name". A half rounds away from zero.
  Currency symbols: USD $, EUR €, GBP £, JPY ¥, CNY, INR, CAD, AUD, KRW, MXN, BRL, NZD, HKD; other codes print as "CHF 1,234.50".

DATES: Intl.DateTimeFormat(locale, opts).format(d) and d.toLocaleDateString(...) take the options in help("dates").

TABLES
  toMarkdown(rows, opts?)
    rows: array of objects (columns = keys in first-seen order) or array of arrays.
    opts.columns: names to pick, in order. opts.header: true (first row is the header) or an array of names, for arrays of arrays.
    Numbers align right, null is empty, "|" is escaped as \|, a newline is <br>, an object or array is its JSON.
  [{name: "ann", n: 3}, {name: "bo", n: 12}] |> toMarkdown()
  // | name | n |
  // | --- | ---: |
  // | ann | 3 |
  // | bo | 12 |`
