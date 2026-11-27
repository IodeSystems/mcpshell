package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// echoArgs returns its arguments.
func echoArgs(_ context.Context, args map[string]any) (any, error) { return args, nil }

// funcToolkit is namespace App: echo (open schema), strict (closed), none
// (no schema), fail, crash, thrown, and wait (returns when ctx ends).
func funcToolkit() mcp.Toolkit {
	return mcp.Toolkit{Namespace: "App", Summary: "test functions", Guide: "App guide text.", Functions: []mcp.Function{
		{Name: "echo", Description: "Echo the arguments.\nMore.", Fn: echoArgs, InputSchema: json.RawMessage(
			`{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"n":{"type":"integer"},"tags":{"type":"array"}}}`)},
		{Name: "strict", Fn: echoArgs, InputSchema: json.RawMessage(
			`{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}}}`)},
		{Name: "none", Fn: echoArgs},
		{Name: "fail", Fn: func(context.Context, map[string]any) (any, error) { return nil, errors.New("no such row") }},
		{Name: "crash", Fn: func(context.Context, map[string]any) (any, error) { panic("boom") }},
		{Name: "thrown", Fn: func(context.Context, map[string]any) (any, error) {
			return nil, &runtime.ShellError{Message: "custom", Value: runtime.Str("custom value")}
		}},
		{Name: "wait", Fn: func(ctx context.Context, _ map[string]any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return "not cancelled", nil
			}
		}},
		{Name: "rows", Fn: func(context.Context, map[string]any) (any, error) {
			type row struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}
			return []row{{1, "a"}, {2, "b"}}, nil
		}},
	}}
}

func funcShell(t *testing.T) *runtime.Shell {
	t.Helper()
	sh := coreShell()
	if err := mcp.InstallToolkit(sh, funcToolkit()); err != nil {
		t.Fatal(err)
	}
	return sh
}

func evalText(sh *runtime.Shell, code string) (string, bool) {
	v, err := sh.Eval(code)
	if err != nil {
		return err.Error(), true
	}
	return v.Display(), false
}

func TestFunctionArgs(t *testing.T) {
	sh := funcShell(t)
	for _, c := range []struct{ code, want string }{
		{`App.echo({name: "x"})`, `{name: "x"}`},
		{`App.echo("x", 5)`, `{n: 5, name: "x"}`},                                    // positional → schema order; result keys sorted (encoding/json)
		{`App.echo({name: 7, n: "5", tags: "a"})`, `{n: 5, name: "7", tags: ["a"]}`}, // lossless conversion
		{`App.echo({name: "x", n: 12345678901234567890123})`, `{n: 12345678901234567890123, name: "x"}`},
		{`App.echo({name: "x", extra: 1})`, `{extra: 1, name: "x"}`}, // open schema: undeclared passes
		{`App.none({a: 1})`, `{a: 1}`},
		{`App.none()`, `{}`},
		{`App.rows() |> map(r => r.name)`, `["a", "b"]`},
	} {
		if got, isErr := evalText(sh, c.code); isErr || got != c.want {
			t.Errorf("%s = %s (err %v), want %s", c.code, got, isErr, c.want)
		}
	}
	for _, c := range []struct{ code, want string }{
		{`App.echo({n: 1})`, "App.echo: missing required argument: name"},
		{`App.echo({name: "x", n: 1.5})`, "argument 'n' must be integer"},
		{`App.echo({name: "x", n: 1.5})`, "App.echo parameters:"},
		{`App.strict({nmae: "x"})`, "App.strict: unknown argument 'nmae'"},
		{`App.strict({other: "x"})`, "unknown argument 'other'"},
		{`App.none(5)`, "App.none takes one object argument"},
	} {
		if got, isErr := evalText(sh, c.code); !isErr || !strings.Contains(got, c.want) {
			t.Errorf("%s = %q, want an error with %q", c.code, got, c.want)
		}
	}
}

// An error from a handler is a script error that try/catch catches.
func TestFunctionErrors(t *testing.T) {
	for _, c := range []struct{ code, want string }{
		{`let r = 0; try { App.fail() } catch (e) { r = e.split("\n")[0] }; r`, "App.fail failed: no such row"},
		{`let r = 0; try { App.crash() } catch (e) { r = e.split("\n")[0] }; r`, "App.crash failed: panic: boom"},
		{`let r = 0; try { App.thrown() } catch (e) { r = e }; r`, "custom value"},
		{`Promise.allSettled([() => App.fail()]) |> map(s => s.status)`, `["rejected"]`},
	} {
		if got, isErr := evalText(funcShell(t), c.code); isErr || got != c.want {
			t.Errorf("%s = %q (err %v), want %q", c.code, got, isErr, c.want)
		}
	}
	if got, isErr := evalText(funcShell(t), `App.fail()`); !isErr || strings.Contains(got, "internal error") {
		t.Errorf("uncaught = %q (err %v)", got, isErr)
	}
}

