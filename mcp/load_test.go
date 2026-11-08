package mcp_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
)

// load() lasts the MCP session: the server keeps one shell across evals.
func TestLoadPersistsAcrossEvals(t *testing.T) {
	sh := coreShell()
	sh.AddLoadable(runtime.Loadable{
		Name: "demo", Summary: "one command",
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			sh.Register(&runtime.CommandDef{Namespace: "Demo", Name: "one", Fn: func([]runtime.Value) runtime.Value { return runtime.Num(1) }})
			return nil, nil
		},
	})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"eval","arguments":{"code":"Demo.one()"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"eval","arguments":{"code":"load(\"demo\")"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"eval","arguments":{"code":"Demo.one()"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := mcp.NewServer(sh, 16000).RunStdio(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d responses", len(lines))
	}
	if !strings.Contains(lines[0], `call load(`) || !strings.Contains(lines[0], `"isError":true`) {
		t.Errorf("before load: %s", lines[0])
	}
	if !strings.Contains(lines[1], "Demo.one") || strings.Contains(lines[1], `"isError":true`) {
		t.Errorf("load: %s", lines[1])
	}
	if !strings.Contains(lines[2], `"text":"1"`) {
		t.Errorf("after load, in a later eval: %s", lines[2])
	}
}
