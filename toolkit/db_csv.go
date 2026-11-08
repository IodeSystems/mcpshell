package toolkit

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	. "github.com/iodesystems/mcpshell/runtime"
)

// CSV for the Db toolkit: parsing, per-column type inference, writing.
//
// A cell is one of: empty (null), integer, decimal, text. A column is
// INTEGER when every non-empty cell is an integer that fits int64; REAL when
// every one is a number; else TEXT. "00123" has a leading zero, so it is text.
// Dates are text. A column with decimals and an integer of more than 15 digits
// is TEXT, because REAL would round the integer.

type csvOpts struct {
	delim  rune
	header bool
	exact  map[string]bool
}

var (
	csvIntRE  = regexp.MustCompile(`^-?(0|[1-9]\d*)$`)
	csvRealRE = regexp.MustCompile(`^-?((0|[1-9]\d*)(\.\d+)?|\.\d+)([eE][+-]?\d+)?$`)
)

type csvClass int

const (
	csvNull csvClass = iota
	csvInt
	csvBigInt // integer of more than 15 digits that fits int64
	csvReal
	csvText
)

func classifyCell(s string) csvClass {
	t := strings.TrimSpace(s)
	switch {
	case t == "":
		return csvNull
	case csvIntRE.MatchString(t):
		if _, err := strconv.ParseInt(t, 10, 64); err != nil {
			return csvText
		}
		if len(strings.TrimPrefix(t, "-")) > 15 {
			return csvBigInt
		}
		return csvInt
	case csvRealRE.MatchString(t):
		if f, err := strconv.ParseFloat(t, 64); err != nil || f > 1.7e308 || f < -1.7e308 {
			return csvText
		}
		return csvReal
	}
	return csvText
}

// csvColumn accumulates what a column holds.
type csvColumn struct{ nInt, nBig, nReal, nText int }

func (c *csvColumn) add(k csvClass) {
	switch k {
	case csvInt:
		c.nInt++
	case csvBigInt:
		c.nBig++
	case csvReal:
		c.nReal++
	case csvText:
		c.nText++
	}
}

// sqlType is the SQL type of the column: "INTEGER", "REAL" or "TEXT".
func (c *csvColumn) sqlType() string {
	switch {
	case c.nText > 0, c.nReal > 0 && c.nBig > 0:
		return "TEXT"
	case c.nReal > 0:
		return "REAL"
	case c.nInt+c.nBig > 0:
		return "INTEGER"
	}
	return "TEXT"
}

func parseDelimiter(o *ObjectVal, def rune) rune {
	s, ok := optObjStr(o, "delimiter")
	if !ok {
		return def
	}
	switch s {
	case "tab", `\t`:
		s = "\t"
	}
	r := []rune(s)
	if len(r) != 1 || r[0] == '"' || r[0] == '\n' || r[0] == '\r' || r[0] == 0xFFFD {
		panic(Runtime(fmt.Sprintf("Db: delimiter must be one character other than a quote or newline, got %q", s)))
	}
	return r[0]
}

func optObjBoolDefault(o *ObjectVal, key string, def bool) bool {
	if o != nil {
		if v, ok := o.Get(key); ok {
			if b, ok := v.(*BoolVal); ok {
				return b.V
			}
		}
	}
	return def
}

// optExact reads opts.exact: an array of column names.
func optExact(cmd string, o *ObjectVal) map[string]bool {
	if o == nil {
		return nil
	}
	v, ok := o.Get("exact")
	if !ok || v == Null {
		return nil
	}
	arr, ok := v.(*ArrayVal)
	if !ok {
		panic(Runtime(cmd + ": exact must be an array of column names, e.g. {exact: [\"price\"]}"))
	}
	out := map[string]bool{}
	for _, e := range arr.Elements {
		s, ok := e.(*StringVal)
		if !ok {
			panic(Runtime(cmd + ": exact must list column names as strings"))
		}
		out[s.V] = true
	}
	return out
}

func readOptsCsv(cmd string, o *ObjectVal, defDelim rune) csvOpts {
	return csvOpts{
		delim:  parseDelimiter(o, defDelim),
		header: optObjBoolDefault(o, "header", true),
		exact:  optExact(cmd, o),
	}
}

// csvReader skips a UTF-8 byte order mark and reads records of any width.
func newCSVReader(r io.Reader, delim rune) *csv.Reader {
	br := bufio.NewReader(r)
	if b, err := br.Peek(3); err == nil && bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.Comma = delim
	cr.FieldsPerRecord = -1
	return cr
}

// csvHeader turns the first record into column names: trimmed, an empty one
// is colN (1-based), a repeat gets _2, _3, ...
func csvHeader(rec []string) []string {
	seen := map[string]int{}
	out := make([]string, len(rec))
	for i, h := range rec {
		h = strings.TrimSpace(h)
		if h == "" {
			h = "col" + strconv.Itoa(i+1)
		}
		base := h
		for seen[strings.ToLower(h)] > 0 {
			seen[strings.ToLower(base)]++
			h = base + "_" + strconv.Itoa(seen[strings.ToLower(base)])
		}
		seen[strings.ToLower(h)]++
		out[i] = h
	}
	return out
}

