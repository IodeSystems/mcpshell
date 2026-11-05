package mcp_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
)

// TestInternalErrorKeepsServing: a Go panic in a command answers that eval
// with an error, logs the script and stack, and the server keeps serving
// with the exported state intact.
func TestInternalErrorKeepsServing(t *testing.T) {
	sh := coreShell()
	sh.Register(&runtime.CommandDef{Name: "boom", Description: "panics",
		Fn: func(args []runtime.Value) runtime.Value {
			var xs []int
			_ = xs[len(args)+3]
			return runtime.Null
		}})
	var log bytes.Buffer
	defer mcp.SetInternalErrorLog(&log)()

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"eval","arguments":{"code":"export let kept = 41"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"eval","arguments":{"code":"let marker = 7\nboom(marker)","vars":{"v":"var-marker"}}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"eval","arguments":{"code":"kept + 1"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := mcp.NewServer(sh, 16000).RunStdio(strings.NewReader(input), &out); err != nil {
		t.Fatalf("RunStdio: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d responses, want 3:\n%s", len(lines), out.String())
	}
	var failed, after map[string]any
	_ = json.Unmarshal([]byte(lines[1]), &failed)
	_ = json.Unmarshal([]byte(lines[2]), &after)

	msg := toolText(t, failed)
	if failed["result"].(map[string]any)["isError"] != true ||
		!strings.Contains(msg, "internal error: runtime error: index out of range") ||
		!strings.Contains(msg, "at line 2:") {
		t.Errorf("failed eval answered %q", msg)
	}
	if strings.Contains(msg, "goroutine") {
		t.Errorf("the stack reached the agent:\n%s", msg)
	}
	for _, want := range []string{"--- script ---\nlet marker = 7\nboom(marker)", "var-marker", "--- stack ---", "internal_error_test.go"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log missing %q:\n%s", want, log.String())
		}
	}
	if got := toolText(t, after); got != "42" {
		t.Errorf("eval after the panic = %q, want 42", got)
	}
}
