package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MapVal is a JS Map, Set, WeakMap or WeakSet. Keys compare as JS
// SameValueZero: a string, number, boolean or null by value, anything else
// (object, array, function, Date) by identity, so two equal-looking objects
// are two keys, as in JS. Insertion order is kept.
type MapVal struct {
	kind  mapKind
	keys  []Value // nil entry = deleted
	vals  []Value // Map and WeakMap; nil for sets
	index map[any]int
	live  int
}

type mapKind int8

const (
	kindMap mapKind = iota
	kindSet
	kindWeakMap
	kindWeakSet
)

// NewMap and NewSet make empty collections (for toolkit code).
func NewMap() *MapVal { return &MapVal{kind: kindMap, index: map[any]int{}} }
func NewSet() *MapVal { return &MapVal{kind: kindSet, index: map[any]int{}} }

func (*MapVal) isValue()          {}
func (*MapVal) TypeName() string  { return "object" }
func (*MapVal) IsTruthy() bool    { return true }
func (m *MapVal) Display() string { return inspectIn(m, &Ancestors{}) }
func (m *MapVal) Inspect() string { return m.Display() }

func (m *MapVal) kindName() string {
	return [...]string{"Map", "Set", "WeakMap", "WeakSet"}[m.kind]
}

func (m *MapVal) isSet() bool  { return m.kind == kindSet || m.kind == kindWeakSet }
func (m *MapVal) isWeak() bool { return m.kind == kindWeakMap || m.kind == kindWeakSet }

// Size is the number of entries.
func (m *MapVal) Size() int { return m.live }

// sameValueKey is the Go map key of a JS SameValueZero comparison.
func sameValueKey(v Value) any {
	switch x := v.(type) {
	case *StringVal:
		return "s" + x.V
	case *NumberVal:
		if math.IsNaN(x.V) {
			return "fNaN"
		}
		if b, ok := x.asBigInt(); ok {
			return "i" + b.String()
		}
		if x.V == 0 {
			return "i0" // -0 and 0 are one key
		}
		return "f" + strconv.FormatFloat(x.V, 'g', -1, 64)
	case *BoolVal:
		if x.V {
			return "btrue"
		}
		return "bfalse"
	case *NullVal, nil:
		return "null"
	}
	return v // by identity
}

func (m *MapVal) checkWeakKey(k Value) {
	if !m.isWeak() {
		return
	}
	switch k.(type) {
	case *ObjectVal, *ArrayVal, *FuncVal, *MapVal, *DateVal, *RegexVal:
		return
	}
	panic(Runtime(fmt.Sprintf("Invalid value used as a %s key: %s. A weak collection holds objects only; use a Map or Set for strings and numbers", m.kindName(), k.Inspect())))
}

// Get returns the value for k.
func (m *MapVal) Get(k Value) (Value, bool) {
	i, ok := m.index[sameValueKey(k)]
	if !ok {
		return nil, false
	}
	if m.isSet() {
		return m.keys[i], true
	}
	return m.vals[i], true
}

// Put sets k to val (val is ignored for a set).
func (m *MapVal) Put(k, val Value) {
	m.checkWeakKey(k)
	if n, ok := k.(*NumberVal); ok && n.V == 0 && math.Signbit(n.V) {
		k = numInt(0) // -0 becomes +0, as in JS
	}
	key := sameValueKey(k)
	if i, ok := m.index[key]; ok {
		if !m.isSet() {
			m.vals[i] = val
		}
		return
	}
	m.index[key] = len(m.keys)
	m.keys = append(m.keys, k)
	if !m.isSet() {
		m.vals = append(m.vals, val)
	}
	m.live++
}

// Remove deletes k and reports whether it was present.
func (m *MapVal) Remove(k Value) bool {
	key := sameValueKey(k)
	i, ok := m.index[key]
	if !ok {
		return false
	}
	delete(m.index, key)
	m.keys[i] = nil
	if !m.isSet() {
		m.vals[i] = nil
	}
	m.live--
	if len(m.keys) > 32 && m.live < len(m.keys)/2 {
		m.compact()
	}
	return true
}

func (m *MapVal) compact() {
	var keys, vals []Value
	m.index = make(map[any]int, m.live)
	for i, k := range m.keys {
		if k == nil {
			continue
		}
		m.index[sameValueKey(k)] = len(keys)
		keys = append(keys, k)
		if !m.isSet() {
			vals = append(vals, m.vals[i])
		}
	}
	m.keys, m.vals = keys, vals
}

func (m *MapVal) clear() {
	m.keys, m.vals, m.index, m.live = nil, nil, map[any]int{}, 0
}

