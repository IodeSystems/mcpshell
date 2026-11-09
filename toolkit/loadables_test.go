package toolkit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func mcpLikeShell(web bool, filesDir string, overflow string) *runtime.Shell {
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	sh.AddLoadable(toolkit.GraphLoadable())
	sh.AddLoadable(toolkit.WebLoadable(web))
	var restore func(*runtime.Shell)
	var roots []string
	if overflow != "" {
		reader := func(sh *runtime.Shell) {
			toolkit.InstallFile(sh, overflow, true)
			sh.SetFileLoader(nil)
			toolkit.ClearFileConfiner(sh)
		}
		reader(sh)
		restore = reader
		roots = []string{overflow}
	}
	sh.AddLoadable(toolkit.FilesLoadable(filesDir, false, roots, restore))
	sh.AddLoadable(toolkit.BrowserLoadable(false))
	return sh
}

func TestGraphIsLoadable(t *testing.T) {
	sh := mcpLikeShell(false, "", "")
	// Global command names of a loadable give the hint, bare and in a pipe.
	for _, src := range []string{`addNode({})`, `root()`, `[1] |> nodes()`} {
		mustContain(t, src, failEval(t, sh, src), `call load("graph") first`)
	}
	mustContain(t, "prompt", sh.ToPrompt(false), `load("graph")`)
	if strings.Contains(sh.ToPrompt(false), "addNode(") {
		t.Error("graph commands are listed before load")
	}
	okEval(t, sh, `load("graph")`)
	okEval(t, sh, `root()`)
}

func TestOperatorTierDisabled(t *testing.T) {
	sh := mcpLikeShell(false, "", "")
	mustContain(t, "load web", failEval(t, sh, `load("web")`), "--web")
	mustContain(t, "Web call", failEval(t, sh, `Web.fetch("http://x")`), "--web", "did not enable")
	mustContain(t, "read", failEval(t, sh, `read("a")`), "--files-dir")
	mustContain(t, "load files", failEval(t, sh, `load("files")`), "--files-dir")
	mustContain(t, "load path", failEval(t, sh, `load("a.mcpshell")`), "--files-dir")
	mustContain(t, "load browser", failEval(t, sh, `load("browser")`), "--browser")
	p := sh.ToPrompt(false)
	for _, bad := range []string{`load("web")`, `load("files")`, `load("browser")`} {
		if strings.Contains(p, bad) {
			t.Errorf("prompt advertises %s", bad)
		}
	}
}

func TestWebLoadableWhenEnabled(t *testing.T) {
	sh := mcpLikeShell(true, "", "")
	mustContain(t, "prompt", sh.ToPrompt(false), `load("web")`)
	mustContain(t, "before", failEval(t, sh, `Html.text("<p>x</p>")`), `call load("web") first`)
	okEval(t, sh, `load("web")`)
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
	mustContain(t, "prompt", sh.ToPrompt(false), `load("files")`)
	mustContain(t, "before", failEval(t, sh, `read("a.txt")`), `call load("files") first`)
	mustContain(t, "write before", failEval(t, sh, `write("b.txt", "x")`), `call load("files") first`)

	// The path form of load installs the toolkit when it is permitted.
	okEval(t, sh, `load("lib.mcpshell")`)
	mustContain(t, "after path load", okEval(t, sh, `read("a.txt")`), "hi")
	if okEval(t, sh, `answer`) != "42" {
		t.Error("file did not run")
	}

	okEval(t, sh, `unload("files")`)
	mustContain(t, "after unload", failEval(t, sh, `read("a.txt")`), `call load("files") first`)
	// The path form installs the toolkit again; the file runs again and
	// replaces its export (it failed on the second declaration before).
	okEval(t, sh, `load("lib.mcpshell")`)
	mustContain(t, "after reload", okEval(t, sh, `[answer, read("a.txt")]`), "42", "hi")
}

