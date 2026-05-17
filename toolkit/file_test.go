package toolkit_test

import (
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
		{"glob", `write("h.txt", "1"); write("i.md", "2"); glob("*.txt")`, `["h.txt"]`},
		{"glob recursive", `mkdir("src"); write("src/m.go", "x"); glob("**/*.go")`, `["src/m.go"]`},
		{"glob grep", `write("j.txt", "ok\nTODO fix\nok"); glob("*.txt", {grep: "TODO"}) |> map(h => h.num)`, "[2]"},
		{"grep content", `write("k.txt", "a\nTODO\nb"); grep("k.txt", {match: "TODO"}) |> len()`, "1"},
		{"grep count", `write("l.txt", "x\nx\ny"); grep("l.txt", {match: "x", mode: "count"})`, "2"},
		{"grep files", `write("n.txt", "has match"); grep("n.txt", {match: "match", mode: "files"})`, "true"},
		{"deletePath", `write("o.txt", "1"); deletePath("o.txt"); exists("o.txt")`, "false"},
		{"mv", `write("p.txt", "moved"); mv("p.txt", "q.txt"); read("q.txt")`, "1: moved"},
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
