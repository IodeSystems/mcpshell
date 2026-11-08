package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// objectParity: classes, prototypes, this, Map/Set and Error, each case
// checked against Node 24 (the want is what Node printed for the same
// source). Regenerate by running the sources in Node when adding a case.
var objectParity = []struct{ name, src, want string }{
	{"class basics", `class P { constructor(x, y) { this.x = x; this.y = y } sum() { return this.x + this.y } }
const p = new P(2, 3)
JSON.stringify([p.sum(), p.x, Object.keys(p), typeof P, typeof p, p instanceof P, P.name])`, `[5,2,["x","y"],"function","object",true,"P"]`},
	{"getter setter", `class T { #c = 0; get c() { return this.#c } set c(v) { if (v < 0) throw new RangeError("neg"); this.#c = v } }
const t = new T(); t.c = 5; let err; try { t.c = -1 } catch (e) { err = [e.name, e.message, e instanceof RangeError, e instanceof Error] }
JSON.stringify([t.c, err, Object.keys(t), JSON.stringify(t)])`, `[5,["RangeError","neg",true,true],[],"{}"]`},
	{"static members", `class C { static count = 0; static make() { C.count++; return new C() } static { C.ready = true } }
C.make(); C.make()
JSON.stringify([C.count, C.ready, Object.keys(new C())])`, `[2,true,[]]`},
	{"inheritance and super", `class A { constructor(n) { this.n = n } hi() { return "A" + this.n } static s() { return "sA" } }
class B extends A { constructor(n) { super(n * 2); this.b = true } hi() { return "B" + super.hi() } static s() { return "sB" + super.s() } }
const b = new B(3)
JSON.stringify([b.hi(), b.n, b.b, B.s(), b instanceof A, b instanceof B, Object.getPrototypeOf(B) === A, Object.keys(b)])`, `["BA6",6,true,"sBsA",true,true,true,["n","b"]]`},
	{"derived default ctor", `class A { constructor(a, b) { this.v = a + b } }
class B extends A {}
JSON.stringify([new B(1, 2).v, new B(1, 2) instanceof A])`, `[3,true]`},
	{"field init order", `const log = []
class A { a = log.push("A field"); constructor() { log.push("A ctor") } }
class B extends A { b = log.push("B field"); constructor() { log.push("B before super"); super(); log.push("B ctor") } }
new B()
JSON.stringify(log)`, `["B before super","A field","A ctor","B field","B ctor"]`},
	{"this before super", `class A {}
class B extends A { constructor() { this.x = 1; super() } }
let msg = "none"; try { new B() } catch (e) { msg = "threw" }
JSON.stringify(msg)`, `"threw"`},
	{"private methods and static private", `class K { #n; static #made = 0; constructor(n) { this.#n = n; K.#made++ } #double() { return this.#n * 2 } get d() { return this.#double() } static made() { return K.#made } }
new K(1); const k = new K(4)
JSON.stringify([k.d, K.made(), Object.keys(k), JSON.stringify(k)])`, `[8,2,[],"{}"]`},
	{"private brand check", `class A { #x = 1; static read(o) { return o.#x } }
let r; try { A.read({}) ; r = "no" } catch (e) { r = "threw" }
JSON.stringify([A.read(new A()), r])`, `[1,"threw"]`},
	{"class expression and computed names", `const key = "dyn"
const C = class Named { [key]() { return "d" } ["a" + "b"] = 3; who() { return Named.name } }
const c = new C()
JSON.stringify([c.dyn(), c.ab, c.who()])`, `["d",3,"Named"]`},
	{"arrow field captures this", `class Btn { label = "ok"; click = () => this.label }
const f = new Btn().click
JSON.stringify(f())`, `"ok"`},
	{"method this in callbacks", `class S { k = 3; scale(xs) { return xs.map(x => x * this.k) } }
JSON.stringify(new S().scale([1, 2]))`, `[3,6]`},
	{"constructor function and prototype", `function Animal(name) { this.name = name }
Animal.prototype.speak = function () { return this.name + " speaks" }
function Dog(name) { Animal.call(this, name); this.kind = "dog" }
Dog.prototype = Object.create(Animal.prototype)
Dog.prototype.constructor = Dog
Dog.prototype.bark = function () { return this.name + " barks" }
const d = new Dog("rex")
const keys = []; for (const k in d) keys.push(k)
JSON.stringify([d.speak(), d.bark(), d instanceof Dog, d instanceof Animal, keys, Object.keys(d)])`, `["rex speaks","rex barks",true,true,["name","kind","constructor","bark","speak"],["name","kind"]]`},
	{"for in skips class methods", `class A { x = 1; m() {} }
const ks = []; for (const k in new A()) ks.push(k)
JSON.stringify(ks)`, `["x"]`},
	{"constructor returns object", `function F() { this.a = 1; return { b: 2 } }
function G() { this.a = 1; return 5 }
JSON.stringify([new F(), new G()])`, `[{"b":2},{"a":1}]`},
	{"call apply bind", `function f(a, b) { return [this.v, a, b] }
const o = { v: 1 }
const g = f.bind(o, 10)
JSON.stringify([f.call(o, 2, 3), f.apply(o, [4, 5]), g(6), f.length, g.name])`, `[[1,2,3],[1,4,5],[1,10,6],2,"bound f"]`},
	{"object literal accessors and super", `const base = { greet() { return "base" } }
const o = { _v: 2, get v() { return this._v * 10 }, set v(x) { this._v = x }, greet() { return "o+" + super.greet() } }
Object.setPrototypeOf(o, base)
o.v = 5
JSON.stringify([o.v, o.greet(), Object.keys(o)])`, `[50,"o+base",["_v","v","greet"]]`},
	{"Object.create null and getPrototypeOf", `const proto = { hello() { return "hi " + this.n } }
const o = Object.create(proto); o.n = "x"
const bare = Object.create(null); bare.k = 1
JSON.stringify([o.hello(), Object.getPrototypeOf(o) === proto, proto.isPrototypeOf(o), Object.keys(bare), bare instanceof Object, o instanceof Object])`, `["hi x",true,true,["k"],false,true]`},
	{"toString in templates and String", "class V { constructor(x) { this.x = x } toString() { return \"V(\" + this.x + \")\" } }\nconst v = new V(7)\nJSON.stringify([`${v}`, String(v), \"a\" + v])", `["V(7)","V(7)","aV(7)"]`},
	{"error subclass", `class NotFound extends Error { constructor(what) { super(what + " missing"); this.name = "NotFound"; this.what = what } }
let out; try { throw new NotFound("user") } catch (e) { out = [e instanceof NotFound, e instanceof Error, e.name, e.message, e.what, String(e), Object.keys(e)] }
JSON.stringify(out)`, `[true,true,"NotFound","user missing","user","NotFound: user missing",["name","what"]]`},
	{"error without new and cause", `const e = Error("boom", { cause: "disk" })
const t = new TypeError("bad")
JSON.stringify([e.message, e.cause, e instanceof Error, t.name, t instanceof TypeError, t instanceof Error, e.toString(), JSON.stringify(e)])`, `["boom","disk",true,"TypeError",true,true,"Error: boom","{}"]`},
	{"throw object", `let got; try { throw { code: 42 } } catch (e) { got = e.code }
JSON.stringify(got)`, `42`},
	{"map basics", `const m = new Map([["a", 1]])
m.set("b", 2).set("c", 3)
m.delete("a")
m.set("a", 9)
JSON.stringify([m.size, m.get("b"), m.has("z"), [...m.keys()], [...m.values()], [...m], Array.from(m.entries())])`, `[3,2,false,["b","c","a"],[2,3,9],[["b",2],["c",3],["a",9]],[["b",2],["c",3],["a",9]]]`},
	{"map identity keys", `const k1 = { id: 1 }, k2 = { id: 1 }
const m = new Map()
m.set(k1, "one"); m.set(k2, "two"); m.set(NaN, "nan"); m.set(1, "num"); m.set("1", "str")
JSON.stringify([m.size, m.get(k1), m.get(k2), m.get({ id: 1 }) === undefined, m.get(NaN), m.get(1), m.get("1")])`, `[5,"one","two",true,"nan","num","str"]`},
	{"map iteration", `const m = new Map(Object.entries({ x: 1, y: 2 }))
const out = []
for (const [k, v] of m) out.push(k + "=" + v)
m.forEach((v, k) => out.push(k + ":" + v))
JSON.stringify([out, Object.fromEntries(m), JSON.stringify(m)])`, `[["x=1","y=2","x:1","y:2"],{"x":1,"y":2},"{}"]`},
	{"set basics", `const s = new Set([3, 1, 3, 2])
s.add(1).add(4)
s.delete(3)
JSON.stringify([s.size, [...s], s.has(4), Array.from(s), [...new Set("hello")].join("")])`, `[3,[1,2,4],true,[1,2,4],"helo"]`},
	{"set algebra", `const a = new Set([1, 2, 3]), b = new Set([2, 3, 4])
JSON.stringify([[...a.union(b)], [...a.intersection(b)], [...a.difference(b)], [...a.symmetricDifference(b)], a.isSubsetOf(new Set([1, 2, 3, 4])), a.isDisjointFrom(new Set([9]))])`, `[[1,2,3,4],[2,3],[1],[1,4],true,true]`},
	{"weakmap", `const wm = new WeakMap(); const k = {}
wm.set(k, 1)
let bad; try { wm.set("s", 1) } catch (e) { bad = "threw" }
JSON.stringify([wm.get(k), wm.has(k), wm.has({}), bad])`, `[1,true,false,"threw"]`},
	{"map groupBy", `const g = Map.groupBy([1, 2, 3, 4, 5], x => x % 2 ? "odd" : "even")
JSON.stringify([...g])`, `[["odd",[1,3,5]],["even",[2,4]]]`},
	{"instanceof builtins", `JSON.stringify([[] instanceof Array, [] instanceof Object, new Map() instanceof Map, new Set() instanceof Map, new Date(0) instanceof Date, (() => 1) instanceof Function, 1 instanceof Number])`, `[true,true,true,false,true,true,false]`},
	{"typeof", `class A {}
JSON.stringify([typeof A, typeof new A(), typeof new Map(), typeof new Set(), typeof Error])`, `["function","object","object","object","function"]`},
	{"static inheritance", `class A { static create() { return new this() } who() { return "A" } }
class B extends A { who() { return "B" } }
JSON.stringify([B.create().who(), B.create() instanceof B])`, `["B",true]`},
	{"inherited getter", `class A { get kind() { return "k:" + this.constructor.name } }
class B extends A {}
JSON.stringify([new B().kind, new A().kind])`, `["k:B","k:A"]`},
	{"class without new", `class A {}
let r; try { A() ; r = "no" } catch (e) { r = "threw" }
JSON.stringify(r)`, `"threw"`},
	{"method chaining builder", `class Q { parts = []; where(c) { this.parts.push(c); return this } build() { return this.parts.join(" AND ") } }
JSON.stringify(new Q().where("a=1").where("b=2").build())`, `"a=1 AND b=2"`},
	{"accumulate in class with map", `class Inventory { #items = new Map(); add(k, n) { this.#items.set(k, (this.#items.get(k) ?? 0) + n); return this } total() { let t = 0; for (const [, n] of this.#items) t += n; return t } get skus() { return [...this.#items.keys()] } }
const inv = new Inventory().add("a", 2).add("b", 3).add("a", 1)
JSON.stringify([inv.total(), inv.skus])`, `[6,["a","b"]]`},
	{"destructuring holes", `const [, second, , fourth] = [1, 2, 3, 4]
const [first, ...rest] = [1, 2, 3]
JSON.stringify([second, fourth, first, rest])`, `[2,4,1,[2,3]]`},
	{"getter only assignment throws", `class A { get v() { return 1 } }
const a = new A(); let r = "no"; try { a.v = 2 } catch (e) { r = "threw" }
JSON.stringify([r, a.v])`, `["threw",1]`},
	{"map -0 and object set", `const m = new Map([[-0, "z"]]); const o = {}; const s = new Set([o, o, {}])
JSON.stringify([m.get(0), s.size, s.has(o)])`, `["z",2,true]`},
	{"super property chain three levels", `class A { v() { return "a" } }
class B extends A { v() { return "b" + super.v() } }
class C extends B { v() { return "c" + super.v() } }
JSON.stringify(new C().v())`, `"cba"`},
	{"instance method shared on prototype", `class A { m() { return 1 } }
const x = new A(), y = new A()
JSON.stringify([x.m === y.m, Object.getPrototypeOf(x) === A.prototype, A.prototype.constructor === A, x.constructor === A])`, `[true,true,true,true]`},
	{"prototype method added later", `class A {}
const a = new A()
A.prototype.late = function () { return "late" }
JSON.stringify(a.late())`, `"late"`},
	{"hasOwnProperty vs in", `class A { x = 1; m() {} }
const a = new A()
JSON.stringify([a.hasOwnProperty("x"), a.hasOwnProperty("m"), "m" in a, "x" in a, "z" in a])`, `[true,false,true,true,false]`},
	{"static this and private static", `class Counter { static #n = 0; static next() { this.#n += 1; return this.#n } }
Counter.next(); Counter.next()
JSON.stringify(Counter.next())`, `3`},
	{"object spread of instance", `class P { constructor() { this.a = 1 } get g() { return 2 } m() {} }
JSON.stringify({ ...new P() })`, `{"a":1}`},
	{"class getter in JSON is skipped", `class P { a = 1; get b() { return 2 } }
JSON.stringify(JSON.stringify(new P()))`, `"{\"a\":1}"`},
	{"literal getter in JSON is read", `const o = { a: 1, get b() { return this.a + 1 } }
JSON.stringify(JSON.stringify(o))`, `"{\"a\":1,\"b\":2}"`},
	{"error stack starts with name", `const e = new RangeError("r")
JSON.stringify([e.stack.split(":")[0], e instanceof RangeError])`, `["RangeError",true]`},
	{"bound method keeps this", `class A { n = 5; get() { return this.n } }
const a = new A(); const g = a.get.bind(a)
JSON.stringify(g())`, `5`},
}

func TestObjectModelNodeParity(t *testing.T) {
	for _, c := range objectParity {
		v, err := toolkit.InstallCore(runtime.NewShell()).Eval(c.src)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if v.Display() != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, v.Display(), c.want)
		}
	}
}