// With an overflow dir the read-only reader is there from the start, whether
// or not --files-dir is set, and unload of files puts it back.
func TestOverflowReaderStaysAvailable(t *testing.T) {
	ovf, dir := t.TempDir(), t.TempDir()
	spill := filepath.Join(ovf, "eval-1.txt")
	if err := os.WriteFile(spill, []byte("l1\nl2\nl3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, filesDir := range []string{"", dir} {
		sh := mcpLikeShell(false, filesDir, ovf)
		mustContain(t, "read spill", okEval(t, sh, `read(`+quote(spill)+`, 2, 1)`), "l2")
		mustContain(t, "grep spill", okEval(t, sh, `grep(`+quote(spill)+`, {match: "l3"})`), "l3")
		mustContain(t, "no write", failEval(t, sh, `write(`+quote(spill)+`, "x")`), "write")
		// The reader is not the files toolkit: it cannot load a .mcpshell file.
		failEval(t, sh, `load("x.mcpshell")`)
		if filesDir == "" {
			continue
		}
		okEval(t, sh, `load("files")`)
		okEval(t, sh, `write("n.txt", "x")`)
		mustContain(t, "read spill", okEval(t, sh, `read(`+quote(spill)+`, 1, 1)`), "l1")
		okEval(t, sh, `unload("files")`)
		mustContain(t, "reader restored", okEval(t, sh, `read(`+quote(spill)+`, 3, 1)`), "l3")
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
		v, err := sh.Eval(`load("db"); Db.loadCsv("p", "p.csv"); Db.query("select sum(amt) s from p")[0].s`)
		if err != nil {
			t.Fatalf("overflow=%v: %v", overflow, err)
		}
		if got := v.Display(); got != "0.3" {
			t.Fatalf("overflow=%v: sum = %s, want 0.3", overflow, got)
		}
	}
	sh := mcpLikeShell(false, "", "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	if _, err := sh.Eval(`load("db"); Db.loadCsv("p", "p.csv")`); err == nil || !strings.Contains(err.Error(), "--files-dir") {
		t.Fatalf("without --files-dir: err = %v, want a hint naming --files-dir", err)
	}
}

// load and unload take several names, in order, as arguments or one array.
func TestLoadVariadic(t *testing.T) {
	sh := mcpLikeShell(false, t.TempDir(), "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	toolkit.AddSolver(sh)
	v, err := sh.Eval(`load("db", "files")`)
	if err != nil || !strings.Contains(v.Display(), `Loaded "db"`) || !strings.Contains(v.Display(), `Loaded "files"`) {
		t.Fatalf("load(db, files) = %v, %v", v, err)
	}
	if _, err := sh.Eval(`ls(".")`); err != nil {
		t.Fatalf("files must be loaded: %v", err)
	}
	if v, err := sh.Eval(`load(["solver"])`); err != nil || !strings.Contains(v.Display(), `Loaded "solver"`) {
		t.Fatalf("load([solver]) = %v, %v", v, err)
	}
	_, err = sh.Eval(`load("graph", "web", "nope")`)
	if err == nil || !strings.Contains(err.Error(), `"web" failed`) || !strings.Contains(err.Error(), "Done before it: graph") || !strings.Contains(err.Error(), "Not tried: nope") {
		t.Fatalf("a failure must name done and not-tried names: %v", err)
	}
	if v, err := sh.Eval(`unload("db", "solver")`); err != nil || !strings.Contains(v.Display(), `Unloaded "solver"`) {
		t.Fatalf("unload(db, solver) = %v, %v", v, err)
	}
	for _, bad := range []string{`load()`, `load("db", 5)`} {
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
	for _, code := range []string{`load("db"); Db.query("select * from nope")`, `load("db"); Db.load(5)`, `load("solver"); Solver.lp({maximize: {a: 1}, subjectTo: [{a: 1}]})`} {
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

// Models copy the Db.query pattern: Files.read, files.ls, Graph.addNode.
// The capitalized namespace is an alias; a wrong-case one errors with the
// right form.
func TestToolkitNamespaceGuesses(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := mcpLikeShell(false, ws, "")
	sh.AddLoadable(toolkit.DbLoadable(toolkit.DbOptions{}))
	if _, err := sh.Eval(`Files.read("a.txt")`); err == nil || !strings.Contains(err.Error(), `load("files")`) {
		t.Errorf("Files.read before load: want the load hint, got %v", err)
	}
	if v, err := sh.Eval(`load("files"); Files.read("a.txt")`); err != nil || !strings.Contains(v.Display(), "hi") {
		t.Fatalf("Files.read after load = %v, %v", v, err)
	}
	if v, err := sh.Eval(`len(Files.ls("."))`); err != nil || v.Display() != "1" {
		t.Errorf("Files.ls = %v, %v", v, err)
	}
	_, err := sh.Eval(`files.ls(".")`)
	if err == nil || !strings.Contains(err.Error(), "commands are bare: glob(), read()") || strings.Contains(err.Error(), "help()") {
		t.Errorf("files.ls: want the bare-commands hint, got %v", err)
	}
	_, err = sh.Eval(`db.query("select 1")`)
	if err == nil || !strings.Contains(err.Error(), "namespace is Db") || !strings.Contains(err.Error(), `load("db")`) {
		t.Errorf("db.query: want the Db namespace hint, got %v", err)
	}
	if v, err := sh.Eval(`load("graph"); Graph.addNode(root(), "x"); len(nodes("x"))`); err != nil || v.Display() != "1" {
		t.Errorf("Graph.addNode = %v, %v", v, err)
	}
	if v, err := sh.Eval(`help()`); err != nil || strings.Contains(v.Display(), "Files.read") {
		t.Errorf("aliases must be hidden from help()")
	}
	if _, err := sh.Eval(`let files = ["x"]; files.length`); err != nil {
		t.Errorf("a variable named files must still work: %v", err)
	}
	if _, err := sh.Eval(`unload("files"); Files.read("a.txt")`); err == nil {
		t.Errorf("Files.read after unload must fail")
	}
}
