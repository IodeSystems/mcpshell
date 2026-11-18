package toolkit_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Graph and Files commands are namespace-only: their old bare names are free
// for top-level session globals, loaded or not, and a call of an old name
// names the namespaced form.
func TestToolkitNamesAreFreeForGlobals(t *testing.T) {
	ws := t.TempDir()
	sh := mcpLikeShell(false, ws, "")
	for _, pass := range []string{"not loaded", "loaded"} {
		for _, src := range []string{`let out = []; out.length`, `let read = 1; read`, `let node = {id: 1}; node.id`, `const edges = 2; edges`} {
			if _, err := sh.EvalExported(src, nil); err != nil {
				t.Errorf("%s: %s: %v", pass, src, err)
			}
		}
		okEval(t, sh, `forget("out", "read", "node", "edges"); load("graph", "files")`)
	}
}

func TestOldBareNameNamesNamespace(t *testing.T) {
	sh := mcpLikeShell(false, t.TempDir(), "")
	mustContain(t, "before load", failEval(t, sh, `read("a.txt")`), `'read' is not defined — use Files.read after load("files")`)
	okEval(t, sh, `load("graph", "files")`)
	mustContain(t, "read", failEval(t, sh, `read("a.txt")`), `'read' is not defined — use Files.read`)
	mustContain(t, "out", failEval(t, sh, `Graph.root() |> out("x")`), `'out' is not defined — use Graph.out`)
	mustContain(t, "addNode", failEval(t, sh, `addNode(Graph.root(), "p")`), `use Graph.addNode`)
	// Without --files-dir the hint names the flag.
	mustContain(t, "disabled", failEval(t, mcpLikeShell(false, "", ""), `ls()`), `use Files.ls`, "--files-dir")
}

func TestToolkitValueMethods(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("x\ny\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := mcpLikeShell(false, ws, "")
	okEval(t, sh, `load("graph", "files")`)
	cases := []struct{ src, want string }{
		{`let a = Graph.addNode(Graph.root(), "p", {n: "a"}); let b = Graph.addNode(Graph.root(), "p", {n: "b"}); Graph.link(a, b, "knows"); a.out("knows").map(n => n.n)`, `["b"]`},
		{`Graph.nodes("p")[1].inbound("knows").map(n => n.n)`, `["a"]`},
		{`Graph.root().outE("p").length`, `2`},
		{`Files.ls(".")[0].readLines()`, `["x", "y"]`},
		{`Files.glob("*.txt")[0].read()`, `1: x
2: y`},
		{`Files.glob("*.txt")[0].endsWith(".txt")`, `true`}, // string methods still pass through
		{`Graph.addNode(Graph.root(), "q", {out: 5}).out`, `5`},
	}
	for _, c := range cases {
		if got := okEval(t, sh, c.src); got != c.want {
			t.Errorf("%s = %s, want %s", c.src, got, c.want)
		}
	}
	// A plain string or object does not get Files or Graph methods.
	mustContain(t, "string", failEval(t, sh, `"a.txt".readLines()`), "readLines")
	mustContain(t, "object", failEval(t, sh, `({id: "n1"}).out()`), "out")
}
