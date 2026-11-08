package runtime

// The JS object model on top of ObjectVal and FuncVal: prototype chains,
// non-enumerable properties, getters and setters, private fields, and the
// properties of functions (prototype, statics). docs/design.md "Objects"
// says why each part is the way it is.

// accessorVal is a getter/setter pair stored as an object property. It is
// not a value scripts can hold: Get calls the getter, Set calls the setter.
type accessorVal struct{ get, set *FuncVal }

func (*accessorVal) isValue()         {}
func (*accessorVal) TypeName() string { return "accessor" }
func (*accessorVal) IsTruthy() bool   { return true }
func (a *accessorVal) Display() string {
	switch {
	case a.get != nil && a.set != nil:
		return "[Getter/Setter]"
	case a.get != nil:
		return "[Getter]"
	}
	return "[Setter]"
}
func (a *accessorVal) Inspect() string { return a.Display() }

func (a *accessorVal) read(recv Value) Value {
	if a.get == nil {
		return Null
	}
	return CallThis(a.get, recv, nil)
}

func (a *accessorVal) put(recv Value, k string, v Value) {
	if a.set == nil {
		panic(Runtime("Cannot set property " + k + ", which has only a getter (add a set " + k + "(v) to the class or object)"))
	}
	CallThis(a.set, recv, []Value{v})
}

// brandVal is a class's private name space: the private fields and methods
// one class body declares. An object carries the brands of the classes that
// constructed it (ObjectVal.priv). Never a script value.
type brandVal struct {
	class   string
	methods map[string]Value // private methods and accessors (#m() {}, get #x())
}

func (*brandVal) isValue()          {}
func (*brandVal) TypeName() string  { return "brand" }
func (*brandVal) IsTruthy() bool    { return true }
func (b *brandVal) Display() string { return "[class " + b.class + " private names]" }
func (b *brandVal) Inspect() string { return b.Display() }

// receiver is the this of a getter or setter found on o: the function when o
// holds a function's properties, else o.
func (o *ObjectVal) receiver() Value {
	if o.fn != nil {
		return o.fn
	}
	return o
}

// getWith looks k up on o and its prototype chain, calling a getter with
// recv as this.
func (o *ObjectVal) getWith(k string, recv Value) (Value, bool) {
	for p := o; p != nil; p = p.proto {
		if v, ok := p.m[k]; ok {
			if acc, isAcc := v.(*accessorVal); isAcc {
				return acc.read(recv), true
			}
			return v, true
		}
	}
	return nil, false
}

// rawOwn is an own property without calling a getter.
func (o *ObjectVal) rawOwn(k string) (Value, bool) {
	v, ok := o.m[k]
	return v, ok
}

// defineHidden sets an own, non-enumerable property (a class method, a
// prototype's constructor): it is found by Get but not listed by Keys.
func (o *ObjectVal) defineHidden(k string, v Value) {
	if _, ok := o.m[k]; ok && !o.hidden[k] {
		o.Delete(k)
	}
	if o.hidden == nil {
		o.hidden = map[string]bool{}
	}
	o.hidden[k] = true
	o.m[k] = v
}

// defineAccessor adds a getter or setter for k, merging with an existing
// pair; enumerable for an object literal, hidden for a class.
func (o *ObjectVal) defineAccessor(k string, get, set *FuncVal, enumerable bool) {
	acc, ok := o.m[k].(*accessorVal)
	if !ok {
		acc = &accessorVal{}
	}
	if get != nil {
		acc.get = get
	}
	if set != nil {
		acc.set = set
	}
	if _, exists := o.m[k]; exists {
		o.m[k] = acc
		return
	}
	if enumerable {
		o.keys = append(o.keys, k)
		o.m[k] = acc
		return
	}
	o.defineHidden(k, acc)
}

// Proto returns the prototype, or nil.
func (o *ObjectVal) Proto() *ObjectVal { return o.proto }

// SetProto sets the prototype; a chain that would loop is an error, as in JS.
func (o *ObjectVal) SetProto(p *ObjectVal) {
	for q := p; q != nil; q = q.proto {
		if q == o {
			panic(Runtime("Cyclic __proto__ value: the prototype chain would loop back to the object"))
		}
	}
	o.proto = p
}

// NewObjectWithProto is Object.create(proto).
func NewObjectWithProto(p *ObjectVal) *ObjectVal {
	o := NewObject()
	o.proto = p
	return o
}

// className is the name of the constructor on o's chain, for display
// (Point {x: 1}); "" for a plain object or a prototype object itself.
func (o *ObjectVal) className() string {
	if o.proto == nil {
		return ""
	}
	if c, ok := o.proto.rawOwnChain("constructor"); ok {
		if f, isFn := c.(*FuncVal); isFn && f.Name != "" && f.Name != "<anonymous>" && f.Name != "Object" {
			return f.Name
		}
	}
	return ""
}

