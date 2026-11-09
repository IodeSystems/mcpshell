package toolkit_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// fileShell returns a shell with core + file toolkits, confined to a temp dir.
func fileShell(t *testing.T, readOnly bool) *runtime.Shell {
	t.Helper()
	return toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), t.TempDir(), readOnly)
}

func TestFileToolkit(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"write then read", `write("a.txt", "hello world"); read("a.txt")`, "1: hello world"},
		{"read numbering", `write("d.txt", "a\nb\nc"); read("d.txt")`, "1: a\n2: b\n3: c"},
		{"readLines", `write("b.txt", "x\ny\nz"); readLines("b.txt")`, `["x", "y", "z"]`},
		{"exists true", `write("c.txt", "1"); exists("c.txt")`, "true"},
		{"exists false", `exists("nope.txt")`, "false"},
		{"append", `write("e.txt", "a"); append("e.txt", "b"); read("e.txt")`, "1: ab"},
		{"edit", `write("f.txt", "const x = 1"); edit("f.txt", "1", "42"); read("f.txt")`, "1: const x = 42"},
		{"edit all", `write("g.txt", "a a a"); edit("g.txt", "a", "b", {all: true}); read("g.txt")`, "1: b b b"},
		{"mkdir nested write", `mkdir("sub/deep"); write("sub/deep/h.txt", "deep"); read("sub/deep/h.txt")`, "1: deep"},
		{"ls", `write("z.txt", "1"); ls(".") |> map(f => f.name)`, `["z.txt"]`},
		{"ls type", `write("z.txt", "1"); ls(".") |> map(f => f.type)`, `["file"]`},
		{"glob", `write("h.txt", "1"); write("i.md", "2"); glob("*.txt")`, `[File("h.txt")]`},
		{"glob recursive", `mkdir("src"); write("src/m.go", "x"); glob("**/*.go")`, `[File("src/m.go")]`},
		{"glob grep", `write("j.txt", "ok\nTODO fix\nok"); glob("*.txt", {grep: "TODO"}) |> map(h => h.num)`, "[2]"},
		{"grep content", `write("k.txt", "a\nTODO\nb"); grep("k.txt", {match: "TODO"}) |> len()`, "1"},
		{"grep count", `write("l.txt", "x\nx\ny"); grep("l.txt", {match: "x", mode: "count"})`, "2"},
		{"grep files", `write("n.txt", "has match"); grep("n.txt", {match: "match", mode: "files"})`, "true"},
		{"deletePath", `write("o.txt", "1"); deletePath("o.txt"); exists("o.txt")`, "false"},
		{"mv", `write("p.txt", "moved"); mv("p.txt", "q.txt"); read("q.txt")`, "1: moved"},
		{"ls row as path", `mkdir("d"); write("d/r.txt", "x\ny"); ls("d") |> map(f => readLines(f))`, `[["x", "y"]]`},
		{"ls dir row as path", `mkdir("d/e"); write("d/e/s.txt", "1"); ls("d") |> map(f => ls(f) |> map(g => g.name))`, `[["d/e/s.txt"]]`},
		{"ls row mv", `write("t.txt", "1"); mv(ls(".")[0], "u.txt"); exists("u.txt")`, "true"},
		{"load", `write("lib.mcpshell", "export function triple(n) { return n * 3 }"); load("lib.mcpshell"); triple(7)`, "21"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := fileShell(t, false)
			v, err := sh.Eval(c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored:\n%v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestFileValue: ls() and glob() return File values, which act as their path
// string wherever a string is expected.
func TestFileValue(t *testing.T) {
	const setup = `mkdir("data/sub"); write("data/x.json", "{\"a\": 1}"); write("data/y.txt", "l1\nl2"); write(".bashrc", "x"); write("a.tar.gz", "z"); `
	cases := []struct{ name, src, want string }{
		{"fields", `let f = glob("data/*.json")[0]; [f.path, f.name, f.base, f.dir, f.ext, f.type, f.size]`,
			`["data/x.json", "data/x.json", "x.json", "data", ".json", "file", 8]`},
		{"ls rows", `ls("data")`, `[File("data/sub", dir), File("data/x.json"), File("data/y.txt")]`},
		{"ls dir fields", `ls("data") |> filter(f => f.type == "dir") |> map(f => [f.base, f.size])`, `[["sub", 0]]`},
		{"ext as Node", `glob("*", {hidden: true}) |> map(f => f.ext)`, `["", ".gz"]`},
		{"root dir", `glob("*.gz")[0].dir`, `.`},
		{"keys", `keys(glob("*.gz")[0])`, `["path", "name", "base", "dir", "ext", "type", "size"]`},
		{"instanceof", `[glob("*.gz")[0] instanceof File, ls()[0] instanceof File]`, `[true, true]`},
		{"template", "let f = glob(\"data/*.txt\")[0]; `<${f}>`", `<data/y.txt>`},
		{"concat", `glob("data/*.txt")[0] + "!"`, `data/y.txt!`},
		{"String", `String(glob("data/*.txt")[0])`, `data/y.txt`},
		{"loose equality", `let f = glob("data/*.txt")[0]; [f == "data/y.txt", "data/y.txt" == f, f != "data/y.txt", f == "other"]`, `[true, true, false, false]`},
		{"strict equality", `let f = glob("data/*.txt")[0]; [f === "data/y.txt", f !== "data/y.txt", f === f]`, `[false, true, true]`},
		{"toJson", `toJson(glob("data/*"))`, `["data/x.json","data/y.txt"]`},
		{"toJson nested", `JSON.stringify({f: glob("*.gz")[0]})`, `{"f":"a.tar.gz"}`},
		{"string methods", `let f = glob("data/*.json")[0]; [f.endsWith(".json"), f.startsWith("data/"), f.includes("x"), f.split("/"), f.replace(".json", ".csv"), f.length, f.upper()]`,
			`[true, true, true, ["data", "x.json"], "data/x.csv", 11, "DATA/X.JSON"]`},
		{"old string code", `glob("data/*") |> filter(f => f.endsWith(".txt")) |> map(f => f.split("/").pop())`, `["y.txt"]`},
		{"pipe into string command", `glob("data/*.txt")[0] |> upper()`, `DATA/Y.TXT`},
		{"array includes path", `[glob("data/*").includes("data/y.txt"), glob("data/*").indexOf("data/y.txt")]`, `[true, 1]`},
		{"set ops across globs", `glob("data/*") |> difference(glob("data/*.txt"))`, `[File("data/x.json")]`},
		{"unique", `[...glob("data/*"), ...glob("data/*")] |> unique() |> len()`, `2`},
		{"sort", `[...glob("data/*")].reverse() |> sort()`, `[File("data/x.json"), File("data/y.txt")]`},
		{"sort by field", `ls("data") |> sort("size") |> map(f => f.base)`, `["sub", "y.txt", "x.json"]`},
		{"join", `glob("data/*").join(",")`, `data/x.json,data/y.txt`},
		{"object key", `let o = {}; for (const f of glob("data/*")) o[f] = f.size; [o, o[glob("data/*.txt")[0]]]`, `[{data/x.json: 8, data/y.txt: 5}, 5]`},
		{"groupBy", `glob("**", {hidden: true}) |> groupBy(f => f.dir) |> keys()`, `[".", "data"]`},
		{"read-only", `let f = glob("*.gz")[0]; try { f.path = "x" } catch (e) { e }`, `File.path is read-only: a File keeps the path it was made with; make another with new File(path)`},
		{"extra field allowed", `let f = glob("*.gz")[0]; f.note = 1; f.note`, `1`},
		{"cycle", `let f = new File("a"); f.self = f; [f, toJson([f]), f == f]`, `[File("a"), "["a"]", true]`},
		{"structuredClone", `let f = glob("*.gz")[0]; structuredClone(f) instanceof File`, `true`},
		{"spread is plain", `({...glob("*.gz")[0]}).path`, `a.tar.gz`},
		{"new File", `let f = new File("data/y.txt"); [f.type, f.size, readLines(f)]`, `[null, null, ["l1", "l2"]]`},
		{"File without new", `File("data/y.txt")`, `File("data/y.txt")`},
		{"new File of File", `new File(glob("*.gz")[0]).path`, `a.tar.gz`},
		{"glob grep file", `glob("data/*", {grep: "l2"}) |> map(h => [h.file, h.file == "data/y.txt", h.num])`, `[[File("data/y.txt"), true, 2]]`},
		// every path argument of a file command takes a File
		{"read", `read(glob("data/*.txt")[0])`, "1: l1\n2: l2"},
		{"readLines", `ls("data") |> filter(f => f.type == "file") |> map(f => readLines(f).length)`, `[1, 2]`},
		{"exists", `exists(glob("*.gz")[0])`, `true`},
		{"ls of dir row", `ls("data")[0] |> ls()`, `[]`},
		{"tree", `tree(ls("data")[0])`, `data/sub/`},
		{"grep", `grep(glob("data/*.txt")[0], {match: "2", mode: "count"})`, `1`},
		{"write append edit", `let f = new File("w.txt"); write(f, "a"); append(f, "b"); edit(f, "ab", "cd"); readLines(f)`, `["cd"]`},
		{"mv mkdir deletePath", `mkdir(new File("m")); mv(glob("*.gz")[0], "m/b.gz"); deletePath(glob("m/*")[0]); [exists("a.tar.gz"), glob("m/*")]`, `[false, []]`},
		{"load", `write("lib.mcpshell", "export const v = 7"); load(glob("*.mcpshell")[0]); v`, `7`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sh := fileShell(t, false)
			v, err := sh.Eval(setup + c.src)
			if err != nil {
				t.Fatalf("eval(%q) errored:\n%v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestFileValueErrors: a File keeps the path confinement of the command that
// uses it, and an old-style {name, type} row is no longer a path.
func TestFileValueErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"confined", `read(new File("../../etc/passwd"))`, "Access denied"},
		{"plain row", `readLines({name: "a.txt", type: "file"})`, "Wrong arguments"},
		{"no path", `new File()`, "needs a path"},
		{"bad path", `new File(3)`, "Type mismatch"},
		{"delete field", `write("a.txt", "1"); let f = ls()[0]; delete f.name`, "read-only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := fileShell(t, false).Eval(c.src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("eval(%q) error = %v, want it to contain %q", c.src, err, c.want)
			}
		})
	}
}

// TestFileStringParity: a File converts to a string as a JS object whose
// toString and toJSON return its path does. want is what Node 24 printed for
// the same body with
// `class PathLike { constructor(p) { this.path = p } toString() { return this.path } toJSON() { return this.path } }`
// and mk = p => new PathLike(p).
func TestFileStringParity(t *testing.T) {
	const body = "const f = mk(\"data/x.json\"), g = mk(\"a.txt\")\nconst o = {}; o[g] = 2\n" +
		"JSON.stringify([`${f}`, f + \"\", String(f), f == \"data/x.json\", f === \"data/x.json\", f != \"data/x.json\", \"data/x.json\" == f, f !== \"data/x.json\", " +
		"JSON.stringify(f), JSON.stringify({f}), [f, g].join(\"|\"), {[f]: 1}, o, typeof f, f + 1, [f, g].sort(), [f < g, g < f], f.toString(), JSON.stringify([f], null, 1)])"
	const want = `["data/x.json","data/x.json","data/x.json",true,false,false,true,true,"\"data/x.json\"","{\"f\":\"data/x.json\"}","data/x.json|a.txt",{"data/x.json":1},{"a.txt":2},"object","data/x.json1",["a.txt","data/x.json"],[false,true],"data/x.json","[\n \"data/x.json\"\n]"]`
	v, err := fileShell(t, false).Eval("const mk = p => new File(p)\n" + body)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Display(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestFileConfinement(t *testing.T) {
	sh := fileShell(t, false)
	_, err := sh.Eval(`read("../../../etc/passwd")`)
	if err == nil || !strings.Contains(err.Error(), "Access denied") {
		t.Fatalf("expected access-denied error, got: %v", err)
	}
}

func TestFileReadOnly(t *testing.T) {
	sh := fileShell(t, true)
	if _, err := sh.Eval(`write("x.txt", "nope")`); err == nil {
		t.Fatalf("write should be unavailable in read-only mode")
	}
	// read commands still work
	if _, err := sh.Eval(`exists("anything")`); err != nil {
		t.Fatalf("exists should work in read-only mode: %v", err)
	}
}

func TestFileReadRoots(t *testing.T) {
	extra := t.TempDir()
	if err := os.WriteFile(filepath.Join(extra, "o.txt"), []byte("saved\noutput"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), t.TempDir(), false, extra)
	sh.SetState(map[string]runtime.Value{"p": runtime.Str(filepath.Join(extra, "o.txt"))})
	for _, c := range []struct{ src, want string }{
		{`read(p)`, "1: saved\n2: output"},
		{`readLines(p)`, `["saved", "output"]`},
		{`exists(p)`, "true"},
		{`grep(p, {match: "out"}) |> map(h => h.line)`, `["output"]`},
	} {
		v, err := sh.Eval(c.src)
		if err != nil || v.Display() != c.want {
			t.Errorf("%s = %v, %v; want %q", c.src, v, err, c.want)
		}
	}
	// Writes stay confined to root: an absolute path maps under root, so the
	// read root is never modified.
	if _, err := sh.Eval(`write(p, "x")`); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(extra, "o.txt")); string(b) != "saved\noutput" {
		t.Errorf("write reached the read root: %q", b)
	}
	// Absolute paths outside root and readRoots are not readable.
	if v, err := sh.Eval(`exists("/etc/hostname")`); err != nil || v.Display() != "false" {
		t.Errorf(`exists("/etc/hostname") = %v, %v; want false`, v, err)
	}
}

// BenchmarkGlobReadLines is a glob over many files, each read with
// readLines: the File values glob returns must not cost more than the
// strings it returned before.
func BenchmarkGlobReadLines(b *testing.B) {
	dir := b.TempDir()
	for i := range 2000 {
		sub := filepath.Join(dir, "d"+strconv.Itoa(i%20))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "f"+strconv.Itoa(i)+".txt"), []byte("a\nb\nc\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	sh := toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), dir, true)
	b.ResetTimer()
	for b.Loop() {
		if _, err := sh.Eval(`glob("**/*.txt") |> map(f => readLines(f).length) |> sum()`); err != nil {
			b.Fatal(err)
		}
	}
}
