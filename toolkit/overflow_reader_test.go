package toolkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Without --files-dir, Files.read() is the overflow reader. A file of the agent's
// own is not there; the error says why and what to do, instead of only "file
// not found" with a path under the overflow directory.
func TestOverflowReaderNotFoundSaysWhy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r1.txt"), []byte("saved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := toolkit.InstallOverflowReader(toolkit.InstallCore(runtime.NewShell()), dir, `run load("files") first`)
	mustContain(t, "saved result", okEval(t, sh, `Files.readLines("r1.txt")`), "saved")
	for _, src := range []string{`Files.read("data.csv")`, `Files.readLines("data.csv")`} {
		mustContain(t, src, failEval(t, sh, src), "file not found 'data.csv'", `run load("files") first`)
	}
}

// load(name) of a bare name that is no toolkit says so and lists the
// toolkits; a toolkit the host installed at start (the CLI's files, graph,
// web) answers that it is there, instead of "file not found './files'".
func TestLoadBareNames(t *testing.T) {
	sh := toolkit.AddSolver(toolkit.InstallGraph(toolkit.InstallFile(toolkit.InstallCore(runtime.NewShell()), t.TempDir(), false)))
	mustContain(t, "installed", failEval(t, sh, `load("files")`), "toolkits load on first use; Files.help() describes it")
	mustContain(t, "graph installed", failEval(t, sh, `load("graph")`), "Graph.help()")
	mustContain(t, "unknown", failEval(t, sh, `load("nope")`), "no toolkit named 'nope'", "Toolkits (Ns.help() describes one): Solver", `load("lib/helpers.mcpshell")`)
	mustContain(t, "path", failEval(t, sh, `load("lib/x.mcpshell")`), "file not found")
}
