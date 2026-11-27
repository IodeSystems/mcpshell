package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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

// runREPLWith runs `mcpshell repl args` over input and returns stdout and
// stderr. It restores the process files even if the REPL fails.
func runREPLWith(t *testing.T, input string, args ...string) (stdout, stderr string) {
	t.Helper()
	oldIn, oldErr := os.Stdin, os.Stderr
	defer func() { os.Stdin, os.Stderr = oldIn, oldErr }()
	inR, inW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Stdin, os.Stderr = inR, errW
	_, _ = inW.WriteString(input)
	_ = inW.Close()
	var eb bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&eb, errR); close(done) }()
	stdout = captureStdout(t, func() { runREPL(args) })
	_ = errW.Close()
	<-done
	return stdout, eb.String()
}

// Each line is one session eval: a top-level let lasts across lines.
func TestREPLSessionPersistsAcrossLines(t *testing.T) {
	out, _ := runREPLWith(t, "let x = 41\nx + 1\n")
	if out != "41\n42\n" {
		t.Fatalf("stdout = %q, want the let value then 42", out)
	}
}

// The repl session is the mcp session: a toolkit loads on first reference,
// and one that needs a flag fails naming it.
func TestREPLLoadsToolkitsOnFirstUse(t *testing.T) {
	out, errOut := runREPLWith(t, "typeof Graph.root()\nFiles.ls(\".\")\nWeb.fetchText(\"x\")\n")
	if strings.TrimSpace(out) == "" {
		t.Errorf("Graph did not load on first reference: stdout %q, stderr %q", out, errOut)
	}
	if !strings.Contains(errOut, "--files-dir") || !strings.Contains(errOut, "--web") {
		t.Errorf("Files and Web need their flags, stderr = %q", errOut)
	}
	out, errOut = runREPLWith(t, "Files.exists(\".\")\n", "--files-dir", t.TempDir())
	if strings.TrimSpace(out) != "true" {
		t.Errorf("--files-dir must permit Files: stdout %q, stderr %q", out, errOut)
	}
}

// --max-output caps a line's output as it does an MCP eval call.
func TestREPLMaxOutput(t *testing.T) {
	out, _ := runREPLWith(t, "\"x\".repeat(500)\n", "--max-output", "100")
	if !strings.Contains(out, "OUTPUT TRUNCATED at 100 of 500 bytes") {
		t.Errorf("stdout = %q", out)
	}
}

// An upstream from --connect is callable, and its process ends at EOF.
func TestREPLCallsUpstream(t *testing.T) {
	t.Setenv("GO_REPL_HELPER", "widgets")
	out, errOut := runREPLWith(t, "app.echo(\"hi\")\n", "--connect", "app="+os.Args[0]+" -test.run=TestREPLHelperProcess")
	if strings.TrimSpace(out) != "echo: hi" {
		t.Fatalf("stdout = %q, stderr = %q", out, errOut)
	}
}

// TestREPLHelperProcess is not a real test: with GO_REPL_HELPER set the test
// binary is a minimal MCP server exposing one tool, echo.
func TestREPLHelperProcess(t *testing.T) {
	name := os.Getenv("GO_REPL_HELPER")
	if name == "" {
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue
		}
		var res any
		switch req.Method {
		case "initialize":
			res = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": name, "version": "0.0.1"}}
		case "tools/list":
			res = map[string]any{"tools": []map[string]any{{"name": "echo", "description": "echoes",
				"inputSchema": json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`)}}}
		case "tools/call":
			msg, _ := req.Params.Arguments["message"].(string)
			res = map[string]any{"content": []map[string]any{{"type": "text", "text": "echo: " + msg}}}
		default:
			continue
		}
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
	}
	os.Exit(0)
}

// --commands is the gate: without it there is no declared command; with it
// the repl calls the program, and --prompt lists it.
func TestREPLCommands(t *testing.T) {
	prog, _ := json.Marshal(os.Args[0])
	decl := `{"commands": {"tb": {"program": ` + string(prog) + `, "methods": {"none": {"args": ["-test.run=^$"]}}}}}`
	out, errOut := runREPLWith(t, "let r = tb.none(); [r.exit, r.stdout.includes(\"PASS\")]\n", "--commands", decl)
	if strings.TrimSpace(out) != "[0, true]" {
		t.Errorf("stdout = %q, stderr = %q", out, errOut)
	}
	if !strings.Contains(errOut, "declared commands: tb.none") {
		t.Errorf("stderr = %q", errOut)
	}
	if out, errOut := runREPLWith(t, "tb.none()\n"); strings.TrimSpace(out) != "" || !strings.Contains(errOut, "'tb'") {
		t.Errorf("without --commands: stdout %q, stderr %q", out, errOut)
	}
	prompt := captureStdout(t, func() { setupSession("mcp", []string{"--prompt", "--commands", decl}) })
	if !strings.Contains(prompt, "tb — programs the operator declared") {
		t.Errorf("prompt lacks the commands:\n%s", prompt)
	}
}
