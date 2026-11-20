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
		{"write then read", `Files.write("a.txt", "hello world"); Files.read("a.txt")`, "1: hello world"},
		{"read numbering", `Files.write("d.txt", "a\nb\nc"); Files.read("d.txt")`, "1: a\n2: b\n3: c"},
		{"readLines", `Files.write("b.txt", "x\ny\nz"); Files.readLines("b.txt")`, `["x", "y", "z"]`},
		{"exists true", `Files.write("c.txt", "1"); Files.exists("c.txt")`, "true"},
		{"exists false", `Files.exists("nope.txt")`, "false"},
		{"append", `Files.write("e.txt", "a"); Files.append("e.txt", "b"); Files.read("e.txt")`, "1: ab"},
		{"edit", `Files.write("f.txt", "const x = 1"); Files.edit("f.txt", "1", "42"); Files.read("f.txt")`, "1: const x = 42"},
		{"edit all", `Files.write("g.txt", "a a a"); Files.edit("g.txt", "a", "b", {all: true}); Files.read("g.txt")`, "1: b b b"},
		{"mkdir nested write", `Files.mkdir("sub/deep"); Files.write("sub/deep/h.txt", "deep"); Files.read("sub/deep/h.txt")`, "1: deep"},
		{"ls", `Files.write("z.txt", "1"); Files.ls(".") |> map(f => f.name)`, `["z.txt"]`},
		{"ls type", `Files.write("z.txt", "1"); Files.ls(".") |> map(f => f.type)`, `["file"]`},
		{"glob", `Files.write("h.txt", "1"); Files.write("i.md", "2"); Files.glob("*.txt")`, `[File("h.txt")]`},
		{"glob recursive", `Files.mkdir("src"); Files.write("src/m.go", "x"); Files.glob("**/*.go")`, `[File("src/m.go")]`},
		{"glob grep", `Files.write("j.txt", "ok\nTODO fix\nok"); Files.glob("*.txt", {grep: "TODO"}) |> map(h => h.num)`, "[2]"},
		{"grep content", `Files.write("k.txt", "a\nTODO\nb"); Files.grep("k.txt", {match: "TODO"}) |> len()`, "1"},
		{"grep count", `Files.write("l.txt", "x\nx\ny"); Files.grep("l.txt", {match: "x", mode: "count"})`, "2"},
		{"grep files", `Files.write("n.txt", "has match"); Files.grep("n.txt", {match: "match", mode: "files"})`, "true"},
		{"deletePath", `Files.write("o.txt", "1"); Files.deletePath("o.txt"); Files.exists("o.txt")`, "false"},
		{"mv", `Files.write("p.txt", "moved"); Files.mv("p.txt", "q.txt"); Files.read("q.txt")`, "1: moved"},
		{"ls row as path", `Files.mkdir("d"); Files.write("d/r.txt", "x\ny"); Files.ls("d") |> map(f => Files.readLines(f))`, `[["x", "y"]]`},
		{"ls dir row as path", `Files.mkdir("d/e"); Files.write("d/e/s.txt", "1"); Files.ls("d") |> map(f => Files.ls(f) |> map(g => g.name))`, `[["d/e/s.txt"]]`},
		{"ls row mv", `Files.write("t.txt", "1"); Files.mv(Files.ls(".")[0], "u.txt"); Files.exists("u.txt")`, "true"},
		{"load", `Files.write("lib.mcpshell", "export function triple(n) { return n * 3 }"); load("lib.mcpshell"); triple(7)`, "21"},
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
	const setup = `Files.mkdir("data/sub"); Files.write("data/x.json", "{\"a\": 1}"); Files.write("data/y.txt", "l1\nl2"); Files.write(".bashrc", "x"); Files.write("a.tar.gz", "z"); `
	cases := []struct{ name, src, want string }{
		{"fields", `let f = Files.glob("data/*.json")[0]; [f.path, f.name, f.base, f.dir, f.ext, f.type, f.size]`,
			`["data/x.json", "data/x.json", "x.json", "data", ".json", "file", 8]`},
		{"ls rows", `Files.ls("data")`, `[File("data/sub", dir), File("data/x.json"), File("data/y.txt")]`},
		{"ls dir fields", `Files.ls("data") |> filter(f => f.type == "dir") |> map(f => [f.base, f.size])`, `[["sub", 0]]`},
		{"ext as Node", `Files.glob("*", {hidden: true}) |> map(f => f.ext)`, `["", ".gz"]`},
		{"root dir", `Files.glob("*.gz")[0].dir`, `.`},
		{"keys", `keys(Files.glob("*.gz")[0])`, `["path", "name", "base", "dir", "ext", "type", "size"]`},
		{"instanceof", `[Files.glob("*.gz")[0] instanceof File, Files.ls()[0] instanceof File]`, `[true, true]`},
		{"template", "let f = Files.glob(\"data/*.txt\")[0]; `<${f}>`", `<data/y.txt>`},
		{"concat", `Files.glob("data/*.txt")[0] + "!"`, `data/y.txt!`},
		{"String", `String(Files.glob("data/*.txt")[0])`, `data/y.txt`},
		{"loose equality", `let f = Files.glob("data/*.txt")[0]; [f == "data/y.txt", "data/y.txt" == f, f != "data/y.txt", f == "other"]`, `[true, true, false, false]`},
		{"strict equality", `let f = Files.glob("data/*.txt")[0]; [f === "data/y.txt", f !== "data/y.txt", f === f]`, `[false, true, true]`},
		{"toJson", `toJson(Files.glob("data/*"))`, `["data/x.json","data/y.txt"]`},
		{"toJson nested", `JSON.stringify({f: Files.glob("*.gz")[0]})`, `{"f":"a.tar.gz"}`},
		{"string methods", `let f = Files.glob("data/*.json")[0]; [f.endsWith(".json"), f.startsWith("data/"), f.includes("x"), f.split("/"), f.replace(".json", ".csv"), f.length, f.upper()]`,
			`[true, true, true, ["data", "x.json"], "data/x.csv", 11, "DATA/X.JSON"]`},
		{"old string code", `Files.glob("data/*") |> filter(f => f.endsWith(".txt")) |> map(f => f.split("/").pop())`, `["y.txt"]`},
		{"pipe into string command", `Files.glob("data/*.txt")[0] |> upper()`, `DATA/Y.TXT`},
		{"array includes path", `[Files.glob("data/*").includes("data/y.txt"), Files.glob("data/*").indexOf("data/y.txt")]`, `[true, 1]`},
		{"set ops across globs", `Files.glob("data/*") |> difference(Files.glob("data/*.txt"))`, `[File("data/x.json")]`},
		{"unique", `[...Files.glob("data/*"), ...Files.glob("data/*")] |> unique() |> len()`, `2`},
		{"sort", `[...Files.glob("data/*")].reverse() |> sort()`, `[File("data/x.json"), File("data/y.txt")]`},
		{"sort by field", `Files.ls("data") |> sort("size") |> map(f => f.base)`, `["sub", "y.txt", "x.json"]`},
		{"join", `Files.glob("data/*").join(",")`, `data/x.json,data/y.txt`},
		{"object key", `let o = {}; for (const f of Files.glob("data/*")) o[f] = f.size; [o, o[Files.glob("data/*.txt")[0]]]`, `[{data/x.json: 8, data/y.txt: 5}, 5]`},
		{"groupBy", `Files.glob("**", {hidden: true}) |> groupBy(f => f.dir) |> keys()`, `[".", "data"]`},
		{"read-only", `let f = Files.glob("*.gz")[0]; try { f.path = "x" } catch (e) { e }`, `File.path is read-only: a File keeps the path it was made with; make another with new File(path)`},
		{"extra field allowed", `let f = Files.glob("*.gz")[0]; f.note = 1; f.note`, `1`},
		{"cycle", `let f = new File("a"); f.self = f; [f, toJson([f]), f == f]`, `[File("a"), "["a"]", true]`},
		{"structuredClone", `let f = Files.glob("*.gz")[0]; structuredClone(f) instanceof File`, `true`},
		{"spread is plain", `({...Files.glob("*.gz")[0]}).path`, `a.tar.gz`},
		{"new File", `let f = new File("data/y.txt"); [f.type, f.size, Files.readLines(f)]`, `[null, null, ["l1", "l2"]]`},
		{"File without new", `File("data/y.txt")`, `File("data/y.txt")`},
		{"new File of File", `new File(Files.glob("*.gz")[0]).path`, `a.tar.gz`},
		{"glob grep file", `Files.glob("data/*", {grep: "l2"}) |> map(h => [h.file, h.file == "data/y.txt", h.num])`, `[[File("data/y.txt"), true, 2]]`},
		// every path argument of a file command takes a File
		{"read", `Files.read(Files.glob("data/*.txt")[0])`, "1: l1\n2: l2"},
		{"readLines", `Files.ls("data") |> filter(f => f.type == "file") |> map(f => Files.readLines(f).length)`, `[1, 2]`},
		{"exists", `Files.exists(Files.glob("*.gz")[0])`, `true`},
		{"ls of dir row", `Files.ls("data")[0] |> Files.ls()`, `[]`},
		{"tree", `Files.tree(Files.ls("data")[0])`, `data/sub/`},
		{"grep", `Files.grep(Files.glob("data/*.txt")[0], {match: "2", mode: "count"})`, `1`},
		{"write append edit", `let f = new File("w.txt"); Files.write(f, "a"); Files.append(f, "b"); Files.edit(f, "ab", "cd"); Files.readLines(f)`, `["cd"]`},
		{"mv mkdir deletePath", `Files.mkdir(new File("m")); Files.mv(Files.glob("*.gz")[0], "m/b.gz"); Files.deletePath(Files.glob("m/*")[0]); [Files.exists("a.tar.gz"), Files.glob("m/*")]`, `[false, []]`},
		{"load", `Files.write("lib.mcpshell", "export const v = 7"); load(Files.glob("*.mcpshell")[0]); v`, `7`},
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
		{"confined", `Files.read(new File("../../etc/passwd"))`, "Access denied"},
		{"plain row", `Files.readLines({name: "a.txt", type: "file"})`, "Wrong arguments"},
		{"no path", `new File()`, "needs a path"},
		{"bad path", `new File(3)`, "Type mismatch"},
		{"delete field", `Files.write("a.txt", "1"); let f = Files.ls()[0]; delete f.name`, "read-only"},
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
	_, err := sh.Eval(`Files.read("../../../etc/passwd")`)
	if err == nil || !strings.Contains(err.Error(), "Access denied") {
		t.Fatalf("expected access-denied error, got: %v", err)
	}
}