// Each calls f for every entry in insertion order. An entry added during
// the walk is visited; one deleted before it is reached is not (JS order).
func (m *MapVal) Each(f func(k, val Value)) {
	for i := 0; i < len(m.keys); i++ {
		k := m.keys[i]
		if k == nil {
			continue
		}
		if m.isSet() {
			f(k, k)
		} else {
			f(k, m.vals[i])
		}
	}
}

// Entries returns [k, v] pairs for a Map, values for a Set: what for…of and
// spread give.
func (m *MapVal) Entries() []Value {
	out := make([]Value, 0, m.live)
	m.Each(func(k, val Value) {
		if m.isSet() {
			out = append(out, k)
		} else {
			out = append(out, Arr(k, val))
		}
	})
	return out
}

func (m *MapVal) inspect(anc *Ancestors) string {
	if m.isWeak() {
		return m.kindName() + " { <items unknown> }"
	}
	parts := make([]string, 0, m.live)
	m.Each(func(k, val Value) {
		if m.isSet() {
			parts = append(parts, inspectIn(k, anc))
		} else {
			parts = append(parts, inspectIn(k, anc)+" => "+inspectIn(val, anc))
		}
	})
	return fmt.Sprintf("%s(%d) {%s}", m.kindName(), m.live, strings.Join(parts, ", "))
}

// Iterate returns the values for…of and spread walk over: an array's
// elements, a string's characters, a Map's [key, value] pairs, a Set's
// values. ok is false for anything else.
func Iterate(v Value) ([]Value, bool) {
	switch x := v.(type) {
	case *ArrayVal:
		return x.Elements, true
	case *StringVal:
		out := []Value{}
		for _, r := range x.V {
			out = append(out, &StringVal{V: string(r)})
		}
		return out, true
	case *MapVal:
		if x.isWeak() {
			return nil, false
		}
		return x.Entries(), true
	}
	return nil, false
}

// newCollection is new Map(iterable) / new Set(iterable) / the weak forms.
func newCollection(kind mapKind, args []Value) *MapVal {
	m := &MapVal{kind: kind, index: map[any]int{}}
	if len(args) == 0 || args[0] == Null {
		return m
	}
	if len(args) > 1 {
		panic(Runtime(fmt.Sprintf("new %s takes one argument (an iterable), got %d", m.kindName(), len(args))))
	}
	items, ok := Iterate(args[0])
	if !ok {
		hint := "an array"
		if _, isObj := args[0].(*ObjectVal); isObj && !m.isSet() {
			hint = "an array of [key, value] pairs: new Map(Object.entries(obj))"
		}
		panic(TypeMismatch("new "+m.kindName(), "iterable", args[0], "pass "+hint))
	}
	for i, it := range items {
		if m.isSet() {
			m.Put(it, nil)
			continue
		}
		pair, ok := it.(*ArrayVal)
		if !ok || len(pair.Elements) < 2 {
			panic(TypeMismatch(fmt.Sprintf("new %s entry %d", m.kindName(), i), "[key, value] pair", it, "new Map([[k1, v1], [k2, v2]]) or new Map(Object.entries(obj))"))
		}
		m.Put(pair.Elements[0], pair.Elements[1])
	}
	return m
}

// collectionNamespace is the global Map, Set, WeakMap or WeakSet: new X(…)
// constructs; X(…) without new is an error, as in JS.
func collectionNamespace(kind mapKind) *ObjectVal {
	name := (&MapVal{kind: kind}).kindName()
	ns := NewObject()
	ns.Set("__new", nativeFn(name, func(args []Value) Value { return newCollection(kind, args) }))
	ns.Set("__call", nativeFn(name, func([]Value) Value {
		panic(Runtime("Constructor " + name + " requires 'new': new " + name + "(…)"))
	}))
	if kind == kindMap {
		ns.Set("groupBy", nativeFn("Map.groupBy", func(args []Value) Value {
			if len(args) != 2 {
				panic(Runtime("Map.groupBy(items, fn) takes 2 arguments"))
			}
			items, ok := Iterate(args[0])
			fn := asCallable(args[1])
			if !ok || fn == nil {
				panic(Runtime("Map.groupBy(items, fn): items is an array, fn a function"))
			}
			out := NewMap()
			for i, it := range items {
				k := Call(fn, []Value{it, numInt(int64(i))})
				g, has := out.Get(k)
				if !has {
					g = &ArrayVal{}
					out.Put(k, g)
				}
				g.(*ArrayVal).Elements = append(g.(*ArrayVal).Elements, it)
			}
			return out
		}))
	}
	return ns
}

