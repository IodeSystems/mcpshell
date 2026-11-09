package runtime

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/iodesystems/mcpshell/parser"
)

// Binding names a script cannot write (they are not identifiers): the this
// of the running function, the home object super looks up from, and a
// class's private name.
const (
	thisBinding = "%this"
	homeBinding = "%home"
	privPrefix  = "%priv"
)

// ctorState is a derived constructor's this before and after super(): this
// is unusable until super() has run, as in JS.
type ctorState struct {
	cls    *FuncVal
	this   *ObjectVal
	called bool
}

func (*ctorState) isValue()          {}
func (*ctorState) TypeName() string  { return "object" }
func (*ctorState) IsTruthy() bool    { return true }
func (c *ctorState) Display() string { return c.this.Display() }
func (c *ctorState) Inspect() string { return c.this.Inspect() }

// currentThis is the value of `this` here: null at the top level and in a
// plain call, the receiver in a method call.
func (v *Visitor) currentThis() Value {
	switch t := v.env.Get(thisBinding).(type) {
	case nil:
		return Null
	case *ctorState:
		if !t.called {
			panic(Runtime("Must call super(…) in the constructor of " + t.cls.Name + " before using 'this' (a derived class's this is made by the parent constructor)"))
		}
		return t.this
	default:
		return t
	}
}

// --- classes -----------------------------------------------------------------

func (v *Visitor) visitClassDecl(ctx *parser.ClassDeclContext) Value {
	name := identOrFunctionText(ctx.IDENTIFIER(), ctx.FUNCTION())
	cls := v.evalClass(name, ctx.ClassTail().(*parser.ClassTailContext))
	v.checkTopName(name, ctx)
	v.env.DefineOrSet(name, cls)
	return Null
}

func (v *Visitor) visitClassExpr(ctx *parser.ClassExprContext) Value {
	name := identOrFunctionTextOrEmpty(ctx.IDENTIFIER(), ctx.FUNCTION())
	return v.evalClass(name, ctx.ClassTail().(*parser.ClassTailContext))
}

// memberMods reads the modifiers before a class member name.
type memberMods struct{ static, async, get, set bool }

func (v *Visitor) readMods(children []antlr.Tree, where string) memberMods {
	var m memberMods
	for _, c := range children {
		tn, ok := c.(antlr.TerminalNode)
		if !ok {
			break // the element name
		}
		switch tn.GetText() {
		case "static":
			m.static = true
		case "async":
			m.async = true
		case "get":
			m.get = true
		case "set":
			m.set = true
		default:
			panic(Runtime(fmt.Sprintf("'%s' before %s is not a modifier. Class members take static, async, get and set; end a field with ; when the next line starts a new one", tn.GetText(), where)))
		}
	}
	if m.get && m.set || (m.get || m.set) && m.async {
		panic(Runtime("a class member is a getter, a setter or a method, not two of them: " + where))
	}
	return m
}

// elementName is a member's name: an identifier or keyword, a string, a
// number, a #private name, or a computed [expression].
func (v *Visitor) elementName(ctx parser.IClassElementNameContext) (name string, private bool) {
	switch {
	case ctx.PRIVATE_NAME() != nil:
		return ctx.PRIVATE_NAME().GetText(), true
	case ctx.FieldName() != nil:
		return v.fieldNameText(ctx.FieldName()), false
	default:
		return propertyKey(v.eval(ctx.Expression()), "computed class member name"), false
	}
}

// propertyKey is the string key of a value used as a property name.
func propertyKey(k Value, what string) string {
	switch x := k.(type) {
	case *StringVal:
		return x.V
	case *NumberVal:
		return x.Display()
	case *BoolVal, *NullVal:
		return x.Display()
	}
	panic(TypeMismatch(what, "string or number", k, ""))
}

