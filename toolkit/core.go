// Package toolkit provides the built-in mcpshell command toolkits.
package toolkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	. "github.com/iodesystems/mcpshell/runtime"
)

// InstallCore registers the core toolkit — data transformation, logic, strings,
// math, JSON, and utility commands — on the shell.
func InstallCore(sh *Shell) *Shell {
	sh.RegisterGuide("core", coreGuide)
	// Results print ±Infinity and NaN as JS does, so the names read back.
	sh.SetBuiltin("Infinity", Num(math.Inf(1)))
	sh.SetBuiltin("NaN", Num(math.NaN()))

	reg := func(name, sig, desc string, examples []string, fn NativeFn) {
		sh.Register(&CommandDef{Name: name, Signature: sig, Description: desc, Examples: examples, Fn: fn})
	}
	regH := func(name, sig, desc string, fn NativeFn) {
		sh.Register(&CommandDef{Name: name, Signature: sig, Description: desc, Hidden: true, Fn: fn})
	}

	// --- Collection transforms ---

	reg("map", "input: array|object, fn: (T, index?) => U",
		"applies fn to each element (with optional index); on objects maps values with (value, key)",
		[]string{`[1, 2, 3] |> map(x => x * 2)`, `map([1, 2, 3], x => x * 2)`},
		func(args []Value) Value {
			fn := requireFn("map", args, 1)
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				out := make([]Value, len(input.Elements))
				for i, el := range input.Elements {
					out[i] = Call(fn, []Value{el, Num(float64(i))})
				}
				return &ArrayVal{Elements: out}
			case *ObjectVal:
				obj := NewObject()
				for _, k := range input.Keys() {
					vv, _ := input.Get(k)
					obj.Set(k, Call(fn, []Value{vv, Str(k)}))
				}
				return obj
			default:
				panic(TypeMismatch("map", "array or object", input,
					"use |> to pipe into map, or |* to scatter elements"))
			}
		})

	reg("filter", "input: array|object, fn: (T, index?) => boolean",
		"keeps elements where fn is truthy; on objects filters entries with (value, key)",
		[]string{`[1, 2, 3, 4] |> filter(x => x > 2)`, `filter([1, 2, 3, 4], x => x > 2)`},
		func(args []Value) Value {
			fn := requireFn("filter", args, 1)
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				var out []Value
				for i, el := range input.Elements {
					if Call(fn, []Value{el, Num(float64(i))}).IsTruthy() {
						out = append(out, el)
					}
				}
				return &ArrayVal{Elements: out}
			case *ObjectVal:
				obj := NewObject()
				for _, k := range input.Keys() {
					vv, _ := input.Get(k)
					if Call(fn, []Value{vv, Str(k)}).IsTruthy() {
						obj.Set(k, vv)
					}
				}
				return obj
			default:
				panic(TypeMismatch("filter", "array or object", input,
					"use |> to pipe into filter, or |* to scatter elements"))
			}
		})

	reg("reduce", "input: array, fn: (acc, T, index?) => acc, init?: any",
		"folds array to single value; without init, starts from the first element as in JS (empty → 0)",
		[]string{`[1, 2, 3] |> reduce((sum, x) => sum + x)`},
		func(args []Value) Value {
			a := requireArray("reduce", arg(args, 0))
			return reduceElements(requireFn("reduce", args, 1), a.Elements, args, false)
		})

	regH("reduceRight", "input: array, fn: (acc, T, index?) => acc, init?: any", "reduce from the last element",
		func(args []Value) Value {
			a := requireArray("reduceRight", arg(args, 0))
			return reduceElements(requireFn("reduceRight", args, 1), a.Elements, args, true)
		})

	reg("sum", "input: array", "sums an array of numbers (empty → 0)",
		[]string{`[1, 2, 3] |> sum()`},
		func(args []Value) Value {
			a := requireArray("sum", arg(args, 0))
			var s float64
			for _, el := range a.Elements {
				s += elemNumber("sum", el)
			}
			return Num(s)
		})

	reg("avg", "input: array", "arithmetic mean of an array of numbers",
		[]string{`[1, 2, 3] |> avg()`},
		func(args []Value) Value {
			a := requireArray("avg", arg(args, 0))
			if len(a.Elements) == 0 {
				panic(Runtime("avg: empty array"))
			}
			var s float64
			for _, el := range a.Elements {
				s += elemNumber("avg", el)
			}
			return Num(s / float64(len(a.Elements)))
		})

	reg("product", "input: array", "product of an array of numbers (empty → 1)",
		[]string{`[2, 3, 4] |> product()`},
		func(args []Value) Value {
			a := requireArray("product", arg(args, 0))
			p := 1.0
			for _, el := range a.Elements {
				p *= elemNumber("product", el)
			}
			return Num(p)
		})

	reg("sort", "input: array, keyOrComparator?: string | (a, b) => number",
		`sorts; string key for objects, comparator fn, or "desc"/"asc" for direction`,
		[]string{`[3, 1, 2] |> sort()`, `users |> sort("name")`, `[3, 1, 2] |> sort("desc")`},
		func(args []Value) Value {
			a := requireArray("sort", arg(args, 0))
			out := append([]Value(nil), a.Elements...)
			second := argOpt(args, 1)
			if cmp, ok := second.(*FuncVal); ok {
				sort.SliceStable(out, func(i, j int) bool {
					if n, ok := Call(cmp, []Value{out[i], out[j]}).(*NumberVal); ok {
						return n.V < 0
					}
					return false
				})
				return &ArrayVal{Elements: out}
			}
			desc, asc, sortKey := false, false, ""
			if s, ok := second.(*StringVal); ok {
				lc := strings.ToLower(s.V)
				desc = lc == "desc" || lc == "descending"
				asc = lc == "asc" || lc == "ascending"
				if !desc && !asc {
					sortKey = s.V
				}
			}
			_ = asc
			sort.SliceStable(out, func(i, j int) bool {
				va, vb := out[i], out[j]
				if sortKey != "" {
					va = objField(va, sortKey)
					vb = objField(vb, sortKey)
				}
				c := compareShellValues(va, vb)
				if desc {
					return c > 0
				}
				return c < 0
			})
			return &ArrayVal{Elements: out}
		})

	reg("shuffle", "input: array", "randomly reorders array elements",
		[]string{`[1, 2, 3, 4, 5] |> shuffle()`},
		func(args []Value) Value {
			a := requireArray("shuffle", arg(args, 0))
			out := append([]Value(nil), a.Elements...)
			rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
			return &ArrayVal{Elements: out}
		})

	reg("reverse", "input: array|string", "reverses",
		[]string{`[1, 2, 3] |> reverse()`, `"hello" |> reverse()`},
		func(args []Value) Value {
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				out := make([]Value, len(input.Elements))
				for i, el := range input.Elements {
					out[len(out)-1-i] = el
				}
				return &ArrayVal{Elements: out}
			case *StringVal:
				return Str(reverseString(input.V))
			default:
				panic(TypeMismatch("reverse", "array or string", input, ""))
			}
		})

	reg("join", "input: array, sep?: string", "joins elements with separator",
		[]string{`["a", "b", "c"] |> join(", ")`},
		func(args []Value) Value {
			a := requireArray("join", arg(args, 0))
			sep := optString(args, 1, ",")
			parts := make([]string, len(a.Elements))
			for i, el := range a.Elements {
				parts[i] = el.Display()
			}
			return Str(strings.Join(parts, sep))
		})

	reg("split", "input: string, sep?: string|regex, limit?: number",
		"splits by separator, into at most limit parts; no separator → [input], as in JS",
		[]string{`"a,b,c" |> split(",")`, `split("hello world", " ")`, `"a,b,c".split(",", 2)`},
		func(args []Value) Value {
			s := requireString("split", arg(args, 0))
			var parts []string
			switch sep := argOpt(args, 1).(type) {
			case *RegexVal:
				parts = regexSplit(buildRegex(sep), s)
			case *StringVal:
				if sep.V == "" {
					parts = stringChars(s)
				} else {
					parts = strings.Split(s, sep.V)
				}
			default:
				parts = []string{s}
			}
			if n := optInt(args, 2); n != nil && *n >= 0 && *n < len(parts) {
				parts = parts[:*n]
			}
			return strArr(parts)
		})

	reg("lines", "input: string", "splits string into lines (trims trailing empty line)",
		[]string{`"line1\nline2\nline3" |> lines()`},
		func(args []Value) Value {
			parts := strings.Split(requireString("lines", arg(args, 0)), "\n")
			if len(parts) > 0 && parts[len(parts)-1] == "" {
				parts = parts[:len(parts)-1]
			}
			return strArr(parts)
		})

	regH("chars", "input: string", "splits string into array of characters",
		func(args []Value) Value {
			return strArr(stringChars(requireString("chars", arg(args, 0))))
		})

	reg("flat", "input: array, depth?: number = 1", "flattens depth levels (Infinity: all)",
		[]string{`[[1, 2], [3, 4]] |> flat()`, `[1, [2, [3]]] |> flat(Infinity)`},
		func(args []Value) Value {
			a := requireArray("flat", arg(args, 0))
			depth := 1.0
			if n, ok := argOpt(args, 1).(*NumberVal); ok {
				depth = n.V
			}
			return &ArrayVal{Elements: flatten(a.Elements, depth)}
		})

	reg("unique", "input: array", "deduplicates",
		[]string{`[1, 2, 2, 3, 1] |> unique()`},
		func(args []Value) Value {
			a := requireArray("unique", arg(args, 0))
			var out []Value
			for _, el := range a.Elements {
				dup := false
				for _, seen := range out {
					if Equal(seen, el) {
						dup = true
						break
					}
				}
				if !dup {
					out = append(out, el)
				}
			}
			return &ArrayVal{Elements: out}
		})

	reg("len", "input: array|string|object", "length",
		[]string{`len([1, 2, 3])`, `"hello" |> len()`},
		func(args []Value) Value {
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				return Num(float64(len(input.Elements)))
			case *StringVal:
				return Num(float64(len([]rune(input.V))))
			case *ObjectVal:
				return Num(float64(input.Len()))
			default:
				return Num(0)
			}
		})

	reg("limit", "input: array|string, n: number", "first n elements",
		[]string{`[1, 2, 3, 4, 5] |> limit(3)`},
		func(args []Value) Value {
			n := int(requireNumber("limit", args, 1))
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				return &ArrayVal{Elements: takeN(input.Elements, n)}
			case *StringVal:
				return Str(string(takeRunes([]rune(input.V), n)))
			default:
				panic(TypeMismatch("limit", "array or string", input, ""))
			}
		})

	reg("skip", "input: array|string, n: number", "drops first n elements",
		[]string{`[1, 2, 3, 4, 5] |> skip(2)`},
		func(args []Value) Value {
			n := int(requireNumber("skip", args, 1))
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				return &ArrayVal{Elements: dropN(input.Elements, n)}
			case *StringVal:
				r := []rune(input.V)
				return Str(string(dropRunes(r, n)))
			default:
				panic(TypeMismatch("skip", "array or string", input, ""))
			}
		})

	lastFn := func(name string) NativeFn {
		return func(args []Value) Value {
			nPtr := optInt(args, 1)
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				if nPtr == nil {
					if len(input.Elements) == 0 {
						return Null
					}
					return input.Elements[len(input.Elements)-1]
				}
				return &ArrayVal{Elements: takeLastN(input.Elements, *nPtr)}
			case *StringVal:
				r := []rune(input.V)
				if nPtr == nil {
					if len(r) == 0 {
						return Null
					}
					return Str(string(r[len(r)-1]))
				}
				return Str(string(takeLastRunes(r, *nPtr)))
			default:
				panic(TypeMismatch(name, "array or string", input, ""))
			}
		}
	}
	reg("last", "input: array|string, n?: number", "last n elements (default 1 element, not wrapped)",
		[]string{`[1, 2, 3, 4, 5] |> last(2)`, `[1, 2, 3] |> last()`}, lastFn("last"))

	firstFn := func(name string) NativeFn {
		return func(args []Value) Value {
			nPtr := optInt(args, 1)
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				if nPtr == nil {
					if len(input.Elements) == 0 {
						return Null
					}
					return input.Elements[0]
				}
				return &ArrayVal{Elements: takeN(input.Elements, *nPtr)}
			case *StringVal:
				r := []rune(input.V)
				if nPtr == nil {
					if len(r) == 0 {
						return Null
					}
					return Str(string(r[0]))
				}
				return Str(string(takeRunes(r, *nPtr)))
			default:
				panic(TypeMismatch(name, "array or string", input, ""))
			}
		}
	}
	regH("first", "input: array|string, n?: number", "first n elements (default 1 element, not wrapped)", firstFn("first"))
	regH("head", "input: array|string, n?: number", "first n elements (alias for first)", firstFn("head"))

	// --- Objects ---

	reg("keys", "input: object", "object keys",
		[]string{`{a: 1, b: 2} |> keys()`},
		func(args []Value) Value {
			if a, ok := arg(args, 0).(*ArrayVal); ok {
				return indexArray(len(a.Elements))
			}
			o := requireObject("keys", arg(args, 0))
			return strArr(o.Keys())
		})

	reg("values", "input: object", "object values",
		[]string{`{a: 1, b: 2} |> values()`},
		func(args []Value) Value {
			if a, ok := arg(args, 0).(*ArrayVal); ok {
				return &ArrayVal{Elements: append([]Value(nil), a.Elements...)}
			}
			o := requireObject("values", arg(args, 0))
			out := make([]Value, 0, o.Len())
			for _, k := range o.Keys() {
				v, _ := o.Get(k)
				out = append(out, v)
			}
			return &ArrayVal{Elements: out}
		})

	reg("entries", "input: object", "object → [[key, value], ...] (JS-compatible)",
		[]string{`{a: 1, b: 2} |> entries()`},
		func(args []Value) Value {
			o := requireObject("entries", arg(args, 0))
			out := make([]Value, 0, o.Len())
			for _, k := range o.Keys() {
				v, _ := o.Get(k)
				out = append(out, Arr(Str(k), v))
			}
			return &ArrayVal{Elements: out}
		})

	reg("fromEntries", "input: array", "[[key, value]] or [{key, value}] → object",
		[]string{`[["a", 1], ["b", 2]] |> fromEntries()`},
		func(args []Value) Value {
			a := requireArray("fromEntries", arg(args, 0))
			obj := NewObject()
			for _, elem := range a.Elements {
				switch e := elem.(type) {
				case *ArrayVal:
					if len(e.Elements) < 2 {
						panic(Runtime(fmt.Sprintf("fromEntries: array entry must have at least 2 elements, got %d", len(e.Elements))))
					}
					obj.Set(keyString(e.Elements[0]), e.Elements[1])
				case *ObjectVal:
					k, ok := e.Get("key")
					ks, isStr := k.(*StringVal)
					if !ok || !isStr {
						panic(Runtime("fromEntries: object entry must have a 'key' field"))
					}
					val, has := e.Get("value")
					if !has {
						val = Null
					}
					obj.Set(ks.V, val)
				default:
					panic(TypeMismatch("fromEntries", "array or {key, value} object", elem, ""))
				}
			}
			return obj
		})

	reg("countBy", "input: array, fn?: (T) => string", "→ {key: count}; fn defaults to identity",
		[]string{`["a", "b", "a", "c", "a"] |> countBy()`},
		func(args []Value) Value {
			a := requireArray("countBy", arg(args, 0))
			fn, _ := argOpt(args, 1).(*FuncVal)
			counts := NewObject()
			for _, elem := range a.Elements {
				key := elem
				if fn != nil {
					key = Call(fn, []Value{elem})
				}
				ks := keyString(key)
				prev := 0.0
				if c, ok := counts.Get(ks); ok {
					prev = c.(*NumberVal).V
				}
				counts.Set(ks, Num(prev+1))
			}
			return counts
		})

	reg("range", "end: number | start: number, end: number", "[start, end) integer array (single arg: range(0, end))",
		[]string{`range(5)`, `range(0, 5)`},
		func(args []Value) Value {
			start, end := 0, 0
			if len(args) == 1 {
				end = int(requireNumber("range", args, 0))
			} else {
				start = int(requireNumber("range", args, 0))
				end = int(requireNumber("range", args, 1))
			}
			var out []Value
			for i := start; i < end; i++ {
				out = append(out, Num(float64(i)))
			}
			return &ArrayVal{Elements: out}
		})

	reg("find", "input: array, fn: (T) => boolean", "first match or null",
		[]string{`[1, 2, 3, 4] |> find(x => x > 2)`},
		func(args []Value) Value {
			a := requireArray("find", arg(args, 0))
			fn := requireFn("find", args, 1)
			for _, el := range a.Elements {
				if Call(fn, []Value{el}).IsTruthy() {
					return el
				}
			}
			return Null
		})

	// contains has no fromIndex, so a method reference works as a callback:
	// filter(allowed.contains) gets (element, index). includes is the JS form
	// with fromIndex, and has JS's quirk with callbacks too.
	contains := func(name string) NativeFn {
		return func(args []Value) Value {
			target := arg(args, 1)
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				els := input.Elements
				if name == "includes" {
					els = els[elemFrom(args, 2, len(els)):]
				}
				for _, el := range els {
					if valuesEqual(el, target) {
						return Bln(true)
					}
				}
				return Bln(false)
			case *StringVal:
				sub, ok := target.(*StringVal)
				if !ok {
					panic(TypeMismatch(name, "string", target, "use a string argument with string input"))
				}
				text := input.V
				if name == "includes" {
					r := []rune(text)
					text = string(r[elemFrom(args, 2, len(r)):])
				}
				return Bln(strings.Contains(text, sub.V))
			default:
				return Bln(false)
			}
		}
	}
	reg("contains", "input: array|string, value: any", "membership test",
		[]string{`[1, 2, 3] |> contains(2)`, `"hello world" |> contains("world")`}, contains("contains"))
	regH("includes", "input: array|string, value: any, fromIndex?: number",
		"membership test from fromIndex on (JS includes)", contains("includes"))

	reg("groupBy", "input: array, fn: (T) => string", "→ {key: elements[]}",
		[]string{`[1, 2, 3, 4, 5] |> groupBy(x => x % 2 == 0 ? "even" : "odd")`},
		func(args []Value) Value {
			a := requireArray("groupBy", arg(args, 0))
			fn := requireFn("groupBy", args, 1)
			groups := NewObject()
			for _, elem := range a.Elements {
				ks := keyString(Call(fn, []Value{elem}))
				if existing, ok := groups.Get(ks); ok {
					arr := existing.(*ArrayVal)
					arr.Elements = append(arr.Elements, elem)
				} else {
					groups.Set(ks, &ArrayVal{Elements: []Value{elem}})
				}
			}
			return groups
		})

	reg("zip", "a: array, b: array", "pairs elements from two arrays",
		[]string{`zip([1, 2, 3], ["a", "b", "c"])`},
		func(args []Value) Value {
			a := requireArray("zip", arg(args, 0))
			b := requireArray("zip", arg(args, 1))
			n := min(len(a.Elements), len(b.Elements))
			out := make([]Value, n)
			for i := range n {
				out[i] = Arr(a.Elements[i], b.Elements[i])
			}
			return &ArrayVal{Elements: out}
		})

	reg("chunk", "input: array, size: number", "splits into sub-arrays of size n",
		[]string{`[1, 2, 3, 4, 5] |> chunk(2)`},
		func(args []Value) Value {
			a := requireArray("chunk", arg(args, 0))
			size := int(requireNumber("chunk", args, 1))
			if size <= 0 {
				panic(Runtime(fmt.Sprintf("chunk: size must be positive, got %d", size)))
			}
			var out []Value
			for i := 0; i < len(a.Elements); i += size {
				end := min(i+size, len(a.Elements))
				out = append(out, &ArrayVal{Elements: append([]Value(nil), a.Elements[i:end]...)})
			}
			return &ArrayVal{Elements: out}
		})

	// --- Conversions ---

	reg("str", "value: any", "→ string", []string{`str(42)`},
		func(args []Value) Value { return Str(arg(args, 0).Display()) })

	reg("num", "value: any", "→ number", []string{`num("42")`},
		func(args []Value) Value {
			switch v := arg(args, 0).(type) {
			case *NumberVal:
				return PlainNumber(v) // Number(1n) is 1
			case *StringVal:
				if n, ok := ParseNumber(strings.TrimSpace(v.V)); ok {
					return n // exact for big integers and decimals
				}
				panic(TypeMismatch("num", "numeric string", v, ""))
			case *BoolVal:
				if v.V {
					return Num(1)
				}
				return Num(0)
			default:
				panic(TypeMismatch("num", "string, number, or boolean", v, ""))
			}
		})

	reg("bool", "value: any", "→ boolean", []string{`bool(1)`},
		func(args []Value) Value { return Bln(arg(args, 0).IsTruthy()) })

	reg("load", "...names: string", "installs loadable toolkits by name, or evaluates a .mcpshell file by path (file toolkit only); several names load in order",
		[]string{`load("db")`, `load("db", "solver")`, `load("lib/helpers.mcpshell")`},
		func(args []Value) Value {
			return loadEach(sh, "load", args, func(name string) Value {
				if sh.IsLoadableName(name) {
					summary, err := sh.Load(name)
					if err != nil {
						panic(Runtime(err.Error()))
					}
					return Str(summary)
				}
				return sh.LoadFile(name)
			})
		})

	reg("unload", "...names: string", "removes loaded toolkits and their commands",
		[]string{`unload("db")`, `unload("db", "solver")`},
		func(args []Value) Value {
			return loadEach(sh, "unload", args, func(name string) Value {
				msg, err := sh.Unload(name)
				if err != nil {
					panic(Runtime(err.Error()))
				}
				return Str(msg)
			})
		})

	reg("toArray", "value: any, mapFn?: (v, i) => T", "array-like→array; supports {length: n} and optional mapFn",
		[]string{`toArray(null)`, `toArray(5)`},
		func(args []Value) Value {
			mapFn, _ := argOpt(args, 1).(*FuncVal)
			var base *ArrayVal
			switch v := arg(args, 0).(type) {
			case *NullVal:
				base = &ArrayVal{}
			case *ArrayVal:
				base = v
			case *ObjectVal:
				if l, ok := v.Get("length"); ok {
					if n, ok := l.(*NumberVal); ok {
						elems := make([]Value, int(n.V))
						for i := range elems {
							elems[i] = Null
						}
						base = &ArrayVal{Elements: elems}
						break
					}
				}
				base = &ArrayVal{Elements: []Value{v}}
			default:
				base = &ArrayVal{Elements: []Value{v}}
			}
			if mapFn == nil {
				return base
			}
			out := make([]Value, len(base.Elements))
			for i, el := range base.Elements {
				out[i] = Call(mapFn, []Value{el, Num(float64(i))})
			}
			return &ArrayVal{Elements: out}
		})

	reg("isArray", "value: any", "true if value is an array", []string{`isArray([1, 2])`},
		func(args []Value) Value {
			_, ok := arg(args, 0).(*ArrayVal)
			return Bln(ok)
		})

	reg("typeof", "value: any", "type name", []string{`typeof(42)`},
		func(args []Value) Value { return Str(arg(args, 0).TypeName()) })

	// --- Utilities ---

	reg("print", "...values: any", "prints, returns last", []string{`print("hello", "world")`},
		func(args []Value) Value {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.Display()
			}
			fmt.Println(strings.Join(parts, " "))
			if len(args) > 0 {
				return args[len(args)-1]
			}
			return Null
		})

	reg("fail", "msg?: string", "throws error", []string{`fail("something went wrong")`},
		func(args []Value) Value {
			msg := "error"
			if s, ok := arg(args, 0).(*StringVal); ok {
				msg = s.V
			}
			panic(Runtime("fail: " + msg))
		})

	reg("assert", "message: string, condition: any", "fails if condition is falsy",
		[]string{`assert("must be positive", x > 0)`},
		func(args []Value) Value {
			msg, ok := arg(args, 0).(*StringVal)
			if !ok {
				panic(WrongArguments("assert", "message: string, condition: any", args,
					`assert("must be positive", x > 0)`))
			}
			if !arg(args, 1).IsTruthy() {
				panic(Runtime("Assertion failed: " + msg.V))
			}
			return Null
		})

	// --- String operations ---

	reg("trim", "input: string", "strips whitespace", []string{`"  hello  " |> trim()`},
		func(args []Value) Value { return Str(strings.TrimSpace(requireString("trim", arg(args, 0)))) })

	reg("lower", "input: string", "→ lowercase", []string{`"Hello" |> lower()`},
		func(args []Value) Value { return Str(strings.ToLower(requireString("lower", arg(args, 0)))) })

	reg("upper", "input: string", "→ uppercase", []string{`"hello" |> upper()`},
		func(args []Value) Value { return Str(strings.ToUpper(requireString("upper", arg(args, 0)))) })

	reg("replace", "input: string, old: string|regex, new: string",
		"replaces the first occurrence, as in JS; a /g regex replaces all (regex replacement takes $1 backrefs). replaceAll replaces all",
		[]string{`"a-b-c" |> replace("-", "+")`, `"a-b-c" |> replace(/-/g, "+")`, `"a-b-c" |> replaceAll("-", "+")`},
		func(args []Value) Value { return Str(replaceString("replace", args, false)) })

	reg("replaceAll", "input: string, old: string|regex, new: string",
		"replaces every occurrence (regex replacement takes $1 backrefs)",
		[]string{`"a-b-c" |> replaceAll("-", "+")`},
		func(args []Value) Value { return Str(replaceString("replaceAll", args, true)) })

	reg("startsWith", "input: string, prefix: string, position?: number", "prefix test, at position (default 0)",
		[]string{`"hello" |> startsWith("hel")`, `"hello".startsWith("ll", 2)`},
		func(args []Value) Value {
			input := requireString("startsWith", arg(args, 0))
			prefix, ok := arg(args, 1).(*StringVal)
			if !ok {
				panic(WrongArguments("startsWith", "input: string, prefix: string", args, ""))
			}
			r := []rune(input)
			return Bln(strings.HasPrefix(string(r[clampIdx(optInt(args, 2), 0, len(r)):]), prefix.V))
		})

	reg("endsWith", "input: string, suffix: string, endPosition?: number", "suffix test, on the first endPosition characters (default all)",
		[]string{`"hello" |> endsWith("llo")`, `"hello".endsWith("ell", 4)`},
		func(args []Value) Value {
			input := requireString("endsWith", arg(args, 0))
			suffix, ok := arg(args, 1).(*StringVal)
			if !ok {
				panic(WrongArguments("endsWith", "input: string, suffix: string", args, ""))
			}
			r := []rune(input)
			return Bln(strings.HasSuffix(string(r[:clampIdx(optInt(args, 2), len(r), len(r))]), suffix.V))
		})

	reg("substring", "input: string, start?: number, end?: number",
		"slice string, as JS substring: negative → 0, start > end swaps",
		[]string{`"hello" |> substring(1, 4)`},
		func(args []Value) Value {
			r := []rune(requireString("substring", arg(args, 0)))
			if len(args) > 1 && args[1] != Null {
				requireNumber("substring", args, 1)
			}
			start := 0
			if s := optInt(args, 1); s != nil {
				start = clamp(*s, 0, len(r))
			}
			end := len(r)
			if e := optInt(args, 2); e != nil {
				end = clamp(*e, 0, len(r))
			}
			if start > end {
				start, end = end, start
			}
			return Str(string(r[start:end]))
		})

	regH("substr", "input: string, start: number, length?: number",
		"slice string, as JS substr: negative start counts from the end, length characters",
		func(args []Value) Value {
			r := []rune(requireString("substr", arg(args, 0)))
			start := int(requireNumber("substr", args, 1))
			if start < 0 {
				start = max(len(r)+start, 0)
			}
			start = min(start, len(r))
			end := len(r)
			if n := optInt(args, 2); n != nil {
				end = start + clamp(*n, 0, len(r)-start)
			}
			return Str(string(r[start:end]))
		})

	reg("padStart", "input: string, length: number, fill?: string", "pads start to target length",
		[]string{`"42" |> padStart(5, "0")`},
		func(args []Value) Value { return Str(pad(args, "padStart", true)) })

	reg("padEnd", "input: string, length: number, fill?: string", "pads end to target length",
		[]string{`"hi" |> padEnd(5, ".")`},
		func(args []Value) Value { return Str(pad(args, "padEnd", false)) })

	reg("match", "input: string, pattern: string|regex",
		"JS-compatible regex match: non-global → [fullMatch, group1, ...] or null; global → [match1, ...]",
		[]string{`"abc123def456" |> match("[0-9]+")`},
		func(args []Value) Value {
			s := requireString("match", arg(args, 0))
			switch pat := arg(args, 1).(type) {
			case *RegexVal:
				re := buildRegex(pat)
				if strings.ContainsRune(pat.Flags, 'g') {
					return strArr(regexFindAll(re, s))
				}
				m := regexFindSubmatch(re, s)
				if m == nil {
					return Null
				}
				return strArr(m)
			case *StringVal:
				m := regexFindSubmatch(compileRegex(pat.V), s)
				if m == nil {
					return Null
				}
				return strArr(m)
			default:
				panic(WrongArguments("match", "input: string, pattern: string|regex", args, ""))
			}
		})

	reg("test", "input: string, pattern: string|regex", "boolean regex test",
		[]string{`"hello123" |> test(/[0-9]+/)`},
		func(args []Value) Value {
			s := requireString("test", arg(args, 0))
			switch pat := arg(args, 1).(type) {
			case *RegexVal:
				return Bln(regexMatch(buildRegex(pat), s))
			case *StringVal:
				return Bln(regexMatch(compileRegex(pat.V), s))
			default:
				panic(WrongArguments("test", "input: string, pattern: string|regex", args, ""))
			}
		})

	reg("charAt", "input: string, index: number", "character at index", []string{`"hello" |> charAt(1)`},
		func(args []Value) Value {
			r := []rune(requireString("charAt", arg(args, 0)))
			idx := int(requireNumber("charAt", args, 1))
			if idx >= 0 && idx < len(r) {
				return Str(string(r[idx]))
			}
			return Str("")
		})

	regH("charCodeAt", "input: string, index: number", "character code at index",
		func(args []Value) Value {
			r := []rune(requireString("charCodeAt", arg(args, 0)))
			idx := int(requireNumber("charCodeAt", args, 1))
			if idx >= 0 && idx < len(r) {
				return Num(float64(r[idx]))
			}
			return Num(math.NaN())
		})

	regH("codePointAt", "input: string, index: number", "code point at index",
		func(args []Value) Value {
			r := []rune(requireString("codePointAt", arg(args, 0)))
			idx := int(requireNumber("codePointAt", args, 1))
			if idx >= 0 && idx < len(r) {
				return Num(float64(r[idx]))
			}
			return Num(math.NaN())
		})

	regH("repeat", "input: string, count: number", "repeats string n times",
		func(args []Value) Value {
			s := requireString("repeat", arg(args, 0))
			count := int(requireNumber("repeat", args, 1))
			if count < 0 {
				panic(Runtime("repeat: count must be non-negative"))
			}
			return Str(strings.Repeat(s, count))
		})

	regH("trimStart", "input: string", "trims leading whitespace",
		func(args []Value) Value {
			return Str(strings.TrimLeft(requireString("trimStart", arg(args, 0)), " \t\n\r\f\v"))
		})

	regH("trimEnd", "input: string", "trims trailing whitespace",
		func(args []Value) Value {
			return Str(strings.TrimRight(requireString("trimEnd", arg(args, 0)), " \t\n\r\f\v"))
		})

	// --- Array operations ---

	reg("at", "input: array|string, index?: number = 0", "element at index (supports negative)",
		[]string{`[1,2,3] |> at(-1)`},
		func(args []Value) Value {
			idx := 0
			if len(args) > 1 && args[1] != Null {
				idx = int(requireNumber("at", args, 1))
			}
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				i := idx
				if i < 0 {
					i += len(input.Elements)
				}
				if i >= 0 && i < len(input.Elements) {
					return input.Elements[i]
				}
				return Null
			case *StringVal:
				r := []rune(input.V)
				i := idx
				if i < 0 {
					i += len(r)
				}
				if i >= 0 && i < len(r) {
					return Str(string(r[i]))
				}
				return Null
			default:
				panic(TypeMismatch("at", "array or string", input, ""))
			}
		})

	reg("forEach", "input: array, fn: (T, index?) => void", "applies fn to each element, returns null",
		[]string{`[1, 2, 3] |> forEach(x => print(x))`},
		func(args []Value) Value {
			a := requireArray("forEach", arg(args, 0))
			fn := requireFn("forEach", args, 1)
			for i, el := range a.Elements {
				Call(fn, []Value{el, Num(float64(i))})
			}
			return Null
		})

	reg("fill", "input: array, value: any, start?: number, end?: number", "fills array with value, returns new array",
		[]string{`[0, 0, 0] |> fill(1)`},
		func(args []Value) Value {
			a := requireArray("fill", arg(args, 0))
			value := arg(args, 1)
			out := append([]Value(nil), a.Elements...)
			start := 0
			if s := optInt(args, 2); s != nil {
				start = *s
			}
			end := len(out)
			if e := optInt(args, 3); e != nil {
				end = *e
			}
			for i := start; i < min(end, len(out)); i++ {
				out[i] = value
			}
			return &ArrayVal{Elements: out}
		})

	reg("concat", "input: array|string, ...values: any",
		"concatenates, as JS concat: array arguments are spread one level, others appended; on a string, joins as strings",
		[]string{`[1, 2] |> concat([3, 4])`, `[1] |> concat([2], 3)`},
		func(args []Value) Value {
			if s, ok := arg(args, 0).(*StringVal); ok {
				var b strings.Builder
				b.WriteString(s.V)
				for _, x := range args[1:] {
					b.WriteString(x.Display())
				}
				return Str(b.String())
			}
			a := requireArray("concat", arg(args, 0))
			out := append([]Value(nil), a.Elements...)
			for _, x := range args[1:] {
				if sub, ok := x.(*ArrayVal); ok {
					out = append(out, sub.Elements...)
				} else {
					out = append(out, x)
				}
			}
			return &ArrayVal{Elements: out}
		})

	regH("push", "input: array, ...values: any", "appends values to array, returns array",
		func(args []Value) Value {
			a := requireArray("push", arg(args, 0))
			a.Elements = append(a.Elements, args[1:]...)
			return a
		})

	regH("pop", "input: array", "removes and returns last element",
		func(args []Value) Value {
			a := requireArray("pop", arg(args, 0))
			if len(a.Elements) == 0 {
				return Null
			}
			last := a.Elements[len(a.Elements)-1]
			a.Elements = a.Elements[:len(a.Elements)-1]
			return last
		})

	reg("indexOf", "input: array|string, value: any, fromIndex?: number", "first index of value from fromIndex on, or -1",
		[]string{`[10, 20, 30] |> indexOf(20)`, `"hello" |> indexOf("ll")`},
		func(args []Value) Value {
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				target := arg(args, 1)
				for i := elemFrom(args, 2, len(input.Elements)); i < len(input.Elements); i++ {
					if valuesEqual(input.Elements[i], target) {
						return Num(float64(i))
					}
				}
				return Num(-1)
			case *StringVal:
				sub, ok := arg(args, 1).(*StringVal)
				if !ok {
					panic(TypeMismatch("indexOf", "string", arg(args, 1), ""))
				}
				return Num(float64(runeIndex(input.V, sub.V, optInt(args, 2), false)))
			default:
				panic(TypeMismatch("indexOf", "array or string", input, ""))
			}
		})

	reg("lastIndexOf", "input: array|string, value: any, fromIndex?: number", "last index of value at or before fromIndex, or -1",
		[]string{`"hello world hello" |> lastIndexOf("hello")`, `[1, 2, 1] |> lastIndexOf(1)`},
		func(args []Value) Value {
			if a, ok := arg(args, 0).(*ArrayVal); ok {
				start := len(a.Elements) - 1
				if from := optInt(args, 2); from != nil {
					start = *from
					if start < 0 {
						start += len(a.Elements)
					}
					start = min(start, len(a.Elements)-1)
				}
				for i := start; i >= 0; i-- {
					if valuesEqual(a.Elements[i], arg(args, 1)) {
						return Num(float64(i))
					}
				}
				return Num(-1)
			}
			input := requireString("lastIndexOf", arg(args, 0))
			sub, ok := arg(args, 1).(*StringVal)
			if !ok {
				panic(WrongArguments("lastIndexOf", "input: string, substr: string", args, ""))
			}
			return Num(float64(runeIndex(input, sub.V, optInt(args, 2), true)))
		})

	reg("flatMap", "input: array, fn: (T) => array", "maps then flattens one level",
		[]string{`[1, 2, 3] |> flatMap(x => [x, x * 10])`},
		func(args []Value) Value {
			a := requireArray("flatMap", arg(args, 0))
			fn := requireFn("flatMap", args, 1)
			var out []Value
			for _, el := range a.Elements {
				if mapped, ok := Call(fn, []Value{el}).(*ArrayVal); ok {
					out = append(out, mapped.Elements...)
				} else {
					out = append(out, Call(fn, []Value{el}))
				}
			}
			return &ArrayVal{Elements: out}
		})

	reg("some", "input: array, fn: (T) => boolean", "true if any element matches",
		[]string{`[1, 2, 3] |> some(x => x > 2)`},
		func(args []Value) Value {
			a := requireArray("some", arg(args, 0))
			fn := requireFn("some", args, 1)
			for _, el := range a.Elements {
				if Call(fn, []Value{el}).IsTruthy() {
					return Bln(true)
				}
			}
			return Bln(false)
		})

	reg("every", "input: array, fn: (T) => boolean", "true if all elements match",
		[]string{`[1, 2, 3] |> every(x => x > 0)`},
		func(args []Value) Value {
			a := requireArray("every", arg(args, 0))
			fn := requireFn("every", args, 1)
			for _, el := range a.Elements {
				if !Call(fn, []Value{el}).IsTruthy() {
					return Bln(false)
				}
			}
			return Bln(true)
		})

	reg("slice", "input: array|string, start?: number, end?: number",
		"extracts section, as JS slice: a negative index counts from the end",
		[]string{`[1, 2, 3, 4] |> slice(1, 3)`, `[1, 2, 3, 4] |> slice(-2)`},
		func(args []Value) Value {
			if len(args) > 1 && args[1] != Null {
				requireNumber("slice", args, 1)
			}
			switch input := arg(args, 0).(type) {
			case *ArrayVal:
				lo, hi := sliceBounds(args, len(input.Elements))
				return &ArrayVal{Elements: append([]Value(nil), input.Elements[lo:hi]...)}
			case *StringVal:
				r := []rune(input.V)
				lo, hi := sliceBounds(args, len(r))
				return Str(string(r[lo:hi]))
			default:
				panic(TypeMismatch("slice", "array or string", input, ""))
			}
		})

	// --- Set operations ---

	reg("difference", "a: array, b: array", "elements in a not in b",
		[]string{`difference([1, 2, 3, 4], [2, 4])`},
		func(args []Value) Value {
			a := requireArray("difference", arg(args, 0))
			b := requireArray("difference", arg(args, 1))
			var out []Value
			for _, ae := range a.Elements {
				if !containsValue(b.Elements, ae) {
					out = append(out, ae)
				}
			}
			return &ArrayVal{Elements: out}
		})

	reg("intersection", "a: array, b: array", "elements in both a and b",
		[]string{`intersection([1, 2, 3], [2, 3, 4])`},
		func(args []Value) Value {
			a := requireArray("intersection", arg(args, 0))
			b := requireArray("intersection", arg(args, 1))
			var out []Value
			for _, ae := range a.Elements {
				if containsValue(b.Elements, ae) {
					out = append(out, ae)
				}
			}
			return &ArrayVal{Elements: out}
		})

	reg("union", "a: array, b: array", "combined, deduplicated",
		[]string{`union([1, 2, 3], [2, 3, 4])`},
		func(args []Value) Value {
			a := requireArray("union", arg(args, 0))
			b := requireArray("union", arg(args, 1))
			out := append([]Value(nil), a.Elements...)
			for _, be := range b.Elements {
				if !containsValue(out, be) {
					out = append(out, be)
				}
			}
			return &ArrayVal{Elements: out}
		})

	// --- Math ---

	reg("floor", "n: number", "rounds down", []string{`3.7 |> floor()`},
		func(args []Value) Value { return IntegerPart(requireNum("floor", args), "floor") })
	reg("ceil", "n: number", "rounds up", []string{`3.2 |> ceil()`},
		func(args []Value) Value { return IntegerPart(requireNum("ceil", args), "ceil") })
	reg("round", "n: number, digits?: number", "rounds to the nearest integer, or to digits decimal places; a half rounds up (2.5 → 3, -2.5 → -2) as in JS Math.round",
		[]string{`3.5 |> round()`, `round(2.345, 2)`},
		func(args []Value) Value { return roundArgs("round", args) })
	reg("abs", "n: number", "absolute value", []string{`-5 |> abs()`},
		func(args []Value) Value { return AbsNumber(requireNum("abs", args)) })

	reg("min", "...values: number[]", "minimum (no arguments → Infinity, as in JS)", []string{`min(3, 1, 2)`},
		func(args []Value) Value { return minMax(args, "min", false) })
	reg("max", "...values: number[]", "maximum (no arguments → -Infinity, as in JS)", []string{`max(3, 1, 2)`},
		func(args []Value) Value { return minMax(args, "max", true) })

	reg("pow", "base: number, exp: number", "exponentiation", []string{`pow(2, 3)`},
		func(args []Value) Value {
			return Num(math.Pow(requireNumber("pow", args, 0), requireNumber("pow", args, 1)))
		})

	// --- Execution limits ---

	// Hidden: the eval tool's profile and limits arguments (and the CLI's
	// --profile and --limits) replace them in the prompt; they stay callable.
	regH("profile", "fn: () => any",
		"runs fn with cost tracing; returns {result, steps, lines: [{line, steps}]} sorted by cost",
		func(args []Value) Value {
			fn := requireFn("profile", args, 0)
			lim := sh.Limits()
			if lim.Profiling() {
				// the whole eval is being profiled: its block has the lines
				out := NewObject()
				out.Set("result", Call(fn, nil))
				out.Set("steps", Num(0))
				out.Set("lines", &ArrayVal{})
				return out
			}
			lim.StartProfile()
			defer func() {
				if lim.Profiling() {
					lim.StopProfile() // ensure tracing is off if fn panicked (e.g. hit a limit)
				}
			}()
			result := Call(fn, nil)
			lines, total := lim.StopProfile()
			out := NewObject()
			out.Set("result", result)
			out.Set("steps", Num(float64(total)))
			rows := make([]Value, 0, len(lines))
			for _, lc := range lines {
				row := NewObject()
				row.Set("line", Num(float64(lc.Line)))
				row.Set("steps", Num(float64(lc.Steps)))
				rows = append(rows, row)
			}
			out.Set("lines", &ArrayVal{Elements: rows})
			return out
		})

	regH("extendLimit", "opts: {steps?: number, timeout?: number, callDepth?: number, outputBytes?: number}",
		"increases execution limits for this eval. Call before heavy computation",
		func(args []Value) Value {
			opts, ok := arg(args, 0).(*ObjectVal)
			if !ok {
				panic(WrongArguments("extendLimit",
					"{steps?: number, timeout?: number, callDepth?: number, outputBytes?: number}", args,
					"extendLimit({steps: 5000000})"))
			}
			lim := sh.Limits()
			result := NewObject()
			if n := optObjInt(opts, "steps"); n != nil {
				if *n <= lim.MaxSteps {
					panic(Runtime(fmt.Sprintf("extendLimit: steps (%d) must be greater than current (%d)", *n, lim.MaxSteps)))
				}
				if *n > lim.CapSteps {
					panic(CapError("extendLimit", "steps", int64(*n), int64(lim.CapSteps)))
				}
				lim.MaxSteps = *n
				result.Set("maxSteps", Num(float64(*n)))
			}
			if n := optObjInt(opts, "timeout"); n != nil {
				if int64(*n) <= lim.TimeoutMs {
					panic(Runtime(fmt.Sprintf("extendLimit: timeout (%d) must be greater than current (%d)", *n, lim.TimeoutMs)))
				}
				if int64(*n) > lim.CapTimeoutMs {
					panic(CapError("extendLimit", "timeout", int64(*n), int64(lim.CapTimeoutMs)))
				}
				lim.TimeoutMs = int64(*n)
				result.Set("timeoutMs", Num(float64(*n)))
			}
			if n := optObjInt(opts, "callDepth"); n != nil {
				if *n <= lim.MaxCallDepth {
					panic(Runtime(fmt.Sprintf("extendLimit: callDepth (%d) must be greater than current (%d)", *n, lim.MaxCallDepth)))
				}
				if *n > lim.CapCallDepth {
					panic(CapError("extendLimit", "callDepth", int64(*n), int64(lim.CapCallDepth)))
				}
				lim.MaxCallDepth = *n
				result.Set("maxCallDepth", Num(float64(*n)))
			}
			if n := optObjInt(opts, "outputBytes"); n != nil {
				if *n <= lim.MaxOutputBytes {
					panic(Runtime(fmt.Sprintf("extendLimit: outputBytes (%d) must be greater than current (%d)", *n, lim.MaxOutputBytes)))
				}
				if *n > lim.CapOutputBytes {
					panic(CapError("extendLimit", "outputBytes", int64(*n), int64(lim.CapOutputBytes)))
				}
				lim.MaxOutputBytes = *n
				result.Set("maxOutputBytes", Num(float64(*n)))
			}
			if result.Len() == 0 {
				panic(Runtime("extendLimit: no valid limits provided. Use {steps, timeout, callDepth, outputBytes}"))
			}
			return result
		})

	regH("limits", "", "shows current execution limits",
		func(args []Value) Value {
			lim := sh.Limits()
			result := NewObject()
			result.Set("maxSteps", Num(float64(lim.MaxSteps)))
			result.Set("maxCallDepth", Num(float64(lim.MaxCallDepth)))
			result.Set("timeoutMs", Num(float64(lim.TimeoutMs)))
			result.Set("maxOutputBytes", Num(float64(lim.MaxOutputBytes)))
			return result
		})

	// --- JSON ---

	reg("parseJson", "input: string", "JSON string → value (loose: bare keys + trailing commas OK; values stay strict — no bare words)", []string{`parseJson("{\"a\": 1}")`, `parseJson("{a: 1, b: [2, 3],}")`},
		func(args []Value) Value {
			s := strings.TrimSpace(requireString("parseJson", arg(args, 0)))
			pos := 0
			return parseJSONValue(s, &pos)
		})

	reg("toJson", "input: any", "value → JSON string", []string{`toJson({a: 1, b: [2, 3]})`},
		func(args []Value) Value { return Str(toJSONString(arg(args, 0))) })

	// --- JS compat (hidden) ---

	regH("xor", "a: number, b: number", "bitwise XOR (alias of the ^ operator)",
		func(args []Value) Value {
			return Bitwise("^", arg(args, 0), arg(args, 1))
		})

	regH("jsonStringify", "value: any, replacer?: null|string[]|(key, value) => any, space?: number|string",
		"JSON.stringify: a replacer function or key list filters; space indents (a number of spaces, at most 10, or a string)",
		func(args []Value) Value {
			value := arg(args, 0)
			switch r := argOpt(args, 1).(type) {
			case nil, *NullVal:
			case *FuncVal:
				value = replaceJSON(r, "", value)
			case *ArrayVal:
				value = filterJSONKeys(r, value)
			default:
				panic(TypeMismatch("JSON.stringify", "replacer function, array of keys, or null", r, ""))
			}
			out := toJSONString(value)
			indent := ""
			switch sp := argOpt(args, 2).(type) {
			case *NumberVal:
				indent = strings.Repeat(" ", clamp(int(sp.V), 0, 10))
			case *StringVal:
				indent = sp.V
				if len(indent) > 10 {
					indent = indent[:10]
				}
			}
			if indent == "" {
				return Str(out)
			}
			var b bytes.Buffer
			if err := json.Indent(&b, []byte(out), "", indent); err != nil {
				return Str(out)
			}
			return Str(b.String())
		})

	regH("parseInt", "value: any, radix?: number", "parse the leading integer of a string in radix (default 10, or 16 after 0x); NaN when it has none",
		func(args []Value) Value {
			radix := 0
			if r := optInt(args, 1); r != nil {
				radix = *r
			}
			switch v := arg(args, 0).(type) {
			case *NumberVal:
				return jsParseInt(v.Display(), radix)
			case *StringVal:
				return jsParseInt(v.V, radix)
			default:
				return Num(math.NaN())
			}
		})

	regH("parseFloat", "value: any", "parse the leading decimal number of a string; NaN when it has none",
		func(args []Value) Value {
			switch v := arg(args, 0).(type) {
			case *NumberVal:
				return v
			case *StringVal:
				return jsParseFloat(v.V)
			default:
				return Num(math.NaN())
			}
		})

	regH("Number", "value: any", "convert to number",
		func(args []Value) Value {
			switch v := arg(args, 0).(type) {
			case *NumberVal:
				return v
			case *StringVal:
				f, err := strconv.ParseFloat(strings.TrimSpace(v.V), 64)
				if err != nil {
					return Num(math.NaN())
				}
				return Num(f)
			case *BoolVal:
				if v.V {
					return Num(1)
				}
				return Num(0)
			case *NullVal:
				return Num(0)
			default:
				return Num(math.NaN())
			}
		})

	regH("findIndex", "input: array, fn: (T, index?) => boolean", "index of the first match, or -1",
		func(args []Value) Value { return Num(float64(findIndexOf("findIndex", args, false))) })

	regH("findLastIndex", "input: array, fn: (T, index?) => boolean", "index of the last match, or -1",
		func(args []Value) Value { return Num(float64(findIndexOf("findLastIndex", args, true))) })

	regH("findLast", "input: array, fn: (T, index?) => boolean", "last match or null",
		func(args []Value) Value {
			if i := findIndexOf("findLast", args, true); i >= 0 {
				return arg(args, 0).(*ArrayVal).Elements[i]
			}
			return Null
		})

	regH("withAt", "input: array, index: number, value: any", "copy with one element replaced (JS arr.with)",
		func(args []Value) Value {
			a := requireArray("with", arg(args, 0))
			i := int(requireNumber("with", args, 1))
			if i < 0 {
				i += len(a.Elements)
			}
			if i < 0 || i >= len(a.Elements) {
				panic(Runtime(fmt.Sprintf("with: index %d is out of range for length %d", int(requireNumber("with", args, 1)), len(a.Elements))))
			}
			out := append([]Value(nil), a.Elements...)
			out[i] = arg(args, 2)
			return &ArrayVal{Elements: out}
		})

	regH("toSpliced", "input: array, start: number, deleteCount?: number, ...items: any", "copy with splice applied (JS arr.toSpliced)",
		func(args []Value) Value {
			a := requireArray("toSpliced", arg(args, 0))
			n := len(a.Elements)
			start := 0
			if s := optInt(args, 1); s != nil {
				start = *s
				if start < 0 {
					start = max(n+start, 0)
				}
				start = min(start, n)
			}
			del := n - start
			if d := optInt(args, 2); d != nil {
				del = clamp(*d, 0, n-start)
			} else if len(args) < 2 {
				del = 0
			}
			out := append([]Value(nil), a.Elements[:start]...)
			if len(args) > 3 {
				out = append(out, args[3:]...)
			}
			out = append(out, a.Elements[start+del:]...)
			return &ArrayVal{Elements: out}
		})

	regH("arrayOf", "...values: any", "array of the arguments (JS Array.of)",
		func(args []Value) Value { return &ArrayVal{Elements: append([]Value(nil), args...)} })

	regH("assign", "target: object, ...sources: object", "copy of target with each source's keys set (JS Object.assign, without mutation)",
		func(args []Value) Value {
			out := NewObject()
			for i, src := range args {
				o, ok := src.(*ObjectVal)
				if !ok {
					if src == Null && i > 0 {
						continue
					}
					panic(TypeMismatch("Object.assign", "object", src, ""))
				}
				for _, k := range o.Keys() {
					v, _ := o.Get(k)
					out.Set(k, v)
				}
			}
			return out
		})

	regH("structuredClone", "value: any", "deep copy of arrays and objects",
		func(args []Value) Value { return deepCopy(arg(args, 0)) })

	regH("localeCompare", "input: string, other: string", "-1, 0 or 1 by case-insensitive order, then case (lowercase first)",
		func(args []Value) Value {
			a := requireString("localeCompare", arg(args, 0))
			b := requireString("localeCompare", arg(args, 1))
			return Num(float64(localeCompare(a, b)))
		})

	regH("fromCharCode", "...codes: number", "string of the given UTF-16 code units (JS String.fromCharCode)",
		func(args []Value) Value {
			units := make([]uint16, len(args))
			for i := range args {
				units[i] = uint16(requireNumber("String.fromCharCode", args, i))
			}
			return Str(string(utf16.Decode(units)))
		})

	regH("isInteger", "value: any", "true for a finite integer number (JS Number.isInteger)",
		func(args []Value) Value {
			n, ok := arg(args, 0).(*NumberVal)
			return Bln(ok && !math.IsInf(n.V, 0) && n.V == math.Trunc(n.V))
		})

	regH("isFinite", "value: any", "true for a finite number (JS Number.isFinite; no string coercion)",
		func(args []Value) Value {
			n, ok := arg(args, 0).(*NumberVal)
			return Bln(ok && !math.IsInf(n.V, 0) && !math.IsNaN(n.V))
		})

	regH("isNaN", "value: any", "true for NaN (JS Number.isNaN; no string coercion)",
		func(args []Value) Value {
			n, ok := arg(args, 0).(*NumberVal)
			return Bln(ok && math.IsNaN(n.V))
		})

	regH("String", "value: any", "convert to string",
		func(args []Value) Value { return Str(arg(args, 0).Display()) })

	regH("Boolean", "value: any", "convert to boolean",
		func(args []Value) Value { return Bln(arg(args, 0).IsTruthy()) })

	// JS ignores one argument past these (the thisArg of a callback method,
	// the radix of Number(x, 10)); ignoring it gives the JS result exactly.
	for _, name := range []string{"map", "filter", "find", "forEach", "flatMap", "some", "every",
		"findIndex", "findLastIndex", "findLast", "toArray",
		"str", "num", "bool", "Number", "String", "Boolean", "parseFloat"} {
		if c := sh.Commands().Get(name); c != nil {
			c.Tolerate = 1
		}
	}
	return sh
}