// collectionMember is m.field.
func (v *Visitor) collectionMember(m *MapVal, field string) Value {
	name := m.kindName()
	method := func(params []string, fn func(args []Value) Value) Value {
		return &FuncVal{Name: field, Params: params, Body: &NativeBody{Fn: fn}}
	}
	need := func(args []Value, n int) {
		if len(args) < n {
			panic(Runtime(fmt.Sprintf("%s.%s needs %d argument(s), got %d", name, field, n, len(args))))
		}
	}
	listing := func() {
		if m.isWeak() {
			panic(Runtime(name + " cannot be listed or counted (" + field + "): its keys are held weakly in JS. Use a " + strings.TrimPrefix(name, "Weak") + " to list them"))
		}
	}
	switch field {
	case "size":
		listing()
		return numInt(int64(m.live))
	case "has":
		return method([]string{"key"}, func(args []Value) Value {
			need(args, 1)
			_, ok := m.Get(args[0])
			return Bln(ok)
		})
	case "delete":
		return method([]string{"key"}, func(args []Value) Value {
			need(args, 1)
			return Bln(m.Remove(args[0]))
		})
	case "clear":
		if m.isWeak() {
			break
		}
		return method(nil, func([]Value) Value { m.clear(); return Null })
	case "forEach":
		listing()
		return method([]string{"fn"}, func(args []Value) Value {
			need(args, 1)
			fn := asCallable(args[0])
			if fn == nil {
				panic(TypeMismatch(name+".forEach", "function", args[0], "m.forEach((value, key) => …)"))
			}
			m.Each(func(k, val Value) { v.callFunction(fn, []Value{val, k, m}, nil) })
			return Null
		})
	case "keys", "values", "entries":
		listing()
		return method(nil, func([]Value) Value {
			out := make([]Value, 0, m.live)
			m.Each(func(k, val Value) {
				switch {
				case field == "keys" || field == "values" && m.isSet():
					out = append(out, k)
				case field == "values":
					out = append(out, val)
				default:
					out = append(out, Arr(k, val))
				}
			})
			return &ArrayVal{Elements: out}
		})
	}
	if m.isSet() {
		switch field {
		case "add":
			return method([]string{"value"}, func(args []Value) Value {
				need(args, 1)
				m.Put(args[0], nil)
				return m
			})
		case "union", "intersection", "difference", "symmetricDifference", "isSubsetOf", "isSupersetOf", "isDisjointFrom":
			if m.isWeak() {
				break
			}
			return method([]string{"other"}, func(args []Value) Value {
				need(args, 1)
				o, ok := args[0].(*MapVal)
				if !ok || o.isWeak() {
					panic(TypeMismatch("Set."+field, "Set or Map", args[0], "a.union(new Set(list))"))
				}
				return setOp(field, m, o)
			})
		}
	} else {
		switch field {
		case "get":
			return method([]string{"key"}, func(args []Value) Value {
				need(args, 1)
				if val, ok := m.Get(args[0]); ok {
					return val
				}
				return Null
			})
		case "set":
			return method([]string{"key", "value"}, func(args []Value) Value {
				need(args, 2)
				m.Put(args[0], args[1])
				return m
			})
		}
	}
	members := "size, has, add, delete, clear, forEach, keys, values, entries, union, intersection, difference"
	switch m.kind {
	case kindMap:
		members = "size, get, set, has, delete, clear, forEach, keys, values, entries"
	case kindWeakMap:
		members = "get, set, has, delete"
	case kindWeakSet:
		members = "add, has, delete"
	}
	panic(Runtime(fmt.Sprintf("%s has no member '%s'. Members: %s", name, field, members)))
}

func setOp(op string, a, b *MapVal) Value {
	in := func(m *MapVal, k Value) bool { _, ok := m.Get(k); return ok }
	out := NewSet()
	switch op {
	case "union":
		a.Each(func(k, _ Value) { out.Put(k, nil) })
		b.Each(func(k, _ Value) { out.Put(k, nil) })
	case "intersection":
		a.Each(func(k, _ Value) {
			if in(b, k) {
				out.Put(k, nil)
			}
		})
	case "difference":
		a.Each(func(k, _ Value) {
			if !in(b, k) {
				out.Put(k, nil)
			}
		})
	case "symmetricDifference":
		a.Each(func(k, _ Value) {
			if !in(b, k) {
				out.Put(k, nil)
			}
		})
		b.Each(func(k, _ Value) {
			if !in(a, k) {
				out.Put(k, nil)
			}
		})
	case "isSubsetOf", "isSupersetOf", "isDisjointFrom":
		x, y := a, b
		if op == "isSupersetOf" {
			x, y = b, a
		}
		all, none := true, true
		x.Each(func(k, _ Value) {
			if in(y, k) {
				none = false
			} else {
				all = false
			}
		})
		if op == "isDisjointFrom" {
			return Bln(none)
		}
		return Bln(all)
	}
	return out
}
