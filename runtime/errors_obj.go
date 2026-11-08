package runtime

import "strings"

// The Error classes: Error and its builtin subclasses, real classes that
// user classes extend (class NotFound extends Error {}). throw keeps any
// value it is given (ShellError.Value), so catch (e) gets the Error object
// back and e instanceof NotFound works. An error mcpshell raises itself (a
// type mismatch, a failed command) stays a message string in catch, as
// before: docs/design.md "Objects".

var errorClassNames = []string{"TypeError", "RangeError", "SyntaxError", "ReferenceError", "EvalError", "URIError"}

// errorClasses makes Error and its subclasses, sharing one Error.prototype.
func errorClasses() map[string]*FuncVal {
	out := map[string]*FuncVal{}
	base := nativeErrorClass("Error", nil)
	out["Error"] = base
	for _, n := range errorClassNames {
		out[n] = nativeErrorClass(n, base)
	}
	return out
}

func nativeErrorClass(name string, parent *FuncVal) *FuncVal {
	cls := &FuncVal{Name: name, kind: kindClass, Params: []string{"message", "options"}}
	cls.Body = &NativeBody{Fn: func([]Value) Value { panic(Runtime(name + " is constructed with new " + name + "(message)")) }}
	info := &classInfo{native: initError}
	cls.Class = info
	proto := NewObject()
	proto.defineHidden("constructor", cls)
	proto.defineHidden("name", Str(name))
	proto.defineHidden("message", Str(""))
	statics := cls.props()
	if parent != nil {
		info.parent, info.derived = parent, true
		proto.proto = parent.prototypeObj()
		statics.proto = parent.props()
	} else {
		proto.errorBase = true
		proto.defineHidden("toString", &FuncVal{Name: "toString", Body: &NativeBody{
			Fn:   func([]Value) Value { return Str("Error") },
			This: func(this Value, _ []Value) Value { return Str(errorText(this)) },
		}})
	}
	statics.defineHidden("prototype", proto)
	return cls
}

// initError is the Error constructor: message and cause are own,
// non-enumerable properties, as in JS; stack is the "Name: message" line
// (there are no JS frames to list).
func initError(_ *Visitor, this *ObjectVal, args []Value) {
	if len(args) > 0 && args[0] != Null {
		this.defineHidden("message", Str(ToDisplayString(args[0])))
	}
	if len(args) > 1 {
		if o, ok := args[1].(*ObjectVal); ok {
			if c, has := o.Get("cause"); has {
				this.defineHidden("cause", c)
			}
		}
	}
	this.defineAccessor("stack", stackGetter, nil, false)
}

// stackGetter is err.stack, read when used so a name set after construction
// (this.name = "NotFound") shows: the "Name: message" line, since there are
// no JS frames to list.
var stackGetter = &FuncVal{Name: "stack", Body: &NativeBody{
	Fn:   func([]Value) Value { return Str("Error") },
	This: func(this Value, _ []Value) Value { return Str(errorText(this)) },
}}

// errorText is Error.prototype.toString: "Name: message", or the one that is
// not empty.
func errorText(v Value) string {
	o, ok := v.(*ObjectVal)
	if !ok {
		return "Error"
	}
	name, msg := "Error", ""
	if n, ok := o.Get("name"); ok && n != Null {
		name = ToDisplayString(n)
	}
	if m, ok := o.Get("message"); ok && m != Null {
		msg = ToDisplayString(m)
	}
	switch {
	case msg == "":
		return name
	case name == "":
		return msg
	}
	return name + ": " + msg
}

// isErrorObject reports whether o's chain reaches Error.prototype.
func (o *ObjectVal) isErrorObject() bool {
	for p := o; p != nil; p = p.proto {
		if p.errorBase {
			return true
		}
	}
	return false
}

// thrownMessage is the error text of a thrown non-string value: an Error's
// "Name: message", anything else as it prints.
func thrownMessage(v Value) string {
	if o, ok := v.(*ObjectVal); ok && o.isErrorObject() {
		return errorText(o)
	}
	return "Uncaught " + v.Inspect()
}

// ToDisplayString is String(v) as JS gives it: a class's own toString (and an
// Error's) is called, other values print as they display.
func ToDisplayString(v Value) string {
	switch x := v.(type) {
	case *StringVal:
		return x.V
	case *ObjectVal:
		if x.proto != nil {
			if f, ok := x.proto.rawOwnChain("toString"); ok {
				if fn, isFn := f.(*FuncVal); isFn {
					return ToDisplayString(CallThis(fn, x, nil))
				}
			}
		}
	}
	return v.Display()
}

// errorInspect is how an Error object prints: its text, then any other own
// fields.
func errorInspect(o *ObjectVal, anc *Ancestors) string {
	text := errorText(o)
	var parts []string
	for _, k := range o.keys {
		if k != "name" && k != "message" { // already in the text
			parts = append(parts, k+": "+inspectIn(o.m[k], anc))
		}
	}
	if len(parts) == 0 {
		return text
	}
	return text + " {" + strings.Join(parts, ", ") + "}"
}