// The handler's context ends at the eval's time limit and on Cancel.
func TestFunctionContext(t *testing.T) {
	sh := funcShell(t)
	if err := sh.Limits().Configure("", "timeout=200ms"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got, isErr := evalText(sh, `App.wait()`)
	if !isErr || !strings.Contains(got, "App.wait: still running at the eval's time limit (200ms); its context was cancelled") {
		t.Errorf("timeout = %q (err %v)", got, isErr)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("timeout took %v", d)
	}

	sh = funcShell(t)
	time.AfterFunc(100*time.Millisecond, sh.Limits().Cancel)
	got, isErr = evalText(sh, `App.wait()`)
	if !isErr || !strings.Contains(got, "App.wait: Execution cancelled") {
		t.Errorf("cancel = %q (err %v)", got, isErr)
	}
}

// The toolkit loads on first reference and is listed like a built-in one;
// AddToolkit also puts the signatures in the eval description.
func TestFunctionAdvertising(t *testing.T) {
	sh := funcShell(t)
	if line := sh.Commands().ToolkitsLine(); !strings.Contains(line, "App") {
		t.Errorf("Toolkits line = %q", line)
	}
	if got, _ := evalText(sh, `help()`); !strings.Contains(got, "App — test functions") {
		t.Errorf("help() lacks the toolkit:\n%s", got)
	}
	got, _ := evalText(sh, `App.help()`)
	for _, want := range []string{"App.echo(name: string, n?: integer, tags?: array) — Echo the arguments.", "App.none(args?: object)", "App guide text."} {
		if !strings.Contains(got, want) {
			t.Errorf("App.help() lacks %q:\n%s", want, got)
		}
	}
	if got, _ := evalText(sh, `help("App.echo")`); !strings.Contains(got, "n?: integer") {
		t.Errorf("help(App.echo) = %s", got)
	}

	srv := mcp.NewServer(toolkit.InstallCore(runtime.NewShell()), 16000)
	if err := srv.AddToolkit(funcToolkit()); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := srv.RunStdio(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Result struct {
			Tools []struct{ Description string }
		}
	}
	if err := json.Unmarshal([]byte(out.String()), &m); err != nil {
		t.Fatal(err)
	}
	desc := m.Result.Tools[0].Description
	for _, want := range []string{"\nApp (one object argument; App.help() describes them): App.echo(name: string, n?: integer, tags?: array) — Echo the arguments.", " · App.none(args?: object)", "Toolkits (Ns.help() describes one): App"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
	if err := srv.AddToolkit(mcp.Toolkit{Namespace: "Late", Functions: []mcp.Function{{Name: "f", Fn: echoArgs}}}); err == nil {
		t.Error("AddToolkit after tools/list succeeded")
	}
	if got, isErr := srv.EvalText(`App.echo({name: "y"}).name`); isErr || got != "y" {
		t.Errorf("served call = %q (err %v)", got, isErr)
	}
}

func TestInstallToolkitErrors(t *testing.T) {
	fn := []mcp.Function{{Name: "f", Fn: echoArgs}}
	for _, c := range []struct {
		tk   mcp.Toolkit
		want string
	}{
		{mcp.Toolkit{Namespace: "a.b", Functions: fn}, "must be an identifier"},
		{mcp.Toolkit{Namespace: "for", Functions: fn}, "must be an identifier"},
		{mcp.Toolkit{Namespace: "App"}, "no functions"},
		{mcp.Toolkit{Namespace: "filter", Functions: fn}, "built-in command"},
		{mcp.Toolkit{Namespace: "Graph", Functions: fn}, "namespace is taken"},
		{mcp.Toolkit{Namespace: "App", Functions: fn}, "namespace is taken"},
		{mcp.Toolkit{Namespace: "X", Functions: []mcp.Function{{Name: "help", Fn: echoArgs}}}, "toolkit's own"},
		{mcp.Toolkit{Namespace: "X", Functions: []mcp.Function{{Name: "f", Fn: echoArgs}, {Name: "f", Fn: echoArgs}}}, "declared twice"},
		{mcp.Toolkit{Namespace: "X", Functions: []mcp.Function{{Name: "f"}}}, "has no Fn"},
		{mcp.Toolkit{Namespace: "X", Functions: []mcp.Function{{Name: "f", Fn: echoArgs, InputSchema: json.RawMessage(`[]`)}}}, "must be a JSON object"},
	} {
		sh := funcShell(t)
		sh.AddLoadable(toolkit.GraphLoadable())
		if err := mcp.InstallToolkit(sh, c.tk); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: err %v, want %q", c.tk.Namespace, err, c.want)
		}
	}
}

// Ns.unload() and DenyLoadables work as for any loadable.
func TestFunctionUnloadDeny(t *testing.T) {
	sh := funcShell(t)
	if got, isErr := evalText(sh, `App.none({a: 1}); App.unload(); App.none({a: 2}).a`); isErr || got != "2" {
		t.Errorf("unload and reload = %q (err %v)", got, isErr)
	}
	sh = funcShell(t)
	sh.DenyLoadables("app")
	if got, isErr := evalText(sh, `App.none()`); !isErr {
		t.Errorf("denied = %q", got)
	}
}
