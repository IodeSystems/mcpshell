package toolkit_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// mcpLikeShell builds the shell cmd/mcpshell builds for mcp: the overflow
// reader is installed at start only when files is not permitted.
func mcpLikeShell(web bool, filesDir string, overflow string) *runtime.Shell {
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	sh.AddLoadable(toolkit.GraphLoadable())
	sh.AddLoadable(toolkit.WebLoadable(web))
	var roots []string
	if overflow != "" {
		if filesDir == "" {
			toolkit.InstallOverflowReader(sh, overflow, "")
			sh.SetFileLoader(nil)
			toolkit.ClearFileConfiner(sh)
		}
		roots = []string{overflow}
	}
	sh.AddLoadable(toolkit.FilesLoadable(filesDir, false, roots))
	sh.AddLoadable(toolkit.BrowserLoadable(false))
	return sh
}

// A reference to a toolkit's namespace loads it: called, in a pipe, typeof,
// Ns.help(). Its commands are not in the prompt.
func TestGraphLoadsOnFirstUse(t *testing.T) {
	sh := mcpLikeShell(false, "", "")
	mustContain(t, "prompt", sh.ToPrompt(false), `Graph — graph store`)
	if strings.Contains(sh.ToPrompt(false), "Graph.addNode(") {
		t.Error("graph commands are listed in the prompt")
	}
	mustContain(t, "help", okEval(t, sh, `Graph.help()`), `The "graph" toolkit (Graph)`, "Graph.addNode(", "TYPICAL: Build a graph", "Graph.unload()")
	for _, src := range []string{`Graph.root().type`, `[Graph.root()] |> Graph.out("x") |> len()`, `typeof Graph`} {
		okEval(t, mcpLikeShell(false, "", ""), src)
	}
	if got := okEval(t, mcpLikeShell(false, "", ""), `typeof Graph`); got != "object" {
		t.Errorf("typeof Graph = %s", got)
	}
	mustContain(t, "load", failEval(t, sh, `load("graph")`), `load("graph"): toolkits load on first use; Graph.help() describes it`)
	mustContain(t, "unload", failEval(t, sh, `unload("graph")`), `Graph.unload() unloads the toolkit`)
}

