package mcp_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
)

// fakeHTTPServer answers MCP over streamable HTTP. sse selects event-stream
// replies; session makes it issue (and then require) an Mcp-Session-Id;
// token, if set, is the only bearer accepted.
func fakeHTTPServer(t *testing.T, sse, session bool, token *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != nil && r.Header.Get("Authorization") != "Bearer "+token.Load().(string) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodDelete {
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if session && req.Method != "initialize" && r.Header.Get("Mcp-Session-Id") != "sess-1" {
			http.Error(w, "missing session", http.StatusBadRequest)
			return
		}
		if len(req.ID) == 0 { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var res any
		switch req.Method {
		case "initialize":
			if session {
				w.Header().Set("Mcp-Session-Id", "sess-1")
			}
			res = map[string]any{"serverInfo": map[string]any{"name": "remote", "version": "1"}}
		case "tools/list":
			res = map[string]any{"tools": []map[string]any{{
				"name":        "echo",
				"description": "echoes the message",
				"inputSchema": json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
			}}}
		case "tools/call":
			var p struct {
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			res = map[string]any{"content": []map[string]any{{"type": "text", "text": fmt.Sprint("echo: ", p.Arguments["message"])}}}
		}
		msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
			// A notification first: the client must skip it and find the response.
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(msg)
	}))
}

func TestHTTPUpstream(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sse, session bool
	}{
		{"json-stateless", false, false},
		{"sse-stateless", true, false},
		{"json-session", false, true},
		{"sse-session", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeHTTPServer(t, tc.sse, tc.session, nil)
			defer srv.Close()
			sh := coreShell()
			closers := mcp.InstallClients(sh, []mcp.NamedServer{
				{Namespace: "remote", Config: mcp.ServerConfig{URL: srv.URL}},
			})
			for _, c := range closers {
				defer c.Close()
			}
			if len(closers) != 1 {
				t.Fatalf("expected the HTTP upstream to connect, got %d connection(s)", len(closers))
			}
			v, err := sh.Eval(`remote.echo("hi")`)
			if err != nil {
				t.Fatalf("remote.echo errored: %v", err)
			}
			if v.Display() != "echo: hi" {
				t.Errorf("remote.echo(\"hi\") = %q, want %q", v.Display(), "echo: hi")
			}
		})
	}
}

// TestHTTPAuthCmdRefresh covers a token that rotates mid-session: the cached
// token gets a 401, auth_cmd runs again, and the request is retried.
func TestHTTPAuthCmdRefresh(t *testing.T) {
	var token atomic.Value
	token.Store("tok-1")
	srv := fakeHTTPServer(t, false, false, &token)
	defer srv.Close()

	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("tok-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := coreShell()
	closers := mcp.InstallClients(sh, []mcp.NamedServer{
		{Namespace: "remote", Config: mcp.ServerConfig{URL: srv.URL, AuthCmd: "cat " + tokenFile}},
	})
	for _, c := range closers {
		defer c.Close()
	}
	if len(closers) != 1 {
		t.Fatalf("expected the HTTP upstream to connect, got %d connection(s)", len(closers))
	}

	token.Store("tok-2")
	if err := os.WriteFile(tokenFile, []byte("tok-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := sh.Eval(`remote.echo("after rotation")`)
	if err != nil {
		t.Fatalf("remote.echo after token rotation errored: %v", err)
	}
	if v.Display() != "echo: after rotation" {
		t.Errorf("got %q", v.Display())
	}
}
