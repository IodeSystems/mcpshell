package mcp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
)

// TestCommandHelperProcess is not a real test: with MCPSHELL_CMD_HELPER set
// the test binary is the declared program. The words after the first "--"
// are the mode, then its arguments.
func TestCommandHelperProcess(t *testing.T) {
	if os.Getenv("MCPSHELL_CMD_HELPER") == "" {
		return
	}
	i := slices.Index(os.Args, "--")
	args := os.Args[i+1:]
	switch mode, rest := args[0], args[1:]; mode {
	case "echo": // argv as JSON on stdout, a line on stderr, exit from $EXIT
		b, _ := json.Marshal(rest)
		fmt.Println(string(b))
		fmt.Fprintln(os.Stderr, "to stderr")
		code, _ := strconv.Atoi(os.Getenv("EXIT"))
		os.Exit(code)
	case "env":
		env := os.Environ()
		slices.Sort(env)
		b, _ := json.Marshal(env)
		fmt.Println(string(b))
	case "pwd":
		wd, _ := os.Getwd()
		fmt.Print(wd)
	case "big": // n bytes
		n, _ := strconv.Atoi(rest[0])
		fmt.Print(strings.Repeat("x", n))
	case "sleep": // a child in the same process group writes its pid to $PIDFILE
		if pf := os.Getenv("PIDFILE"); pf != "" {
			child := exec.Command(os.Args[0], "-test.run=^TestCommandHelperProcess$", "--", "sleep")
			child.Env = []string{"MCPSHELL_CMD_HELPER=1"}
			child.Stdout = os.Stdout
			if err := child.Start(); err != nil {
				os.Exit(9)
			}
			_ = os.WriteFile(pf, []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
		time.Sleep(30 * time.Second)
	}
	os.Exit(0)
}

// helperDecl declares namespace t with the test binary as the program;
// methods is the "methods" object, extra more command keys (", …" or "").
func helperDecl(methods, extra string) string {
	prog, _ := json.Marshal(os.Args[0])
	return `{"commands": {"t": {"program": ` + string(prog) + `, "args": ["-test.run=^TestCommandHelperProcess$", "--"],
		"env": ["MCPSHELL_CMD_HELPER=1"` + extra + `], "methods": ` + methods + `}}}`
}

// commandServer installs the declaration in a core shell and returns its
// server.
func commandServer(t *testing.T, decl string, opts mcp.CommandOptions) *mcp.Server {
	t.Helper()
	cmds, err := mcp.ParseCommands([]byte(decl), opts)
	if err != nil {
		t.Fatal(err)
	}
	sh := coreShell()
	cs, err := mcp.InstallCommands(sh, cmds)
	if err != nil {
		t.Fatal(err)
	}
	return mcp.NewServer(sh, 16000).WithCommands(cs)
}

const echoMethods = `{
	"echo": {"description": "Echo argv.\nMore.", "args": ["echo", "fixed"], "params": {
		"file": {"type": "string", "positional": true, "required": true},
		"more": {"type": "array", "items": {"type": "string"}, "positional": true},
		"n": {"type": "integer", "flag": "-n"},
		"pat": {"type": "string", "flag": "--pat="},
		"x": {"type": "number", "flag": "-x"},
		"v": {"type": "boolean", "flag": "-v"},
		"g": {"type": "array", "items": {"type": "string"}, "flag": "-g"}}},
	"dd": {"args": ["echo"], "dashdash": true, "params": {"p": {"type": "string", "positional": true}}},
	"none": {"args": ["echo"]}}`

func TestCommandArgv(t *testing.T) {
	srv := commandServer(t, helperDecl(echoMethods, ""), mcp.CommandOptions{})
	for _, c := range []struct{ code, want string }{
		{`t.echo({file: "a b"})`, `["fixed","a b"]`},
		{`t.echo({file: "f", n: "5", pat: "-x y", x: 1.5, v: true, g: ["*.go", 3], more: ["m1", "m2"]})`,
			`["fixed","-n","5","--pat=-x y","-x","1.5","-v","-g","*.go","-g","3","f","m1","m2"]`},
		{`t.echo({file: "f", v: false, n: 5.0})`, `["fixed","-n","5","f"]`},
		{`t.echo({file: "$(rm -rf /); 'q' | x > y *"})`, `["fixed","$(rm -rf /); 'q' | x > y *"]`},
		{`t.echo("pos")`, `["fixed","pos"]`},
		{`t.echo({file: "f", more: "one"})`, `["fixed","f","one"]`},
		{`t.dd({p: "-rf"})`, `["--","-rf"]`},
		{`t.dd()`, `[]`},
		{`t.none()`, `[]`},
	} {
		got, isErr := srv.EvalText(`JSON.stringify(JSON.parse(` + c.code + `.stdout))`)
		if isErr || got != c.want {
			t.Errorf("%s = %s (err %v), want %s", c.code, got, isErr, c.want)
		}
	}
	for _, c := range []struct{ code, want string }{
		{`t.echo({file: "-rf"})`, `a positional value may not start with "-"`},
		{`t.echo({file: "f", more: ["ok", "--x"]})`, `argument 'more' is "--x"`},
		{`t.echo({file: "f", n: 1.5})`, `argument 'n' must be integer`},
		{`t.echo({file: "f", g: [{}]})`, `argument 'g'[0] must be a string`},
		{`t.echo({n: 1})`, `missing required argument: file`},
		{`t.echo({file: "f", pats: "x"})`, `unknown argument 'pats'`},
		{`t.echo({file: "a" + String.fromCharCode(0)})`, `NUL byte`},
		{`t.none(1)`, `takes no arguments`},
	} {
		got, isErr := srv.EvalText(c.code)
		if !isErr || !strings.Contains(got, c.want) {
			t.Errorf("%s = %q, want an error with %q", c.code, got, c.want)
		}
	}
}

// A nonzero exit is a value with the output; a program that cannot start
// is an error.
func TestCommandResult(t *testing.T) {
	srv := commandServer(t, helperDecl(echoMethods, `, "EXIT=3"`), mcp.CommandOptions{})
	got, isErr := srv.EvalText(`let r = t.echo("f"); [r.exit, r.stderr == "to stderr\n", r.stdout == "[\"fixed\",\"f\"]\n"]`)
	if isErr || got != `[3, true, true]` {
		t.Errorf("exit 3 = %s (err %v)", got, isErr)
	}
	missing := `{"commands": {"nope": {"program": "/does/not/exist", "methods": {"run": {}}}}}`
	defer mcp.SetServerLog(&lockedBuf{})()
	srv = commandServer(t, missing, mcp.CommandOptions{})
	if got, isErr := srv.EvalText(`try { nope.run() } catch (e) { "caught: " + e }`); isErr || !strings.HasPrefix(got, "caught: nope.run: cannot start /does/not/exist") {
		t.Errorf("not found = %q (err %v)", got, isErr)
	}
}

// Only PATH and the declared entries reach the program.
func TestCommandEnv(t *testing.T) {
	t.Setenv("MCPSHELL_PASSED", "yes")
	t.Setenv("MCPSHELL_SECRET", "no")
	srv := commandServer(t, helperDecl(`{"env": {"args": ["env"]}}`, `, "MCPSHELL_PASSED", "MCPSHELL_UNSET", "FIXED=a=b"`), mcp.CommandOptions{})
	got, isErr := srv.EvalText(`JSON.parse(t.env().stdout) |> map(e => e.split("=")[0]) |> join(",")`)
	if isErr || got != "FIXED,MCPSHELL_CMD_HELPER,MCPSHELL_PASSED,PATH" {
		t.Errorf("env names = %s (err %v)", got, isErr)
	}
	if got, _ := srv.EvalText(`JSON.parse(t.env().stdout) |> filter(e => e.startsWith("FIXED")) |> join(",")`); got != "FIXED=a=b" {
		t.Errorf("fixed value = %s", got)
	}
}

// cwd resolves against Dir and must be inside a root when roots are given.
func TestCommandCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	decl := strings.Replace(helperDecl(`{"pwd": {"args": ["pwd"]}}`, ""), `"env"`, `"cwd": "sub", "env"`, 1)
	srv := commandServer(t, decl, mcp.CommandOptions{Dir: dir, Roots: []string{dir}})
	if got, isErr := srv.EvalText(`t.pwd().stdout`); isErr || got != filepath.Join(dir, "sub") {
		t.Errorf("pwd = %q (err %v)", got, isErr)
	}
	srv = commandServer(t, helperDecl(`{"pwd": {"args": ["pwd"]}}`, ""), mcp.CommandOptions{Dir: dir})
	if got, _ := srv.EvalText(`t.pwd().stdout`); got != dir {
		t.Errorf("default cwd = %q, want %s", got, dir)
	}
	other := t.TempDir()
	if _, err := mcp.ParseCommands([]byte(decl), mcp.CommandOptions{Dir: dir, Roots: []string{other}}); err == nil || !strings.Contains(err.Error(), "outside the file roots") {
		t.Errorf("cwd outside the roots: %v", err)
	}
}

// Output over maxOutputBytes is cut, and a note says so.
func TestCommandOutputCap(t *testing.T) {
	decl := strings.Replace(helperDecl(`{"big": {"args": ["big"], "params": {"n": {"type": "string", "positional": true}}}}`, ""), `"env"`, `"maxOutputBytes": 10, "env"`, 1)
	_, srvSh := commandServerShell(t, decl)
	v, err := srvSh.Eval(`len(t.big("100").stdout)`)
	if err != nil || v.Display() != "10" {
		t.Errorf("capped length = %v, %v", v, err)
	}
	if notes := srvSh.Limits().Notes(); len(notes) != 1 || !strings.Contains(notes[0], "t.big: stdout cut at 10 of 100 bytes") {
		t.Errorf("notes = %q", notes)
	}
}

func commandServerShell(t *testing.T, decl string) (*mcp.Server, *runtime.Shell) {
	t.Helper()
	cmds, err := mcp.ParseCommands([]byte(decl), mcp.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sh := coreShell()
	cs, err := mcp.InstallCommands(sh, cmds)
	if err != nil {
		t.Fatal(err)
	}
	return mcp.NewServer(sh, 16000).WithCommands(cs), sh
}

// The eval's time limit caps a command without its own timeout.
func TestCommandEvalTimeLimit(t *testing.T) {
	srv := commandServer(t, helperDecl(`{"sleep": {"args": ["sleep"]}}`, ""), mcp.CommandOptions{})
	raw, _ := json.Marshal(map[string]any{"code": `t.sleep()`, "limits": map[string]any{"timeout": 300}})
	start := time.Now()
	got, isErr := evalRaw(t, srv, string(raw))
	if !isErr || !strings.Contains(got, "t.sleep: still running at the eval's time limit (300ms); the process was killed") {
		t.Errorf("eval = %q", got)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
}

// evalRaw runs one eval call with raw JSON arguments.
func evalRaw(t *testing.T, srv *mcp.Server, args string) (string, bool) {
	t.Helper()
	var out strings.Builder
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"eval","arguments":` + args + `}}` + "\n"
	if err := srv.RunStdio(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out.String()), &m); err != nil {
		t.Fatal(err)
	}
	return toolText(t, m), m["result"].(map[string]any)["isError"].(bool)
}

func TestCommandDeclErrors(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	m := func(params string) string {
		return `{"commands": {"c": {"program": "x", "methods": {"m": {"params": {` + params + `}}}}}}`
	}
	for _, c := range []struct{ decl, want string }{
		{`not json`, "invalid JSON"},
		{`{}`, `expected a "commands" object`},
		{`{"commands": {}}`, "no command declared"},
		{`{"commands": {"Git": {"program": "git", "methods": {"a": {}}}}}`, "lower-case letter"},
		{`{"commands": {"for": {"program": "git", "methods": {"a": {}}}}}`, "lower-case letter"},
		{`{"commands": {"g": {"methods": {"a": {}}}}}`, `"program" is required`},
		{`{"commands": {"g": {"program": "git"}}}`, `"methods" is required`},
		{`{"commands": {"g": {"program": "git", "methods": {}}}}`, "no method declared"},
		{`{"commands": {"g": {"program": "git", "progam": "x", "methods": {"a": {}}}}}`, `unknown field "progam"`},
		{`{"commands": {"g": {"program": "git", "env": ["A B"], "methods": {"a": {}}}}}`, `env "A B"`},
		{`{"commands": {"g": {"program": "git", "cwd": "/does/not/exist", "methods": {"a": {}}}}}`, "is not a directory"},
		{`{"commands": {"g": {"program": "git", "timeoutMs": -1, "methods": {"a": {}}}}}`, "negative"},
		{`{"commands": {"g": {"program": "git", "methods": {"help": {}}}}}`, "toolkit's own"},
		{`{"commands": {"g": {"program": "git", "methods": {"a-b": {}}}}}`, "identifier"},
		{m(`"p": {"type": "object", "flag": "-p"}`), `type "object"`},
		{m(`"p": {"type": "array", "flag": "-p"}`), `"items": {"type": "string"}`},
		{m(`"p": {"type": "array", "items": {"type": "integer"}, "flag": "-p"}`), `"items": {"type": "string"}`},
		{m(`"p": {"type": "string"}`), `needs "flag"`},
		{m(`"p": {"type": "string", "flag": "-p", "positional": true}`), "exclude each other"},
		{m(`"p": {"type": "boolean", "positional": true}`), "a boolean is a flag"},
		{m(`"p": {"type": "boolean", "flag": "--p="}`), `drop the "="`},
		{m(`"p": {"type": "string", "flag": "p"}`), "must start with -"},
		{m(`"p": {"type": "string", "flag": "-p x"}`), "has a space"},
		{m(`"p": {"type": "string", "positonal": true}`), `unknown field "positonal"`},
		{m(`"a": {"type": "string", "positional": true}, "b": {"type": "string", "positional": true}`), `after the optional positional "a"`},
	} {
		_, err := mcp.ParseCommands([]byte(c.decl), mcp.CommandOptions{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", c.decl, err, c.want)
		}
	}

	// Namespaces taken at install.
	sh := coreShell()
	parse := func(decl string) []*mcp.Command {
		cmds, err := mcp.ParseCommands([]byte(decl), mcp.CommandOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return cmds
	}
	if _, err := mcp.InstallCommands(sh, parse(`{"commands": {"filter": {"program": "x", "methods": {"a": {}}}}}`)); err == nil || !strings.Contains(err.Error(), "built-in command") {
		t.Errorf("builtin name: %v", err)
	}
	twice := append(parse(`{"commands": {"g": {"program": "x", "methods": {"a": {}}}}}`), parse(`{"commands": {"g": {"program": "y", "methods": {"b": {}}}}}`)...)
	if _, err := mcp.InstallCommands(sh, twice); err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Errorf("twice: %v", err)
	}
}

// Without a declaration there is no command namespace and no Commands
// line; with one, the description lists each signature and the first line
// of its description, the eval schema stays in Gemini's subset, and the
// shell hint names the commands.
func TestCommandAdvertising(t *testing.T) {
	plain := mcp.NewServer(coreShell(), 16000)
	if got, isErr := plain.EvalText(`t.echo("f")`); !isErr || !strings.Contains(got, "Unknown command 't'") {
		t.Errorf("undeclared = %q", got)
	}
	if got, _ := plain.EvalText(`exec("ls")`); strings.Contains(got, "declared") {
		t.Errorf("hint without commands = %q", got)
	}

	srv := commandServer(t, helperDecl(echoMethods, ""), mcp.CommandOptions{})
	var out strings.Builder
	if err := srv.RunStdio(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Result struct {
			Tools []struct {
				Description string
				InputSchema any
			}
		}
	}
	if err := json.Unmarshal([]byte(out.String()), &m); err != nil {
		t.Fatal(err)
	}
	desc := m.Result.Tools[0].Description
	for _, want := range []string{"\nCommands (programs the operator declared", "t.echo(file: string, more?: array, n?: integer, pat?: string, x?: number, v?: boolean, g?: array) — Echo argv.", " · t.dd(p?: string)", "Toolkits (Ns.help() describes one): t"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
	if strings.Contains(desc, "More.") {
		t.Errorf("description has more than the first line:\n%s", desc)
	}
	checkGeminiSubset(t, "eval", m.Result.Tools[0].InputSchema)

	if got, _ := srv.EvalText(`Shell.run("ls")`); !strings.Contains(got, "The operator declared these commands: t.echo, t.dd, t.none") {
		t.Errorf("hint = %q", got)
	}
	if got, isErr := srv.EvalText(`t.help()`); isErr || !strings.Contains(got, "no shell") || !strings.Contains(got, "t.none() — runs ") {
		t.Errorf("help = %q", got)
	}
}

// With host functions as well, the shell hint names both, and the eval
// description has both lines.
func TestCommandHintWithHost(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	srv := commandServer(t, helperDecl(`{"none": {"args": ["echo"]}}`, ""), mcp.CommandOptions{}).WithHostFunctions()
	c := startClient(t, srv)
	c.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"experimental": hostDecl(0)}}})
	c.must("initialize")
	desc := c.description()
	if !strings.Contains(desc, "\nCommands (") || !strings.Contains(desc, "\nHost functions (") {
		t.Errorf("description:\n%s", desc)
	}
	if got, isErr := c.eval(10, `Shell.run("ls")`); !isErr || !strings.Contains(got, "The operator declared these commands: t.none") || !strings.Contains(got, "The host provides Host.exec") {
		t.Errorf("hint = %q", got)
	}
	c.close()
}
