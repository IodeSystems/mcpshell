package mcp_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
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
	// The notification produces no response: 5 requests → 5 responses.
	if len(resps) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(resps))
	}

	// initialize
	si := resps[0]["result"].(map[string]any)["serverInfo"].(map[string]any)
	if si["name"] != "mcpshell" {
		t.Errorf("serverInfo.name = %v, want mcpshell", si["name"])
	}
	// tools/list
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
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