func (v *Visitor) evalClass(name string, tail *parser.ClassTailContext) *FuncVal {
	info := &classInfo{}
	cls := &FuncVal{Name: name, kind: kindClass, Class: info}
	cls.Body = &NativeBody{Fn: func([]Value) Value {
		panic(Runtime("Class constructor " + nameOr(name, "<anonymous>") + " cannot be invoked without 'new'"))
	}}
	var protoParent, staticParent *ObjectVal
	if tail.EXTENDS() != nil {
		pv := v.eval(tail.PostfixExpr())
		switch p := pv.(type) {
		case *FuncVal:
			if !p.constructible() {
				panic(Runtime(fmt.Sprintf("Class %s extends %s, which is not a constructor (a class or a function declared with function)", nameOr(name, "<anonymous>"), p.Display())))
			}
			info.parent, info.derived = p, true
			protoParent, staticParent = p.prototypeObj(), p.props()
		case *ObjectVal:
			if n := builtinName(v, p); n == "Object" {
				break // extends Object is a base class
			} else if n != "" {
				panic(Runtime(fmt.Sprintf("Class %s cannot extend %s here: keep one in a field instead (this.items = new %s())", nameOr(name, "<anonymous>"), n, n)))
			}
			panic(Runtime("Class " + nameOr(name, "<anonymous>") + " extends an object, which is not a constructor"))
		default:
			panic(TypeMismatch("class extends", "class or constructor function", pv, ""))
		}
	}
	classEnv := v.env.Child()
	if name != "" {
		classEnv.Define(name, cls)
	}
	proto := NewObject()
	proto.proto = protoParent
	proto.defineHidden("constructor", cls)
	statics := cls.props()
	statics.proto = staticParent
	statics.defineHidden("prototype", proto)

	members := tail.AllClassMember()
	// Private names are known to every member, whatever their order.
	declared := map[string]bool{}
	for _, m := range members {
		var en parser.IClassElementNameContext
		switch c := m.(type) {
		case *parser.ClassMethodContext:
			en = c.ClassElementName()
		case *parser.ClassFieldContext:
			en = c.ClassElementName()
		}
		if en != nil && en.PRIVATE_NAME() != nil {
			if info.brand == nil {
				info.brand = &brandVal{class: nameOr(name, "<anonymous>"), methods: map[string]Value{}}
			}
			pn := en.PRIVATE_NAME().GetText()
			if declared[pn] {
				continue // a getter and setter pair
			}
			declared[pn] = true
			classEnv.Define(privPrefix+pn, info.brand)
		}
	}
	method := func(c *parser.ClassMethodContext, mname string, home *ObjectVal, async bool) *FuncVal {
		params, defaults, nodes := v.extractParams(c.ParamList())
		return &FuncVal{
			Name: mname, Params: params, ParamDefaults: defaults, ParamNodes: nodes,
			Body:  &BlockBody{Node: c.Block(), CapturedEnv: classEnv, Commands: v.itp.commands, Limits: v.itp.limits},
			Async: async, kind: kindMethod, Home: home,
		}
	}
	type staticInit struct {
		field *classField
		block parser.IBlockContext
	}
	var statInits []staticInit
	for _, m := range members {
		switch c := m.(type) {
		case *parser.ClassMethodContext:
			mname, private := v.elementName(c.ClassElementName())
			mods := v.readMods(c.GetChildren(), "method "+mname)
			home := proto
			if mods.static {
				home = statics
			}
			fn := method(c, mname, home, mods.async)
			switch {
			case mname == "constructor" && !mods.static && !private:
				if mods.get || mods.set || mods.async {
					panic(Runtime("a class constructor cannot be a getter, setter or async"))
				}
				if info.ctor != nil {
					panic(Runtime("a class has one constructor; " + nameOr(name, "this class") + " has two"))
				}
				info.ctor = fn
			case private && mods.static:
				statics.addBrand(info.brand)
				if mods.get || mods.set {
					panic(Runtime("static private getters and setters are not supported: use a static private method"))
				}
				statics.priv[info.brand][mname] = fn
			case private:
				if mods.get || mods.set {
					acc, _ := info.brand.methods[mname].(*accessorVal)
					if acc == nil {
						acc = &accessorVal{}
					}
					if mods.get {
						acc.get = fn
					} else {
						acc.set = fn
					}
					info.brand.methods[mname] = acc
				} else {
					info.brand.methods[mname] = fn
				}
			case mods.get:
				home.defineAccessor(mname, fn, nil, false)
			case mods.set:
				home.defineAccessor(mname, nil, fn, false)
			default:
				home.defineHidden(mname, fn)
			}
		case *parser.ClassFieldContext:
			fname, private := v.elementName(c.ClassElementName())
			mods := v.readMods(c.GetChildren(), "field "+fname)
			if mods.async || mods.get || mods.set {
				panic(Runtime("a field takes only static; '" + fname + "' has async, get or set (a method needs parentheses: " + fname + "() {…})"))
			}
			f := classField{name: fname, private: private, init: c.Expression(), env: classEnv, home: proto}
			if mods.static {
				f.home = statics
				statInits = append(statInits, staticInit{field: &f})
			} else {
				info.fields = append(info.fields, f)
			}
		case *parser.ClassStaticBlockContext:
			if c.IDENTIFIER().GetText() != "static" {
				panic(Runtime("a block in a class body must be a static block: static { … }"))
			}
			statInits = append(statInits, staticInit{block: c.Block()})
		}
	}
	// Static fields and blocks run once, in order, with this = the class.
	for _, si := range statInits {
		if si.block != nil {
			env := classEnv.Child()
			env.Define(thisBinding, cls)
			env.Define(homeBinding, statics)
			outer := v.env
			v.env = env
			func() {
				defer func() { v.env = outer }()
				v.visitBlock(si.block)
			}()
			continue
		}
		v.initField(statics, info.brand, si.field, cls)
	}
	return cls
}

