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
	mustContain(t, "loader gone", failEval(t, sh, `load("lib.mcpshell")`), "")
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