// loadEach runs load or unload for each name, in order. Names may be given
// as arguments or as one array. One name returns its result as before;
// several return the summaries joined by lines. It stops at the first
// failure, and the error names what was already done and what was not
// tried, so the agent does not have to guess the state.
func loadEach(sh *Shell, cmd string, args []Value, one func(string) Value) Value {
	if len(args) == 1 {
		if arr, ok := args[0].(*ArrayVal); ok {
			args = arr.Elements
		}
	}
	if len(args) == 0 {
		panic(Runtime(cmd + ": give at least one toolkit name, e.g. " + cmd + `("db")`))
	}
	names := make([]string, len(args))
	for i, a := range args {
		s, ok := a.(*StringVal)
		if !ok {
			panic(WrongArguments(cmd, "string (a toolkit name or a file path)", args, ""))
		}
		names[i] = s.V
	}
	if len(names) == 1 {
		return one(names[0])
	}
	var out []string
	for i, name := range names {
		v, err := tryOne(func() Value { return one(name) })
		if err != nil {
			msg := fmt.Sprintf("%s: %q failed: %s", cmd, name, err.Thrown())
			if i > 0 {
				msg += fmt.Sprintf("\n\n  Done before it: %s", strings.Join(names[:i], ", "))
			}
			if i < len(names)-1 {
				msg += fmt.Sprintf("\n  Not tried: %s", strings.Join(names[i+1:], ", "))
			}
			panic(Runtime(msg))
		}
		out = append(out, v.Display())
	}
	return Str(strings.Join(out, "\n"))
}

// tryOne runs fn and returns a script error it raised. A Go panic that is
// not a ShellError (a bug) is re-raised, so it is reported as internal.
func tryOne(fn func() Value) (v Value, err *ShellError) {
	defer func() {
		if r := recover(); r != nil {
			se, ok := r.(*ShellError)
			if !ok || se.Internal() {
				panic(r)
			}
			err = se
		}
	}()
	return fn(), nil
}
