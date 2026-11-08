package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func omShell() *runtime.Shell { return toolkit.InstallCore(runtime.NewShell()) }

// An exported class keeps its identity in the next eval (the MCP server runs
// each call as its own eval): instanceof and Error subclasses still match.
func TestClassAcrossEvals(t *testing.T) {
	sh := omShell()
	steps := []struct{ src, want string }{
		{`export class Acct { #bal = 0; deposit(n) { this.#bal += n; return this } get balance() { return this.#bal } }`, "null"},
		{`export let a = new Acct().deposit(5); a.balance`, "5"},
		{`a.deposit(2); [a.balance, a instanceof Acct]`, "[7, true]"},
		{`export class Oops extends Error {}`, "null"},
		{`let r; try { throw new Oops("x") } catch (e) { r = [e instanceof Oops, e instanceof Error, e.message] }; r`, `[true, true, "x"]`},
		{`export let m = new Map([["k", 1]]); m.size`, "1"},
		{`m.set("j", 2); [...m.keys()]`, `["k", "j"]`},
	}
	for _, s := range steps {
		v, err := sh.EvalExported(s.src, nil)
		if err != nil || v.Display() != s.want {
			t.Fatalf("%s = %v, %v; want %s", s.src, v, err, s.want)
		}
	}
}

func TestObjectModelBehavior(t *testing.T) {
	for src, want := range map[string]string{
		`this`:                              "null",
		`function f() { return this }; f()`: "null",
		`let o = {n: 1, m() { return () => this.n }}; o.m()()`:                                                 "1",
		`class A { #x = 1 }; let a = new A(); a.self = a; a`:                                                   "A {self: [Circular]}",
		`class E extends Error {}; async function f() { throw new E("late") }; f().catch(e => e instanceof E)`: "true",
		`let r = allSettled(() => { throw new RangeError("r") }); r[0].reason instanceof RangeError`:           "true",
		`class A {}; [new A() == new A(), new A() == {}]`:                                                      "[true, false]",
		`let s = new Set([1, 2]); [s |> len, new Map() |> len]`:                                                "[2, 0]",
		`let f = function () {}; [f.prototype.constructor === f, (() => 1).prototype]`:                         "[true, null]",
		`class A { static k = this.name + "!" }; A.k`:                                                          `A!`,
		`class A extends Object { x = 1 }; new A()`:                                                            "A {x: 1}",
		`class A { items = []; add(x) { this.items.push(x); return this } }; new A().add(1).add(2).items`:      "[1, 2]",
		`let o = {list: [1]}; let get = () => o; get().list.push(2); o.list`:                                   "[1, 2]",
		`new Error("m")`: "Error: m",
		`class V extends Error { constructor() { super("v"); this.code = 7 } }; new V()`: "Error: v {code: 7}",
		`new Map([[{a: 1}, [1]]])`: `Map(1) {{a: 1} => [1]}`,
		`Function`:                 "function Function()",
	} {
		v, err := omShell().Eval(src)
		if err != nil || v.Display() != want {
			t.Errorf("%s = %v, %v; want %s", src, v, err, want)
		}
	}
}

// What is not built errors with what to use instead.
func TestObjectModelErrors(t *testing.T) {
	for src, want := range map[string][]string{
		`class A {}; A()`:                                                                  {"without 'new'", "new A(…)"},
		`let f = () => 1; new f()`:                                                         {"not a constructor", "class or a function declared with function"},
		`class A { m() {} }; new (new A().m)()`:                                            {"Syntax error"},
		`class A { foo bar() {} }`:                                                         {"'foo' before method bar is not a modifier"},
		`class B extends Map {}`:                                                           {"cannot extend Map", "field"},
		`class A { m() { return this.#x } }; new A().m()`:                                  {"#x is not declared in an enclosing class"},
		`class A { #x = 1; static r(o) { return o.#x } }; A.r({})`:                         {"cannot read private member #x", "not made by class A"},
		`class A {}; class B extends A { constructor() {} }; new B()`:                      {"must call super(…) before it returns"},
		`class A {}; class B extends A { constructor() { this.x = 1; super() } }; new B()`: {"before using 'this'"},
		`class A { get v() { return 1 } }; let a = new A(); a.v = 2`:                       {"only a getter", "set v(v)"},
		`new Map(5)`:                         {"iterable"},
		`new Map({a: 1})`:                    {"new Map(Object.entries(obj))"},
		`new Map().foo`:                      {"Map has no member 'foo'", "get, set, has"},
		`new WeakMap().size`:                 {"cannot be listed", "Use a Map"},
		`new WeakSet().add(1)`:               {"Invalid value used as a WeakSet key"},
		`Map()`:                              {"requires 'new'"},
		`Object.create({}, {x: {value: 1}})`: {"descriptors) is not supported"},
		`new Set([1]) |> sort`:               {"[...s] |> sort"},
		`for (const x of {a: 1}) {}`:         {"Object.entries(obj)"},
		`Function("return 1")`:               {"cannot make a function from a string"},
		`let o = {get x(a) { return 1 }}`:    {"a getter takes no parameters"},
		`throw new TypeError("bad")`:         {"TypeError: bad"},
		`throw {code: 1}`:                    {"Uncaught {code: 1}"},
		`1 instanceof 2`:                     {"right side of instanceof"},
	} {
		_, err := omShell().Eval(src)
		if err == nil {
			t.Errorf("%s: want an error", src)
			continue
		}
		for _, w := range want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: error %q lacks %q", src, err.Error(), w)
			}
		}
	}
}