func csvReadErr(cmd string, cr *csv.Reader, err error) *ShellError {
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return Runtime(fmt.Sprintf("%s: CSV syntax error on line %d: %v\n\n  Hint: quote fields that contain the delimiter, a quote or a newline, and double a quote inside a quoted field", cmd, pe.StartLine, pe.Err))
	}
	return Runtime(cmd + ": " + err.Error())
}

// csvScan reads every record: the header (when opts.header) and each data row,
// padded with empty cells to the header width. A row wider than the header is
// an error. rowFn gets 1-based data row numbers.
func csvScan(cmd string, r io.Reader, o csvOpts, headerFn func(names []string), rowFn func(n int, rec []string)) {
	cr := newCSVReader(r, o.delim)
	var width int
	n := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(csvReadErr(cmd, cr, err))
		}
		if width == 0 {
			if o.header {
				names := csvHeader(rec)
				width = len(names)
				headerFn(names)
				continue
			}
			width = len(rec)
			names := make([]string, width)
			for i := range names {
				names[i] = "c" + strconv.Itoa(i+1)
			}
			headerFn(names)
		}
		if len(rec) > width {
			line, _ := cr.FieldPos(0)
			panic(Runtime(fmt.Sprintf("%s: line %d has %d fields but there are %d columns\n\n  Hint: check the delimiter (delimiter: \"\\t\" for TSV) and quoting", cmd, line, len(rec), width)))
		}
		for len(rec) < width {
			rec = append(rec, "")
		}
		n++
		rowFn(n, rec)
	}
	if width == 0 {
		panic(Runtime(cmd + ": the CSV is empty"))
	}
}

// csvInferTypes scans r once and returns the column names and their types.
func csvInferTypes(cmd string, r io.Reader, o csvOpts) (names []string, cols []*csvColumn) {
	csvScan(cmd, r, o,
		func(h []string) {
			names = h
			cols = make([]*csvColumn, len(h))
			for i := range cols {
				cols[i] = &csvColumn{}
			}
		},
		func(_ int, rec []string) {
			for i, c := range rec {
				cols[i].add(classifyCell(c))
			}
		})
	return
}

// csvCanonical writes a decimal cell of an exact column in canonical form.
func decimalCanonical(s string) (string, bool) {
	r, ok := parseDecimal(s)
	if !ok {
		return "", false
	}
	return ratDecimal(r)
}

// parseCsvValues builds mcpshell values from CSV text: numbers are exact
// numbers, empty cells null.
func parseCsvValues(text string, o csvOpts) Value {
	var names []string
	var rows [][]string
	csvScan("Db.parseCsv", strings.NewReader(text), o,
		func(h []string) { names = h },
		func(_ int, rec []string) { rows = append(rows, rec) })
	cols := make([]csvColumn, len(names))
	for _, rec := range rows {
		for i, c := range rec {
			cols[i].add(classifyCell(c))
		}
	}
	out := make([]Value, len(rows))
	for ri, rec := range rows {
		cells := make([]Value, len(rec))
		for i, c := range rec {
			k := classifyCell(c)
			switch {
			case k == csvNull:
				cells[i] = Null
			case cols[i].sqlType() == "TEXT":
				cells[i] = Str(c)
			default:
				n, _ := ParseNumber(strings.TrimSpace(c))
				cells[i] = n
			}
		}
		if !o.header {
			out[ri] = &ArrayVal{Elements: cells}
			continue
		}
		obj := NewObject()
		for i, v := range cells {
			obj.Set(names[i], v)
		}
		out[ri] = obj
	}
	return &ArrayVal{Elements: out}
}

// toCsv writes rows (objects or arrays) as CSV text.
func toCsvText(cmd string, rows *ArrayVal, o *ObjectVal) string {
	delim := parseDelimiter(o, ',')
	header := optObjBoolDefault(o, "header", true)
	var cols []string
	if o != nil {
		if v, ok := o.Get("columns"); ok {
			if arr, ok := v.(*ArrayVal); ok {
				for _, e := range arr.Elements {
					cols = append(cols, e.Display())
				}
			}
		}
	}
	objects := false
	if cols == nil {
		seen := map[string]bool{}
		for _, r := range rows.Elements {
			if ob, ok := r.(*ObjectVal); ok {
				objects = true
				for _, k := range ob.Keys() {
					if !seen[k] {
						seen[k] = true
						cols = append(cols, k)
					}
				}
			}
		}
	} else {
		objects = true
	}
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.Comma = delim
	cell := func(v Value) string {
		switch x := v.(type) {
		case nil:
			return ""
		case *NullVal:
			return ""
		case *StringVal:
			return x.V
		case *ObjectVal, *ArrayVal:
			return ToJSON(v)
		}
		return v.Display()
	}
	if header && objects {
		w.Write(cols)
	}
	for i, r := range rows.Elements {
		var rec []string
		switch x := r.(type) {
		case *ObjectVal:
			for _, c := range cols {
				v, _ := x.Get(c)
				rec = append(rec, cell(v))
			}
		case *ArrayVal:
			if objects {
				panic(Runtime(fmt.Sprintf("%s: row %d is an array but earlier rows are objects; use one shape", cmd, i)))
			}
			for _, v := range x.Elements {
				rec = append(rec, cell(v))
			}
		default:
			panic(Runtime(fmt.Sprintf("%s: row %d is a %s; rows must be objects or arrays", cmd, i, r.TypeName())))
		}
		w.Write(rec)
	}
	w.Flush()
	return sb.String()
}