func (o *ObjectVal) rawOwnChain(k string) (Value, bool) {
	for p := o; p != nil; p = p.proto {
		if v, ok := p.m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// --- private fields --------------------------------------------------------

func (o *ObjectVal) addBrand(b *brandVal) {
	if o.priv == nil {
		o.priv = map[*brandVal]map[string]Value{}
	}
	if o.priv[b] == nil {
		o.priv[b] = map[string]Value{}
	}
}

func (o *ObjectVal) hasBrand(b *brandVal) bool {
	_, ok := o.priv[b]
	return ok
}

// --- functions as objects ----------------------------------------------------

// funcKind tells what a function can do with this and new.
type funcKind int8

const (
	kindFunction funcKind = iota // function f() {}: has this, constructible
	kindArrow                    // () => …: lexical this, not constructible
	kindMethod                   // obj/class method, getter, setter: has this, not constructible
	kindClass                    // class C {}: constructible only with new
)

// classInfo is what a class value knows to construct an instance.
type classInfo struct {
	parent  Value // the extends value: *FuncVal, or nil for a base class
	ctor    *FuncVal
	fields  []classField // instance fields, in order
	brand   *brandVal    // private names of this class body, or nil
	derived bool
	native  func(v *Visitor, this *ObjectVal, args []Value) // a builtin base (Error)
}

type classField struct {
	name    string // "" with key set for a computed name
	private bool
	init    any // parser.IExpressionContext, or nil
	env     *Environment
	home    *ObjectVal
}

// props returns the function's own property object, made on first use.
func (f *FuncVal) props() *ObjectVal {
	if f.obj == nil {
		f.obj = NewObject()
		f.obj.fn = f
	}
	return f.obj
}

// constructible reports whether new f(...) works.
func (f *FuncVal) constructible() bool {
	if f.Class != nil {
		return true
	}
	if _, native := f.Body.(*NativeBody); native {
		return false
	}
	return f.kind == kindFunction
}

// prototypeObj is f.prototype, made on first use for a constructible
// function, with a hidden constructor pointing back to f.
func (f *FuncVal) prototypeObj() *ObjectVal {
	o := f.props()
	if p, ok := o.rawOwn("prototype"); ok {
		po, _ := p.(*ObjectVal)
		return po
	}
	if !f.constructible() {
		return nil
	}
	p := NewObject()
	p.defineHidden("constructor", f)
	o.defineHidden("prototype", p)
	return p
}

// CallThis calls fn with this bound (a method call from Go: a getter, a
// toString). User functions run in a fresh visitor over their captured scope.
func CallThis(fn *FuncVal, this Value, args []Value) Value {
	if nb, ok := fn.Body.(*NativeBody); ok {
		if nb.This != nil {
			return nb.This(this, args)
		}
		return nb.Fn(args)
	}
	var cmds *CommandRegistry
	var lim *ExecutionLimits
	var cenv *Environment
	switch b := fn.Body.(type) {
	case *ExpressionBody:
		cmds, lim, cenv = b.Commands, b.Limits, b.CapturedEnv
	case *BlockBody:
		cmds, lim, cenv = b.Commands, b.Limits, b.CapturedEnv
	default:
		panic(Runtime("not callable"))
	}
	itp := &Interpreter{commands: cmds, globals: cenv, limits: lim}
	return newVisitor(itp, cenv).callWith(fn, this, args)
}

// objectProtoMethod is an Object.prototype method that an object has unless
// it defines its own: toString, valueOf, isPrototypeOf. Only for an object
// with a prototype (an instance, Object.create(p)) or a plain object read as
// a method; a missing key is otherwise null, as before.
func objectProtoMethod(o *ObjectVal, field string) *FuncVal {
	if o.nullProto {
		return nil
	}
	switch field {
	case "toString":
		return &FuncVal{Name: "toString", Body: &NativeBody{
			Fn: func([]Value) Value { return Str("[object Object]") },
			This: func(this Value, _ []Value) Value {
				if t, ok := this.(*ObjectVal); ok && t.isErrorObject() {
					return Str(errorText(t))
				}
				return Str("[object Object]")
			},
		}}
	case "valueOf":
		return &FuncVal{Name: "valueOf", Body: &NativeBody{Fn: func([]Value) Value { return o }}}
	case "isPrototypeOf":
		return &FuncVal{Name: "isPrototypeOf", Params: []string{"object"}, Body: &NativeBody{Fn: func(args []Value) Value {
			if len(args) == 0 {
				return Bln(false)
			}
			x, ok := args[0].(*ObjectVal)
			if !ok {
				return Bln(false)
			}
			for p := x.proto; p != nil; p = p.proto {
				if p == o {
					return Bln(true)
				}
			}
			return Bln(false)
		}}}
	}
	return nil
}