// initField evaluates a field's initializer with this bound and defines it
// on target (an own property even when a setter exists up the chain, as a JS
// class field is).
func (v *Visitor) initField(target *ObjectVal, brand *brandVal, f *classField, this Value) {
	var val Value = Null
	if f.init != nil {
		env := f.env.Child()
		env.Define(thisBinding, this)
		env.Define(homeBinding, f.home)
		outer := v.env
		v.env = env
		func() {
			defer func() { v.env = outer }()
			val = v.eval(f.init.(parser.IExpressionContext))
		}()
	}
	if f.private {
		target.addBrand(brand)
		target.priv[brand][f.name] = val
		return
	}
	target.defineOwn(f.name, val)
}

// defineOwn makes k an own enumerable data property without calling a
// setter.
func (o *ObjectVal) defineOwn(k string, val Value) {
	if _, ok := o.m[k]; ok {
		if o.hidden[k] {
			delete(o.hidden, k)
			o.keys = append(o.keys, k)
		}
		o.m[k] = val
		return
	}
	o.keys = append(o.keys, k)
	o.m[k] = val
}

// --- new ---------------------------------------------------------------------

// construct runs new callee(args).
func (v *Visitor) construct(callee *FuncVal, full string, args []Value, ctx antlr.ParserRuleContext) Value {
	v.step(ctx)
	if !callee.constructible() {
		what := "a method or arrow function"
		if _, native := callee.Body.(*NativeBody); native {
			what = "a builtin function"
		}
		panic(Runtime(fmt.Sprintf("%s is not a constructor: it is %s. new works on a class or a function declared with function", full, what)))
	}
	this := NewObjectWithProto(callee.prototypeObj())
	if callee.Class == nil {
		r := v.callWith(callee, this, args)
		if v.explicitReturn {
			if obj := objectResult(r); obj != nil {
				return obj
			}
		}
		return this
	}
	if r := v.initialize(callee, this, args); r != nil {
		return r
	}
	return this
}

// objectResult is r when a constructor returned an object (which new then
// returns instead of this, as in JS), else nil.
func objectResult(r Value) Value {
	switch r.(type) {
	case *ObjectVal, *ArrayVal, *FuncVal, *MapVal:
		return r
	}
	return nil
}

