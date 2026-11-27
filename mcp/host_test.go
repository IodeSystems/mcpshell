package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/mcp"
)

// hostDecl is the client's initialize capability that declares exec and
// sleep; sleep's own timeout is timeoutMs.
func hostDecl(timeoutMs int) map[string]any {
	return map[string]any{"mcpshell/host": map[string]any{"functions": []any{
		map[string]any{"name": "exec", "description": "Run a shell command in the workspace.\nMore detail.",
			"inputSchema": map[string]any{"type": "object", "required": []any{"cmd"},
				"properties": map[string]any{"cmd": map[string]any{"type": "string"}, "timeout": map[string]any{"type": "integer"}}}},
		map[string]any{"name": "sleep", "description": "Wait.", "timeoutMs": timeoutMs,
			"inputSchema": map[string]any{"type": "object"}},
	}}}
}

// hostClient starts a server (allowed: --host-functions) and initializes it
// with experimental capabilities exp (nil: none).
func hostClient(t *testing.T, allowed bool, exp map[string]any) *fakeClient {
	t.Helper()
	srv := mcp.NewServer(coreShell(), 16000)
	if allowed {
		srv.WithHostFunctions()
	}
	c := startClient(t, srv)
	caps := map[string]any{}
	if exp != nil {
		caps["experimental"] = exp
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": caps}})
	c.must("initialize")
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return c
}

func (c *fakeClient) description() string {
	c.t.Helper()
	c.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	m := c.must("tools/list")
	return m["result"].(map[string]any)["tools"].([]any)[0].(map[string]any)["description"].(string)
}

func (c *fakeClient) startEval(id int, args map[string]any) {
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": "eval", "arguments": args}})
}

// expectHostCall waits for a mcpshell/host/call and returns it.
func (c *fakeClient) expectHostCall() map[string]any {
	c.t.Helper()
	m := c.must("host call")
	if m["method"] != "mcpshell/host/call" {
		c.t.Fatalf("expected mcpshell/host/call, got %v", m)
	}
	return m
}

// evalAnswer waits for the eval answer id, failing on any other message.
func (c *fakeClient) evalAnswer(id int) (string, bool) {
	c.t.Helper()
	m := c.must("eval answer")
	if m["id"] != float64(id) {
		c.t.Fatalf("expected the eval answer %d, got %v", id, m)
	}
	return toolText(c.t, m), m["result"].(map[string]any)["isError"].(bool)
}

// expectCancelled waits for notifications/cancelled and returns its params.
func (c *fakeClient) expectCancelled() map[string]any {
	c.t.Helper()
	m := c.must("notifications/cancelled")
	if m["method"] != "notifications/cancelled" {
		c.t.Fatalf("expected notifications/cancelled, got %v", m)
	}
	return m["params"].(map[string]any)
}

func TestHostCall(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	c := hostClient(t, true, hostDecl(0))
	desc := c.description()
	for _, want := range []string{"Host functions (", "Host.exec(cmd: string, timeout?: integer) — Run a shell command in the workspace.", "Host.sleep()", "Toolkits (Ns.help() describes one): ", "Host"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
	if strings.Contains(desc, "More detail") {
		t.Errorf("description has more than the first line:\n%s", desc)
	}

	// Round trip: arguments go out as JSON, a nested result comes back as a value.
	c.startEval(10, map[string]any{"code": `let r = Host.exec({cmd: "ls", timeout: "5"}); [r.exit, r.out.lines[1], r.big]`})
	call := c.expectHostCall()
	p := call["params"].(map[string]any)
	args := p["arguments"].(map[string]any)
	if p["name"] != "exec" || args["cmd"] != "ls" || args["timeout"] != float64(5) {
		t.Errorf("host call params = %v", p)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": call["id"], "result": map[string]any{
		"exit": 1, "out": map[string]any{"lines": []any{"a", "b"}}, "big": json.Number("12345678901234567890")}})
	if got, isErr := c.evalAnswer(10); isErr || got != "[1, \"b\", 12345678901234567890]" {
		t.Errorf("eval = %q, %v", got, isErr)
	}

	// A JSON-RPC error is a thrown error that try/catch catches.
	c.startEval(11, map[string]any{"code": `try { Host.exec("false") } catch (e) { "caught: " + e }`})
	call = c.expectHostCall()
	if a := call["params"].(map[string]any)["arguments"].(map[string]any); a["cmd"] != "false" {
		t.Errorf("positional argument = %v", a)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": call["id"], "error": map[string]any{"code": -32000, "message": "permission denied"}})
	if got, isErr := c.evalAnswer(11); isErr || !strings.HasPrefix(got, "caught: Host.exec failed: permission denied") {
		t.Errorf("caught host error = %q, %v", got, isErr)
	}

	// A missing required argument fails before the call.
	if got, isErr := c.eval(12, `Host.exec({})`); !isErr || !strings.Contains(got, "missing required argument: cmd") {
		t.Errorf("missing cmd = %q", got)
	}
	// Host.help() describes the functions; exec points at Host.exec.
	if got, _ := c.eval(13, `Host.help()`); !strings.Contains(got, "Host.exec(cmd: string, timeout?: integer)") || !strings.Contains(got, "Host.sleep") {
		t.Errorf("Host.help() = %q", got)
	}
	if got, isErr := c.eval(14, `exec("ls")`); !isErr || !strings.Contains(got, "The host provides Host.exec") {
		t.Errorf("exec hint = %q", got)
	}
	// clear: true unloads Host; the next reference loads it again.
	c.startEval(15, map[string]any{"clear": true, "code": `Host.sleep()`})
	call = c.expectHostCall()
	c.send(map[string]any{"jsonrpc": "2.0", "id": call["id"], "result": "done"})
	if got, isErr := c.evalAnswer(15); isErr || !strings.HasPrefix(got, "done") || !strings.Contains(got, "unloaded host") {
		t.Errorf("after clear = %q, %v", got, isErr)
	}
	c.close()
}

// No answer in the function's timeoutMs: notifications/cancelled with the
// request's id, and the eval gets an error.
func TestHostCallTimeout(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	c := hostClient(t, true, hostDecl(100))
	c.startEval(10, map[string]any{"code": `Host.sleep()`})
	call := c.expectHostCall()
	cp := c.expectCancelled()
	if cp["requestId"] != call["id"] || cp["reason"] != "the function's timeoutMs was reached" {
		t.Errorf("cancelled = %v for request %v", cp, call["id"])
	}
	if got, isErr := c.evalAnswer(10); !isErr || !strings.Contains(got, "Host.sleep: no answer within its timeout (100ms") {
		t.Errorf("eval = %q", got)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": call["id"], "result": map[string]any{}}) // late: dropped
	if got, isErr := c.eval(11, `1 + 1`); isErr || got != "2" {
		t.Errorf("after a late answer = %q", got)
	}
	c.close()
}

// The eval's time limit caps a longer timeoutMs.
func TestHostCallEvalLimit(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	c := hostClient(t, true, hostDecl(60_000))
	start := time.Now()
	c.startEval(10, map[string]any{"code": `Host.sleep()`, "limits": map[string]any{"timeout": 300}})
	call := c.expectHostCall()
	cp := c.expectCancelled()
	if cp["requestId"] != call["id"] || cp["reason"] != "the eval's time limit was reached" {
		t.Errorf("cancelled = %v for request %v", cp, call["id"])
	}
	got, isErr := c.evalAnswer(10)
	if !isErr || !strings.Contains(got, "no answer within the eval's time limit (300ms)") || !strings.Contains(got, "limits: {timeout: 600}") {
		t.Errorf("eval = %q", got)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
	c.close()
}

// The input ends while a call waits: the call is cancelled and the server
// returns.
func TestHostCallConnectionClosed(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	c := hostClient(t, true, hostDecl(0))
	c.startEval(10, map[string]any{"code": `Host.sleep()`})
	call := c.expectHostCall()
	_ = c.w.Close()
	cp := c.expectCancelled()
	if cp["requestId"] != call["id"] || cp["reason"] != "connection closed" {
		t.Errorf("cancelled = %v", cp)
	}
	if got, isErr := c.evalAnswer(10); !isErr || !strings.Contains(got, "Host.sleep failed: connection closed") {
		t.Errorf("eval = %q", got)
	}
	for c.next(5*time.Second) != nil {
	}
	if err := <-c.errc; err != nil {
		t.Errorf("RunStdio: %v", err)
	}
}

// Host needs both the operator flag and the client's declaration.
func TestHostOptIn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed bool
		exp     map[string]any
		log     string
	}{
		{"flag without declaration", true, nil, ""},
		{"declaration without flag", false, hostDecl(0), "ignored (the operator did not pass --host-functions)"},
		{"empty declaration", true, map[string]any{"mcpshell/host": map[string]any{"functions": []any{}}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &lockedBuf{}
			defer mcp.SetServerLog(log)()
			c := hostClient(t, tc.allowed, tc.exp)
			if desc := c.description(); strings.Contains(desc, "Host") {
				t.Errorf("description has Host:\n%s", desc)
			}
			if got, _ := c.eval(10, `typeof Host`); got != "undefined" {
				t.Errorf("typeof Host = %q", got)
			}
			if got, isErr := c.eval(11, `exec("ls")`); !isErr || !strings.Contains(got, "use the host's exec tool") {
				t.Errorf("exec hint = %q", got)
			}
			c.close()
			if tc.log != "" && !strings.Contains(log.String(), tc.log) {
				t.Errorf("log lacks %q:\n%s", tc.log, log)
			}
		})
	}
}

// Without exec, the shell hint names the functions there are.
func TestHostShellHintWithoutExec(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	c := hostClient(t, true, map[string]any{"mcpshell/host": map[string]any{"functions": []any{
		map[string]any{"name": "readClipboard", "description": "Clipboard text."}}}})
	if got, isErr := c.eval(10, `Shell.run("ls")`); !isErr || !strings.Contains(got, "The host provides Host.readClipboard (Host.help() describes them); for other commands use the host's exec tool.") {
		t.Errorf("hint = %q", got)
	}
	c.close()
}

// evalRequestId is the id of the eval tools/call, raw, for numeric and
// string ids; each sequential eval carries its own.
func TestHostCallEvalRequestId(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	c := hostClient(t, true, hostDecl(0))
	for _, id := range []any{41, "req-α/7", 42} {
		c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{"name": "eval", "arguments": map[string]any{"code": `Host.sleep()`}}})
		call := c.expectHostCall()
		p := call["params"].(map[string]any)
		want := id
		if n, ok := id.(int); ok {
			want = float64(n)
		}
		if p["evalRequestId"] != want {
			t.Errorf("evalRequestId = %#v, want %#v", p["evalRequestId"], want)
		}
		c.send(map[string]any{"jsonrpc": "2.0", "id": call["id"], "result": "ok"})
		m := c.must("eval answer")
		if m["id"] != want {
			t.Errorf("answer id = %v, want %v", m["id"], want)
		}
	}
	c.close()
}