func TestFileReadOnly(t *testing.T) {
	sh := fileShell(t, true)
	if _, err := sh.Eval(`Files.write("x.txt", "nope")`); err == nil {
		t.Fatalf("write should be unavailable in read-only mode")
	}
	// read commands still work
	if _, err := sh.Eval(`Files.exists("anything")`); err != nil {
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
		{`Files.read(p)`, "1: saved\n2: output"},
		{`Files.readLines(p)`, `["saved", "output"]`},
		{`Files.exists(p)`, "true"},
		{`Files.grep(p, {match: "out"}) |> map(h => h.line)`, `["output"]`},
	} {
		v, err := sh.Eval(c.src)
		if err != nil || v.Display() != c.want {
			t.Errorf("%s = %v, %v; want %q", c.src, v, err, c.want)
		}
	}
	// A read root takes no writes: the error names it as read-only.
	if _, err := sh.Eval(`Files.write(p, "x")`); err == nil || !strings.Contains(err.Error(), "is in a read-only root") ||
		!strings.Contains(err.Error(), "Read-only root: "+extra+" (read-only)") {
		t.Errorf("write to read root: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(extra, "o.txt")); string(b) != "saved\noutput" {
		t.Errorf("write reached the read root: %q", b)
	}
	// Absolute paths outside root and readRoots are denied, not taken under root.
	if _, err := sh.Eval(`Files.exists("/etc/hostname")`); err == nil || !strings.Contains(err.Error(), "Access denied") {
		t.Errorf(`Files.exists("/etc/hostname") = %v; want Access denied`, err)
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
		if _, err := sh.Eval(`Files.glob("**/*.txt") |> map(f => Files.readLines(f).length) |> sum()`); err != nil {
			b.Fatal(err)
		}
	}
}

// A leading ~ is the home directory, not a directory named "~" under the root.
func TestFileHomeTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "work", "a.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Home as the root: ~/ paths resolve inside it, for reads and writes.
	sh := toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), home, false)
	for _, c := range []struct{ src, want string }{
		{`Files.readLines("~/work/a.txt")`, `["hi"]`},
		{`Files.ls("~/work") |> map(f => f.base) |> toJson()`, `["a.txt"]`},
		{`Files.exists("~")`, "true"},
		{`Files.write("~/work/b.txt", "x"); Files.readLines("work/b.txt")`, `["x"]`},
	} {
		v, err := sh.Eval(c.src)
		if err != nil || v.Display() != c.want {
			t.Errorf("%s = %v, %v; want %q", c.src, v, err, c.want)
		}
	}
	// A root that does not hold home: ~ is still home, so it is denied, not joined.
	sh = toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), t.TempDir(), false)
	if _, err := sh.Eval(`Files.ls("~/work")`); err == nil || !strings.Contains(err.Error(), "Access denied") ||
		!strings.Contains(err.Error(), filepath.Join(home, "work")) {
		t.Errorf("~ outside root: %v", err)
	}
	// Home as an extra read root: ~ reads work.
	sh = toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), t.TempDir(), false, home)
	if v, err := sh.Eval(`Files.readLines("~/work/a.txt")`); err != nil || v.Display() != `["hi"]` {
		t.Errorf("~ in read root: %v, %v", v, err)
	}
	if _, err := sh.Eval(`Files.ls("~bob/x")`); err == nil || !strings.Contains(err.Error(), "~user paths are not supported") {
		t.Errorf("~user: %v", err)
	}
	// A name that only starts with ~ inside the path is a plain name.
	if v, err := sh.Eval(`Files.exists("a~b")`); err != nil || v.Display() != "false" {
		t.Errorf("a~b: %v, %v", v, err)
	}
}

func TestShellCommandHint(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	_, err := sh.Eval(`Shell.run("cd ~/x && git status")`)
	if err == nil || !strings.Contains(err.Error(), "use the host's exec tool") {
		t.Errorf("Shell hint: %v", err)
	}
}