// initialize runs class cls's constructor on this: a base class's fields,
// then its constructor body; a derived class's body, whose super(…) runs the
// parent and then this class's fields.
func (v *Visitor) initialize(cls *FuncVal, this *ObjectVal, args []Value) Value {
	ci := cls.Class
	if ci.native != nil {
		ci.native(v, this, args)
		return nil
	}
	if !ci.derived {
		v.initFields(cls, this)
		if ci.ctor == nil {
			return nil
		}
		r := v.callWith(ci.ctor, this, args)
		if !v.explicitReturn {
			return nil
		}
		return objectResult(r)
	}
	if ci.ctor == nil {
		v.superConstruct(cls, this, args)
		return nil
	}
	st := &ctorState{cls: cls, this: this}
	r := v.callSetup(ci.ctor, this, args, func(env *Environment) {
		env.Define(thisBinding, st)
		env.Define(homeBinding, ci.ctor.Home)
	})
	if obj := objectResult(r); obj != nil && v.explicitReturn {
		return obj
	}
	if !st.called {
		panic(Runtime("the constructor of " + cls.Name + " must call super(…) before it returns: a derived class's this is made by the parent constructor"))
	}
	return nil
}

func (v *Visitor) initFields(cls *FuncVal, this *ObjectVal) {
	ci := cls.Class
	if ci.brand != nil {
		this.addBrand(ci.brand)
	}
	for i := range ci.fields {
		v.initField(this, ci.brand, &ci.fields[i], this)
	}
}

// superConstruct is super(args): the parent constructor on this, then this
// class's fields.
func (v *Visitor) superConstruct(cls *FuncVal, this *ObjectVal, args []Value) {
	parent := cls.Class.parent.(*FuncVal)
	if parent.Class != nil {
		if r := v.initialize(parent, this, args); r != nil && r != Value(this) {
			panic(Runtime("the parent constructor of " + cls.Name + " returned another object; a derived class here keeps the this made by new"))
		}
	} else {
		v.callWith(parent, this, args)
	}
	v.initFields(cls, this)
}

// visitSuperCall runs super(args) in a derived constructor.
func (v *Visitor) superCall(args []Value) Value {
	st, ok := v.env.Get(thisBinding).(*ctorState)
	if !ok {
		panic(Runtime("super(…) is only valid in the constructor of a class that extends another"))
	}
	if st.called {
		panic(Runtime("super(…) was already called in this constructor"))
	}
	v.superConstruct(st.cls, st.this, args)
	st.called = true
	return Null
}

// superMember is super.name: the parent's member, with this kept.
func (v *Visitor) superMember(name string) (Value, Value) {
	home, ok := v.env.Get(homeBinding).(*ObjectVal)
	if !ok {
		panic(Runtime("'super' is only valid inside a method (a class or object literal method)"))
	}
	this := v.currentThis()
	if home.proto == nil {
		return Null, this
	}
	val, found := home.proto.getWith(name, this)
	if !found {
		return Null, this
	}
	return val, this
}

// --- private members -------------------------------------------------------

func (v *Visitor) privateBrand(name string) *brandVal {
	b, ok := v.env.Get(privPrefix + name).(*brandVal)
	if !ok {
		panic(Runtime("private name " + name + " is not declared in an enclosing class (declare it in the class body: " + name + " = …)"))
	}
	return b
}

// privateHolder is the object that holds private members for value obj: the
// object itself, or a class's static property object.
func privateHolder(obj Value) *ObjectVal {
	switch o := obj.(type) {
	case *ObjectVal:
		return o
	case *FuncVal:
		return o.props()
	}
	return nil
}

func (v *Visitor) readPrivate(obj Value, name string) Value {
	b := v.privateBrand(name)
	h := privateHolder(obj)
	if h == nil || !h.hasBrand(b) {
		panic(Runtime(fmt.Sprintf("cannot read private member %s from %s: it was not made by class %s", name, describeReceiver(obj), b.class)))
	}
	if val, ok := h.priv[b][name]; ok {
		return val
	}
	if m, ok := b.methods[name]; ok {
		if acc, isAcc := m.(*accessorVal); isAcc {
			return acc.read(obj)
		}
		return m
	}
	panic(Runtime(fmt.Sprintf("class %s has no private member %s", b.class, name)))
}

