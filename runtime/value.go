package runtime

import (
	"math"
	"strconv"
	"strings"
)

// Value is the mcpshell runtime value type — a closed set of concrete types,
// always handled as pointers so that arrays and objects carry JS-style mutable
// reference semantics.
type Value interface {
	isValue()
	// TypeName returns the mcpshell type name ("string", "number", ...).
	TypeName() string
	// IsTruthy applies mcpshell/JS truthiness rules.
	IsTruthy() bool
	// Display is the human-facing rendering (toDisplayString).
	Display() string
	// Inspect is Display, but strings are quoted (toInspectString).
	Inspect() string
}

// --- String -----------------------------------------------------------------

type StringVal struct{ V string }

func (*StringVal) isValue()          {}
func (*StringVal) TypeName() string  { return "string" }
func (s *StringVal) IsTruthy() bool  { return s.V != "" }
func (s *StringVal) Display() string { return s.V }
func (s *StringVal) Inspect() string { return "\"" + s.V + "\"" }

// --- Number ------------------------------------------------------------------

type NumberVal struct{ V float64 }

func (*NumberVal) isValue()         {}
func (*NumberVal) TypeName() string { return "number" }
func (n *NumberVal) IsTruthy() bool { return n.V != 0 }

func (n *NumberVal) Display() string {
	v := n.V
	if !math.IsInf(v, 0) && !math.IsNaN(v) && v == math.Trunc(v) {
		// Integer-valued: render without exponent.
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
func (n *NumberVal) Inspect() string { return n.Display() }

// --- Boolean -----------------------------------------------------------------

type BoolVal struct{ V bool }

func (*BoolVal) isValue()          {}
func (*BoolVal) TypeName() string  { return "boolean" }
func (b *BoolVal) IsTruthy() bool  { return b.V }
func (b *BoolVal) Display() string { return strconv.FormatBool(b.V) }
func (b *BoolVal) Inspect() string { return b.Display() }

// --- Null --------------------------------------------------------------------

type NullVal struct{}

// Null is the single shared null instance ("undefined" is an alias for it).
var Null = &NullVal{}

func (*NullVal) isValue()         {}
func (*NullVal) TypeName() string { return "null" }
func (*NullVal) IsTruthy() bool   { return false }
func (*NullVal) Display() string  { return "null" }
func (*NullVal) Inspect() string  { return "null" }

// --- Array -------------------------------------------------------------------

// ArrayVal is a mutable reference type. Mutating commands (push/pop/...) and
// the interpreter rewrite Elements in place, and all holders observe it.
type ArrayVal struct{ Elements []Value }

func (*ArrayVal) isValue()         {}
func (*ArrayVal) TypeName() string { return "array" }
func (a *ArrayVal) IsTruthy() bool { return len(a.Elements) > 0 }

func (a *ArrayVal) Display() string {
	parts := make([]string, len(a.Elements))
	for i, e := range a.Elements {
		parts[i] = e.Inspect()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (a *ArrayVal) Inspect() string { return a.Display() }

// --- Object ------------------------------------------------------------------

// ObjectVal is an insertion-ordered, mutable map. Ordering matters: object
// display, entries(), and help() output all depend on it, and Go's builtin map
// iteration is randomized — hence the explicit key slice.
type ObjectVal struct {
	keys []string
	m    map[string]Value
}

func NewObject() *ObjectVal {
	return &ObjectVal{m: make(map[string]Value)}
}

func (*ObjectVal) isValue()         {}
func (*ObjectVal) TypeName() string { return "object" }
func (o *ObjectVal) IsTruthy() bool { return len(o.keys) > 0 }
func (o *ObjectVal) Len() int       { return len(o.keys) }

func (o *ObjectVal) Get(k string) (Value, bool) {
	v, ok := o.m[k]
	return v, ok
}

func (o *ObjectVal) Has(k string) bool {
	_, ok := o.m[k]
	return ok
}

// Set inserts or updates k, preserving first-insertion order.
func (o *ObjectVal) Set(k string, v Value) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *ObjectVal) Delete(k string) {
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the keys in insertion order. The result must not be mutated.
func (o *ObjectVal) Keys() []string { return o.keys }

func (o *ObjectVal) Display() string {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(o.m[k].Inspect())
	}
	sb.WriteByte('}')
	return sb.String()
}
func (o *ObjectVal) Inspect() string { return o.Display() }

// --- Regex -------------------------------------------------------------------

type RegexVal struct {
	Pattern string
	Flags   string
}

func (*RegexVal) isValue()          {}
func (*RegexVal) TypeName() string  { return "regex" }
func (*RegexVal) IsTruthy() bool    { return true }
func (r *RegexVal) Display() string { return "/" + r.Pattern + "/" + r.Flags }
func (r *RegexVal) Inspect() string { return r.Display() }

// --- Function ----------------------------------------------------------------

// NativeFn is a command implemented in Go. It panics with *ShellError on
// failure, mirroring the throw-based error handling of the interpreter.
type NativeFn func(args []Value) Value

// FunctionBody is one of NativeBody, ExpressionBody, or BlockBody.
type FunctionBody interface{ isFunctionBody() }

// NativeBody wraps a Go-implemented command or pipe function.
type NativeBody struct{ Fn NativeFn }

// ExpressionBody is an arrow function with an expression body: `x => expr`.
type ExpressionBody struct {
	Node        any // parser.IExpressionContext
	CapturedEnv *Environment
	Commands    *CommandRegistry
	Limits      *ExecutionLimits
}

// BlockBody is a named function or a block-bodied arrow: `fn f() {}` / `x => {}`.
type BlockBody struct {
	Node        any // parser.IBlockContext
	CapturedEnv *Environment
	Commands    *CommandRegistry
	Limits      *ExecutionLimits
}

func (*NativeBody) isFunctionBody()     {}
func (*ExpressionBody) isFunctionBody() {}
func (*BlockBody) isFunctionBody()      {}

// FuncVal is a callable value: native command, arrow, or named function.
type FuncVal struct {
	Name   string
	Params []string
	Body   FunctionBody
	// ParamDefaults holds the default-value AST node per param (nil = none).
	ParamDefaults []any
	// ParamNodes holds the full param AST node per param, for destructured params.
	ParamNodes []any
}

func (*FuncVal) isValue()         {}
func (*FuncVal) TypeName() string { return "function" }
func (*FuncVal) IsTruthy() bool   { return true }

func (f *FuncVal) Display() string {
	name := f.Name
	if name == "" {
		name = "<anonymous>"
	}
	return "function " + name + "(" + strings.Join(f.Params, ", ") + ")"
}
func (f *FuncVal) Inspect() string { return f.Display() }

// --- Constructors ------------------------------------------------------------

func Str(s string) *StringVal    { return &StringVal{V: s} }
func Num(f float64) *NumberVal   { return &NumberVal{V: f} }
func Bln(b bool) *BoolVal        { return &BoolVal{V: b} }
func Arr(els ...Value) *ArrayVal { return &ArrayVal{Elements: els} }
