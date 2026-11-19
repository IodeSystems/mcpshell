package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// captureStdout swaps os.Stdout for a buffer around fn and returns what was
// written. It restores stdout even if fn fails.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() { _, _ = io.Copy(&buf, r); close(done) }()
	fn()
	_ = w.Close()
	<-done
	os.Stdout = old
	return buf.String()
}

// TestFileFormRunsTheFileCode runs main's -f dispatch end to end: the file's
// code is read and evaluated, and its result is printed.
func TestFileFormRunsTheFileCode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prog.mcpshell")
	if err := os.WriteFile(p, []byte("1 + 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { runFile(p) })
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("-f form printed %q, want 2", out)
	}
}

// TestStdinFormEvaluatesPipedStdin runs main's piped-stdin dispatch end to
// end: the piped bytes are evaluated and the result is printed.
func TestStdinFormEvaluatesPipedStdin(t *testing.T) {
	out := captureStdout(t, func() { runStdin(strings.NewReader("let x = 41; x + 1")) })
	if strings.TrimSpace(out) != "42" {
		t.Fatalf("stdin form printed %q, want 42", out)
	}
}

func TestTakeEvalFlags(t *testing.T) {
	evalOpts = runtime.EvalOptions{}
	rest := takeEvalFlags([]string{"--profile", "--limits", "steps=5000,timeout=60s", "1 + 1"})
	if !slices.Equal(rest, []string{"1 + 1"}) || !evalOpts.Profile {
		t.Fatalf("rest = %v, opts = %+v", rest, evalOpts)
	}
	if *evalOpts.Limits.Steps != 5000 || *evalOpts.Limits.TimeoutMs != 60000 {
		t.Errorf("limits = %+v", evalOpts.Limits)
	}
	evalOpts = runtime.EvalOptions{}
	rest = takeEvalFlags([]string{"--limits=steps=7", "-f", "x.mcpshell"})
	if !slices.Equal(rest, []string{"-f", "x.mcpshell"}) || *evalOpts.Limits.Steps != 7 {
		t.Errorf("rest = %v, limits = %+v", rest, evalOpts.Limits)
	}
	evalOpts = runtime.EvalOptions{}
	if rest = takeEvalFlags([]string{"mcp", "--profile"}); len(rest) != 2 || evalOpts.Profile {
		t.Errorf("flags after the first argument stay: %v", rest)
	}
}

// The CLI installs graph and files with the same namespaced commands as
// mcp's loadables, so a script runs in both.
func TestCLIGraphIsNamespaced(t *testing.T) {
	sh := newShell()
	v, err := sh.Eval(`let a = Graph.addNode(Graph.root(), "p"); [Graph.root() |> Graph.out("p") |> len(), Graph.root().out("p").length, typeof Files.readLines]`)
	if err != nil || v.Display() != `[1, 1, "function"]` {
		t.Fatalf("Graph.out in the CLI = %v, %v", v, err)
	}
	if _, err := sh.EvalExported(`let out = []; let read = 1`, nil); err != nil {
		t.Errorf("bare toolkit names must be free: %v", err)
	}
}