func (v *Visitor) writePrivate(obj Value, name string, val Value) {
	b := v.privateBrand(name)
	h := privateHolder(obj)
	if h == nil || !h.hasBrand(b) {
		panic(Runtime(fmt.Sprintf("cannot write private member %s to %s: it was not made by class %s", name, describeReceiver(obj), b.class)))
	}
	if _, ok := h.priv[b][name]; ok {
		h.priv[b][name] = val
		return
	}
	if acc, ok := b.methods[name].(*accessorVal); ok {
		acc.put(obj, name, val)
		return
	}
	if _, ok := b.methods[name]; ok {
		panic(Runtime("private method " + name + " cannot be assigned"))
	}
	panic(Runtime(fmt.Sprintf("class %s has no private field %s; declare it in the class body (%s = …)", b.class, name, name)))
}

func describeReceiver(v Value) string {
	if o, ok := v.(*ObjectVal); ok {
		if c := o.className(); c != "" {
			return "an instance of " + c
		}
	}
	return "a " + v.TypeName()
}

// --- instanceof ----------------------------------------------------------------

// instanceOf is left instanceof right.
func (v *Visitor) instanceOf(left, right Value) bool {
	switch r := right.(type) {
	case *FuncVal:
		if r.Class == nil && !r.constructible() {
			if n := builtinName(v, r); n != "" {
				return builtinInstance(n, left)
			}
			panic(Runtime("the right side of instanceof must be a class or constructor; " + r.Display() + " is not"))
		}
		p := r.prototypeObj()
		var start *ObjectVal
		switch l := left.(type) {
		case *ObjectVal:
			start = l.proto
		case *FuncVal:
			start = l.props().proto
			if start != nil && start.fn != nil { // a class's statics chain: B extends A
				return start.fn == r || v.instanceOf(start.fn, r)
			}
			return false
		default:
			return false
		}
		for q := start; q != nil; q = q.proto {
			if q == p {
				return true
			}
		}
		return false
	case *ObjectVal:
		if n := builtinName(v, r); n != "" {
			return builtinInstance(n, left)
		}
	}
	panic(TypeMismatch("instanceof", "class or constructor", right, "the right side of instanceof must be a class (x instanceof Point) or a builtin such as Array, Map, Date"))
}

// builtinName names a builtin constructor value (Array, Map, …), or "".
func builtinName(v *Visitor, x Value) string {
	for _, n := range []string{"Array", "Object", "Date", "Map", "Set", "WeakMap", "WeakSet", "Function", "RegExp", "String", "Number", "Boolean", "Promise"} {
		if g := v.env.Get(n); g != nil && g == x {
			return n
		}
	}
	return ""
}

func builtinInstance(name string, x Value) bool {
	switch name {
	case "Array":
		_, ok := x.(*ArrayVal)
		return ok
	case "Object":
		switch o := x.(type) {
		case *ObjectVal:
			return o.proto != nil || !o.nullProto
		case *ArrayVal, *FuncVal, *MapVal, *DateVal, *RegexVal:
			return true
		}
	case "Date":
		_, ok := x.(*DateVal)
		return ok
	case "Map", "Set", "WeakMap", "WeakSet":
		m, ok := x.(*MapVal)
		return ok && m.kindName() == name
	case "Function":
		_, ok := x.(*FuncVal)
		return ok
	case "RegExp":
		_, ok := x.(*RegexVal)
		return ok
	}
	return false // a primitive is not an instance of String, Number or Boolean
}

// --- functions as objects ----------------------------------------------------