// Ns.unload() unloads the toolkit and closes it; a later reference loads it
// again, empty. Either namespace of a toolkit with two unloads it.
func TestNamespaceUnloadThenReload(t *testing.T) {
	sh := mcpLikeShell(true, "", "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	okEval(t, sh, `Db.load("t", [{a: 1}]); Graph.addNode(Graph.root(), "p")`)
	mustContain(t, "unload", okEval(t, sh, `Db.unload()`), `Unloaded "db"`)
	if got := okEval(t, sh, `Db.tables()`); got != `[]` {
		t.Errorf("reloaded Db kept its tables: %s", got)
	}
	okEval(t, sh, `Graph.unload()`)
	if got := okEval(t, sh, `Graph.nodes("p").length`); got != "0" {
		t.Errorf("reloaded Graph kept its nodes: %s", got)
	}
	okEval(t, sh, `Html.text("<p>x</p>")`)
	mustContain(t, "Html.unload", okEval(t, sh, `Html.unload()`), `Unloaded "web"`)
	if _, err := sh.Eval(`typeof Web.fetch`); err != nil {
		t.Errorf("Web after Html.unload: %v", err)
	}
	mustContain(t, "old unload", failEval(t, sh, `unload("db")`), "Db.unload()")
}

// A toolkit the operator did not permit is an error at the reference that
// names the flag, as load gave; typeof is "undefined". An install error is an
// error at the reference.
func TestOperatorTierDisabled(t *testing.T) {
	sh := mcpLikeShell(false, "", "")
	mustContain(t, "Web", failEval(t, sh, `Web.fetch("http://x")`), `toolkit "web" is not enabled: the operator did not pass --web`)
	mustContain(t, "Html", failEval(t, sh, `Html.text("<p>x</p>")`), "--web")
	mustContain(t, "read", failEval(t, sh, `Files.read("a")`), "--files-dir")
	mustContain(t, "load files", failEval(t, sh, `load("files")`), "Files.help()")
	mustContain(t, "load path", failEval(t, sh, `load("a.mcpshell")`), "--files-dir")
	mustContain(t, "Browser", failEval(t, sh, `Browser.open("x")`), "--browser")
	if got := okEval(t, sh, `typeof Web`); got != "undefined" {
		t.Errorf("typeof Web = %s", got)
	}
	p := sh.ToPrompt(false)
	for _, bad := range []string{"Web", "Files", "Browser"} {
		if strings.Contains(p, bad+" — ") {
			t.Errorf("prompt advertises %s", bad)
		}
	}
	denied := mcpLikeShell(false, "", "")
	denied.DenyLoadables("graph")
	mustContain(t, "denied", failEval(t, denied, `Graph.root()`), `toolkit "graph" is disabled by the operator`)
	broken := mcpLikeShell(false, "", "")
	broken.AddLoadable(runtime.Loadable{Name: "bad", Install: func(sh *runtime.Shell) (io.Closer, error) {
		return nil, errors.New("no backend")
	}})
	mustContain(t, "install error", failEval(t, broken, `Bad.x()`), `toolkit "bad" failed to load: no backend`)
}

func TestWebLoadableWhenEnabled(t *testing.T) {
	sh := mcpLikeShell(true, "", "")
	mustContain(t, "prompt", sh.ToPrompt(false), `Web, Html — `)
	if got := okEval(t, sh, `Html.text("<p>x</p>")`); !strings.Contains(got, "x") {
		t.Errorf("Html.text = %q", got)
	}
}

func TestFilesLoadable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.mcpshell"), []byte(`export let answer = 42`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := mcpLikeShell(false, dir, "")
	mustContain(t, "prompt", sh.ToPrompt(false), `Files — `)

	// The path form of load installs the toolkit when it is permitted.
	okEval(t, sh, `load("lib.mcpshell")`)
	mustContain(t, "after path load", okEval(t, sh, `Files.read("a.txt")`), "hi")
	if okEval(t, sh, `answer`) != "42" {
		t.Error("file did not run")
	}

	okEval(t, sh, `Files.unload()`)
	// The path form installs the toolkit again; the file runs again and
	// replaces its export (it failed on the second declaration before).
	okEval(t, sh, `load("lib.mcpshell")`)
	mustContain(t, "after reload", okEval(t, sh, `[answer, Files.read("a.txt")]`), "42", "hi")
	okEval(t, sh, `Files.unload()`)
	okEval(t, sh, `Files.write("b.txt", "x")`)
}

