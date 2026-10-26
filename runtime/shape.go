package runtime

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// InferType describes a value as a TypeScript type, inferred from the whole
// value, so a model can query a value it was shown only the start of.
//
//   - Array elements merge into one element type; a key missing from some
//     objects is optional (key?).
//   - A string field with few distinct values (an enum: status, priority) is a
//     union of the values seen. Ids, names and dates stay string.
//   - An object whose values share one composite type (groupBy output), or
//     one with many keys of one type (countBy output), is a Record.
//   - The result is capped at typeMaxBytes; past it, objects list their first
//     keys and "…N more".
func InferType(v Value) string {
	s := &shapeNode{}
	s.add(v, 0)
	t := s.render(0, 1<<30, false)
	if len(t) <= typeMaxBytes {
		return t
	}
	for fields := 8; fields >= 1; fields /= 2 {
		if t = s.render(0, fields, false); len(t) <= typeMaxBytes {
			return t
		}
	}
	return t[:typeMaxBytes] + "…"
}

const (
	typeMaxBytes   = 600
	typeMaxDepth   = 6
	enumMaxValues  = 5 // more distinct strings than this is not an enum
	recordMinKeys  = 3
	recordManyKeys = 8
)

type shapeNode struct {
	kinds   map[string]bool
	strs    map[string]bool // distinct strings, up to enumMaxValues+1
	nStrs   int
	elem    *shapeNode
	fields  map[string]*shapeNode
	order   []string
	seen    map[string]int
	objects int
}

func (s *shapeNode) kind(k string) {
	if s.kinds == nil {
		s.kinds = map[string]bool{}
	}
	s.kinds[k] = true
}

func (s *shapeNode) add(v Value, depth int) {
	switch x := v.(type) {
	case *StringVal:
		s.kind("string")
		s.nStrs++
		if s.strs == nil {
			s.strs = map[string]bool{}
		}
		if len(s.strs) <= enumMaxValues {
			s.strs[x.V] = true
		}
	case *NumberVal:
		s.kind("number")
	case *BoolVal:
		s.kind("boolean")
	case *NullVal:
		s.kind("null")
	case *FuncVal:
		s.kind("function")
	case *ArrayVal:
		s.kind("array")
		if s.elem == nil {
			s.elem = &shapeNode{}
		}
		if depth < typeMaxDepth {
			for _, e := range x.Elements {
				s.elem.add(e, depth+1)
			}
		}
	case *ObjectVal:
		s.kind("object")
		if s.fields == nil {
			s.fields, s.seen = map[string]*shapeNode{}, map[string]int{}
		}
		s.objects++
		if depth >= typeMaxDepth {
			return
		}
		for _, k := range x.Keys() {
			f := s.fields[k]
			if f == nil {
				f = &shapeNode{}
				s.fields[k] = f
				s.order = append(s.order, k)
			}
			s.seen[k]++
			fv, _ := x.Get(k)
			f.add(fv, depth+1)
		}
	default:
		s.kind("unknown")
	}
}

// render writes the type; maxFields caps the keys listed per object. plain
// renders every string as string, to compare structure (see renderRecord).
func (s *shapeNode) render(depth, maxFields int, plain bool) string {
	var parts []string
	for _, k := range []string{"string", "number", "boolean", "array", "object", "function", "unknown", "null"} {
		if !s.kinds[k] {
			continue
		}
		switch k {
		case "string":
			if plain {
				parts = append(parts, "string")
			} else {
				parts = append(parts, s.renderString())
			}
		case "array":
			el := "unknown"
			if s.elem != nil && len(s.elem.kinds) > 0 {
				el = s.elem.render(depth+1, maxFields, plain)
			}
			parts = append(parts, "Array<"+el+">")
		case "object":
			parts = append(parts, s.renderObject(depth, maxFields, plain))
		case "function":
			parts = append(parts, "Function")
		default:
			parts = append(parts, k)
		}
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, " | ")
}

// renderString is a union of the values seen when they look like an enum:
// few distinct values, each seen at least twice on average. One string seen
// once is just a string.
func (s *shapeNode) renderString() string {
	if n := len(s.strs); n == 0 || n > enumMaxValues || s.nStrs < 2*n {
		return "string"
	}
	lits := make([]string, 0, len(s.strs))
	for v := range s.strs {
		lits = append(lits, strconv.Quote(v))
	}
	sort.Strings(lits)
	return strings.Join(lits, " | ")
}

func (s *shapeNode) renderObject(depth, maxFields int, plain bool) string {
	if len(s.order) == 0 {
		return "{}"
	}
	if r := s.renderRecord(depth, maxFields, plain); r != "" {
		return r
	}
	var fs []string
	for i, k := range s.order {
		if i == maxFields {
			fs = append(fs, fmt.Sprintf("…%d more", len(s.order)-maxFields))
			break
		}
		opt := ""
		if s.seen[k] < s.objects {
			opt = "?"
		}
		key := k
		if !isIdentifier(k) {
			key = strconv.Quote(k)
		}
		fs = append(fs, key+opt+": "+s.fields[k].render(depth+1, maxFields, plain))
	}
	return "{" + strings.Join(fs, ", ") + "}"
}

// renderRecord collapses an object whose keys are data: every value has the
// same structure (string literals aside — groupBy(t => t.status) gives each
// group its own status), and either it is composite (groupBy → arrays) or
// there are many keys (countBy → numbers). A few keys are listed as a union.
func (s *shapeNode) renderRecord(depth, maxFields int, plain bool) string {
	if len(s.order) < recordMinKeys {
		return ""
	}
	first := s.fields[s.order[0]].render(depth+1, maxFields, true)
	for _, k := range s.order[1:] {
		if s.seen[k] != s.seen[s.order[0]] || s.fields[k].render(depth+1, maxFields, true) != first {
			return ""
		}
	}
	composite := s.fields[s.order[0]].kinds["array"] || s.fields[s.order[0]].kinds["object"]
	if !composite && len(s.order) <= recordManyKeys {
		return ""
	}
	// Merge every value, so the type covers all keys, not only the first.
	merged := &shapeNode{}
	for _, k := range s.order {
		merged.merge(s.fields[k])
	}
	key := "string"
	if len(s.order) <= recordManyKeys {
		keys := make([]string, len(s.order))
		for i, k := range s.order {
			keys[i] = strconv.Quote(k)
		}
		key = strings.Join(keys, " | ")
	}
	return "Record<" + key + ", " + merged.render(depth+1, maxFields, plain) + ">"
}

func (s *shapeNode) merge(o *shapeNode) {
	for k := range o.kinds {
		s.kind(k)
	}
	s.nStrs += o.nStrs
	for v := range o.strs {
		if s.strs == nil {
			s.strs = map[string]bool{}
		}
		if len(s.strs) <= enumMaxValues {
			s.strs[v] = true
		}
	}
	if o.elem != nil {
		if s.elem == nil {
			s.elem = &shapeNode{}
		}
		s.elem.merge(o.elem)
	}
	if o.fields != nil {
		if s.fields == nil {
			s.fields, s.seen = map[string]*shapeNode{}, map[string]int{}
		}
		s.objects += o.objects
		for _, k := range o.order {
			f := s.fields[k]
			if f == nil {
				f = &shapeNode{}
				s.fields[k] = f
				s.order = append(s.order, k)
			}
			s.seen[k] += o.seen[k]
			f.merge(o.fields[k])
		}
	}
}
