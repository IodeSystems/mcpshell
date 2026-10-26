package mcp_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func coreShell() *runtime.Shell {
	return toolkit.InstallCore(runtime.NewShell())
}

// --- server protocol ---------------------------------------------------------

func TestServerProtocol(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"eval","arguments":{"code":"2 + 3"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"eval","arguments":{"code":"nope("}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"eval","arguments":{"code":"x * 2","vars":{"x":21}}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"eval","arguments":{"code":"o","vars":{"o":{"z":12345678901234567890123,"a":0.12345678901234567890}}}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"eval","arguments":{"code":"floor(2.7, 9)"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"eval","arguments":{"code":"let all = 1\nfloor(2.7, 9) + nope"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := mcp.NewServer(coreShell(), 16000).RunStdio(strings.NewReader(input), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}

	var resps []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad response line %q: %v", line, err)
		}
		resps = append(resps, m)
	}
	// The notification produces no response: 8 requests → 8 responses.
	if len(resps) != 8 {
		t.Fatalf("expected 8 responses, got %d", len(resps))
	}

	// initialize
	si := resps[0]["result"].(map[string]any)["serverInfo"].(map[string]any)
	if si["name"] != "mcpshell" {
		t.Errorf("serverInfo.name = %v, want mcpshell", si["name"])
	}
	// tools/list
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	// eval success
	if got := toolText(t, resps[2]); got != "5" {
		t.Errorf("eval 2+3 = %q, want 5", got)
	}
	// eval error → isError
	if !resps[3]["result"].(map[string]any)["isError"].(bool) {
		t.Errorf("syntax error should set isError")
	}
	// eval with vars
	if got := toolText(t, resps[4]); got != "42" {
		t.Errorf("eval with vars = %q, want 42", got)
	}
	// vars keep exact numbers and key order
	if got := toolText(t, resps[5]); got != "{z: 12345678901234567890123, a: 0.1234567890123456789}" {
		t.Errorf("eval with exact vars = %q", got)
	}
	// warnings follow the result, and an error
	want := "2\n\nwarning: line 1: floor(n: number) takes 1 argument(s); ignored the extra 9."
	if got := toolText(t, resps[6]); !strings.HasPrefix(got, want) {
		t.Errorf("eval with a dropped argument = %q, want prefix %q", got, want)
	}
	got := toolText(t, resps[7])
	for _, sub := range []string{"ERROR:", "warning: line 1: 'all' replaces the builtin", "warning: line 2: floor"} {
		if !strings.Contains(got, sub) {
			t.Errorf("failed eval = %q, want %q", got, sub)
		}
	}
}

func toolText(t *testing.T, resp map[string]any) string {
	t.Helper()
	content := resp["result"].(map[string]any)["content"].([]any)
	return content[0].(map[string]any)["text"].(string)
}

// --- upstream client + self-omission -----------------------------------------

// helperCommand re-invokes the test binary as a fake MCP server identifying
// itself as serverName and exposing a single tool named toolName.
func helperCommand(serverName, toolName string) ([]string, map[string]string) {
	return []string{os.Args[0], "-test.run=TestHelperProcess"},
		map[string]string{"GO_MCP_HELPER": serverName, "GO_MCP_TOOL": toolName}
}

func TestInstallClientsOmitsMcpshell(t *testing.T) {
	sh := coreShell()
	cmd, env := helperCommand("mcpshell", "echo") // upstream identifies itself as mcpshell
	closers := mcp.InstallClients(sh, []mcp.NamedServer{
		{Namespace: "selfish", Config: mcp.ServerConfig{Command: cmd, Env: env}},
	})
	for _, c := range closers {
		defer c.Close()
	}
	if len(closers) != 0 {
		t.Fatalf("an upstream identifying as mcpshell must be skipped, got %d connection(s)", len(closers))
	}
	if _, err := sh.Eval(`selfish.echo("x")`); err == nil {
		t.Errorf("the skipped upstream must not register any commands")
	}
}

// TestInstallClientsOmitsToolCollision covers an upstream that does NOT name
// itself mcpshell but exposes a reserved tool name (eval) — still skipped.
func TestInstallClientsOmitsToolCollision(t *testing.T) {
	sh := coreShell()
	cmd, env := helperCommand("sneaky", "eval")
	closers := mcp.InstallClients(sh, []mcp.NamedServer{
		{Namespace: "sneaky", Config: mcp.ServerConfig{Command: cmd, Env: env}},
	})
	for _, c := range closers {
		defer c.Close()
	}
	if len(closers) != 0 {
		t.Fatalf("an upstream exposing an 'eval' tool must be skipped, got %d connection(s)", len(closers))
	}
	if _, err := sh.Eval(`sneaky.eval("x")`); err == nil {
		t.Errorf("the skipped upstream must not register any commands")
	}
}

func TestInstallClientsRegistersUpstream(t *testing.T) {
	sh := coreShell()
	cmd, env := helperCommand("widgets", "echo") // a normal (non-mcpshell) upstream
	closers := mcp.InstallClients(sh, []mcp.NamedServer{
		{Namespace: "app", Config: mcp.ServerConfig{Command: cmd, Env: env}},
	})
	for _, c := range closers {
		defer c.Close()
	}
	if len(closers) != 1 {
		t.Fatalf("expected the upstream to connect, got %d connection(s)", len(closers))
	}
	v, err := sh.Eval(`app.echo("hi")`)
	if err != nil {
		t.Fatalf("app.echo errored: %v", err)
	}
	if v.Display() != "echo: hi" {
		t.Errorf("app.echo(\"hi\") = %q, want %q", v.Display(), "echo: hi")
	}
}

// TestHelperProcess is not a real test: when GO_MCP_HELPER is set the test
// binary instead behaves as a minimal MCP server, used by the tests above.
func TestHelperProcess(t *testing.T) {
	name := os.Getenv("GO_MCP_HELPER")
	if name == "" {
		return
	}
	runFakeMCPServer(name)
	os.Exit(0)
}

func runFakeMCPServer(name string) {
	toolName := os.Getenv("GO_MCP_TOOL")
	if toolName == "" {
		toolName = "echo"
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(line, &req) != nil || len(req.ID) == 0 {
			continue
		}
		var res any
		switch req.Method {
		case "initialize":
			res = map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": name, "version": "0.0.1"},
			}
		case "tools/list":
			res = map[string]any{"tools": []map[string]any{{
				"name":        toolName,
				"description": "echoes the message",
				"inputSchema": json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
			}}}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			msg, _ := p.Arguments["message"].(string)
			res = map[string]any{
				"content": []map[string]any{{"type": "text", "text": "echo: " + msg}},
				"isError": false,
			}
		default:
			continue
		}
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
	}
}

func TestOverflowSavedToFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-1")
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"eval","arguments":{"code":"\"x\".repeat(500)"}}}` + "\n"
	var out bytes.Buffer
	if err := mcp.NewServer(coreShell(), 100).WithOverflowDir(dir).RunStdio(strings.NewReader(input), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	text := toolText(t, resp)
	files, _ := filepath.Glob(filepath.Join(dir, "eval-*.txt"))
	if len(files) != 1 {
		t.Fatalf("expected 1 overflow file in %s, got %v", dir, files)
	}
	if !strings.Contains(text, "OUTPUT TRUNCATED at 100 of 500 bytes. Full output saved to "+files[0]+" (text)") {
		t.Errorf("response should name the file:\n%s", text)
	}
	if b, _ := os.ReadFile(files[0]); string(b) != strings.Repeat("x", 500) {
		t.Errorf("overflow file holds %d bytes, want the full 500", len(b))
	}
}

func TestOverflowArrayIsJSONLines(t *testing.T) {
	dir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"eval","arguments":{"code":"range(0, 50) |> map(i => ({id: i, s: \"a\\\"b\"}))"}}}` + "\n"
	var out bytes.Buffer
	if err := mcp.NewServer(coreShell(), 100).WithOverflowDir(dir).RunStdio(strings.NewReader(input), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "eval-*.txt"))
	if len(files) != 1 {
		t.Fatalf("expected 1 overflow file, got %v", files)
	}
	b, _ := os.ReadFile(files[0])
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 50 || lines[1] != `{"id":1,"s":"a\"b"}` {
		t.Errorf("overflow file should be JSON Lines, got %d lines, line 2 = %q", len(lines), lines[1])
	}
}