// With an overflow dir and no --files-dir, the read-only reader is Files.*
// from the start. With --files-dir, Files is the full toolkit, which reads
// the overflow dir too.
func TestOverflowReaderStaysAvailable(t *testing.T) {
	ovf, dir := t.TempDir(), t.TempDir()
	spill := filepath.Join(ovf, "eval-1.txt")
	if err := os.WriteFile(spill, []byte("l1\nl2\nl3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, filesDir := range []string{"", dir} {
		sh := mcpLikeShell(false, filesDir, ovf)
		mustContain(t, "read spill", okEval(t, sh, `Files.read(`+quote(spill)+`, 2, 1)`), "l2")
		mustContain(t, "grep spill", okEval(t, sh, `Files.grep(`+quote(spill)+`, {match: "l3"})`), "l3")
		if filesDir == "" {
			mustContain(t, "no write", failEval(t, sh, `Files.write(`+quote(spill)+`, "x")`), "write")
			// The reader is not the files toolkit: it cannot load a .mcpshell file.
			failEval(t, sh, `load("x.mcpshell")`)
			continue
		}
		okEval(t, sh, `Files.write("n.txt", "x")`)
		okEval(t, sh, `Files.unload()`)
		mustContain(t, "reloaded", okEval(t, sh, `Files.read(`+quote(spill)+`, 3, 1)`), "l3")
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }

func TestXorIsHiddenAlias(t *testing.T) {
	sh := toolkit.InstallCore(runtime.NewShell())
	if okEval(t, sh, `xor(5, 3)`) != "6" {
		t.Error("xor must keep working")
	}
	if strings.Contains(sh.ToPrompt(false), "xor(") {
		t.Error("xor is listed in the prompt")
	}
}

// Db.loadCsv installs a permitted file toolkit itself, and does not read
// through the overflow reader.
func TestDbLoadCsvLoadsFiles(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		ws := t.TempDir()
		ov := ""
		if overflow {
			ov = t.TempDir()
		}
		if err := os.WriteFile(filepath.Join(ws, "p.csv"), []byte("sku,amt\nA,0.1\nB,0.2\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sh := mcpLikeShell(false, ws, ov)
		sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
		v, err := sh.Eval(`Db.loadCsv("p", "p.csv"); Db.query("select sum(amt) s from p")[0].s`)
		if err != nil {
			t.Fatalf("overflow=%v: %v", overflow, err)
		}
		if got := v.Display(); got != "0.3" {
			t.Fatalf("overflow=%v: sum = %s, want 0.3", overflow, got)
		}
	}
	sh := mcpLikeShell(false, "", "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	if _, err := sh.Eval(`Db.loadCsv("p", "p.csv")`); err == nil || !strings.Contains(err.Error(), "--files-dir") {
		t.Fatalf("without --files-dir: err = %v, want a hint naming --files-dir", err)
	}
}

// load takes several paths, in order; a toolkit name among them is an error
// that names what ran.
func TestLoadVariadic(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{"a.mcpshell": `export let a = 1`, "b.mcpshell": `export let b = 2`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh := mcpLikeShell(false, dir, "")
	if got := okEval(t, sh, `load("a.mcpshell", "b.mcpshell"); a + b`); got != "3" {
		t.Fatalf("load(a, b) = %s", got)
	}
	_, err := sh.Eval(`load("a.mcpshell", "graph", "b.mcpshell")`)
	if err == nil || !strings.Contains(err.Error(), "toolkits load on first use") || !strings.Contains(err.Error(), "Done before it: a.mcpshell") || !strings.Contains(err.Error(), "Not tried: b.mcpshell") {
		t.Fatalf("a failure must name done and not-tried names: %v", err)
	}
	for _, bad := range []string{`load()`, `load("a.mcpshell", 5)`} {
		if _, err := sh.Eval(bad); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

// An error from a loaded toolkit's command ends with the right call and an
// example, once.
func TestLoadedCommandErrorHasUsage(t *testing.T) {
	sh := mcpLikeShell(false, "", "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	toolkit.AddSolver(sh)
	for _, code := range []string{`Db.query("select * from nope")`, `Db.load(5)`, `Solver.lp({maximize: {a: 1}, subjectTo: [{a: 1}]})`} {
		_, err := sh.Eval(code)
		if err == nil {
			t.Fatalf("%s: want an error", code)
		}
		msg := err.Error()
		if !strings.Contains(msg, "Usage: e.g. ") || strings.Count(msg, "Usage: e.g. ") != 1 {
			t.Errorf("%s: want one usage example line:\n%s", code, msg)
		}
	}
}

// Models copy the Db.query pattern: files.ls, db.query. A wrong-case
// namespace errors with the right form.
func TestToolkitNamespaceGuesses(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := mcpLikeShell(false, ws, "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	if v, err := sh.Eval(`Files.read("a.txt")`); err != nil || !strings.Contains(v.Display(), "hi") {
		t.Fatalf("Files.read = %v, %v", v, err)
	}
	_, err := sh.Eval(`files.ls(".")`)
	if err == nil || !strings.Contains(err.Error(), "namespace is Files: Files.<command>. Files.help() describes it") {
		t.Errorf("files.ls: want the Files namespace hint, got %v", err)
	}
	_, err = sh.Eval(`db.query("select 1")`)
	if err == nil || !strings.Contains(err.Error(), "namespace is Db") || strings.Contains(err.Error(), `load(`) {
		t.Errorf("db.query: want the Db namespace hint, got %v", err)
	}
	if _, err := sh.Eval(`let files = ["x"]; files.length`); err != nil {
		t.Errorf("a variable named files must still work: %v", err)
	}
}