// funcMember reads a property of a function: an own one (statics, a
// prototype assigned or made on first use), then name, length, and the
// Function.prototype methods call, apply and bind.
func (v *Visitor) funcMember(f *FuncVal, field string) Value {
	if field == "prototype" {
		if p := f.prototypeObj(); p != nil {
			return p
		}
		return Null
	}
	if val, ok := f.props().getWith(field, f); ok {
		return val
	}
	switch field {
	case "name":
		return Str(strings.TrimPrefix(nameOr(f.Name, ""), "<anonymous>"))
	case "length":
		n := 0
		for i := range f.Params {
			if i < len(f.ParamDefaults) && f.ParamDefaults[i] != nil {
				break
			}
			if i < len(f.ParamNodes) && f.ParamNodes[i] != nil {
				if p, ok := f.ParamNodes[i].(parser.IParamContext); ok && p.SPREAD() != nil {
					break
				}
			}
			n++
		}
		return numInt(int64(n))
	case "call":
		return &FuncVal{Name: "call", Params: []string{"thisArg", "...args"}, Body: &NativeBody{Fn: func(args []Value) Value {
			var this Value = Null
			if len(args) > 0 {
				this, args = args[0], args[1:]
			}
			return v.callMethod(f, this, args, nil)
		}}}
	case "apply":
		return &FuncVal{Name: "apply", Params: []string{"thisArg", "args"}, Body: &NativeBody{Fn: func(args []Value) Value {
			var this Value = Null
			var rest []Value
			if len(args) > 0 {
				this = args[0]
			}
			if len(args) > 1 {
				switch a := args[1].(type) {
				case *ArrayVal:
					rest = a.Elements
				case *NullVal:
				default:
					panic(TypeMismatch("apply", "array of arguments", args[1], "f.apply(thisArg, [a, b])"))
				}
			}
			return v.callMethod(f, this, rest, nil)
		}}}
	case "bind":
		return &FuncVal{Name: "bind", Params: []string{"thisArg", "...args"}, Body: &NativeBody{Fn: func(args []Value) Value {
			var this Value = Null
			var pre []Value
			if len(args) > 0 {
				this, pre = args[0], append([]Value(nil), args[1:]...)
			}
			return &FuncVal{Name: "bound " + strings.TrimPrefix(nameOr(f.Name, ""), "<anonymous>"), Body: &NativeBody{Fn: func(more []Value) Value {
				return v.callMethod(f, this, append(append([]Value(nil), pre...), more...), nil)
			}}}
		}}}
	case "toString":
		return &FuncVal{Name: "toString", Body: &NativeBody{Fn: func([]Value) Value { return Str(f.Display()) }}}
	}
	return Null
}

// superPrimary evaluates super and its first postfix op: super(args) in a
// constructor, or super.name / super[key] in a method. It returns the value,
// and this as the receiver for a call that follows.
func (v *Visitor) superPrimary(ops []parser.IPostfixOpContext) (Value, Value, string) {
	if len(ops) == 0 {
		panic(Runtime("'super' must be called, super(…), or read, super.method(…)"))
	}
	op := ops[0].(*parser.PostfixOpContext)
	var result, this Value
	var name string
	v.withLocation(op, func() Value {
		switch {
		case op.LPAREN() != nil:
			var ca callArgs
			if al := op.ArgumentList(); al != nil {
				ca = v.evalCallArgs(al)
			}
			if ca.hasNamed() {
				panic(Runtime("super(…) takes positional arguments"))
			}
			result = v.superCall(ca.positional)
		case op.FieldName() != nil:
			name = v.fieldNameText(op.FieldName())
			result, this = v.superMember(name)
		case op.LBRACKET() != nil && op.OPTIONAL_CHAIN() == nil:
			name = propertyKey(v.eval(op.Expression()), "super[key]")
			result, this = v.superMember(name)
		default:
			panic(Runtime("'super' must be called, super(…), or read, super.method(…)"))
		}
		return nil
	})
	return result, this, name
}

// setFuncMember assigns a property of a function. F.prototype = obj replaces
// the prototype that new F() uses.
func (v *Visitor) setFuncMember(f *FuncVal, name string, val Value) {
	if _, native := f.Body.(*NativeBody); native && f.Class == nil && f.obj == nil {
		panic(Runtime("cannot set ." + name + " on the builtin " + f.Display()))
	}
	if name == "prototype" {
		p, ok := val.(*ObjectVal)
		if !ok {
			panic(TypeMismatch("assignment to .prototype", "object", val, ""))
		}
		f.props().defineHidden("prototype", p)
		return
	}
	f.props().Set(name, val)
}
