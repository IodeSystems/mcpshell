package mcp_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
)

// connectHTTP installs one HTTP upstream as namespace "remote".
func connectHTTP(t *testing.T, cfg mcp.ServerConfig) *runtime.Shell {
	t.Helper()
	sh := coreShell()
	closers := mcp.InstallClients(sh, []mcp.NamedServer{{Namespace: "remote", Config: cfg}})
	t.Cleanup(func() {
		for _, c := range closers {
			_ = c.Close()
		}
	})
	if len(closers) != 1 {
		t.Fatalf("expected the HTTP upstream to connect, got %d connection(s)", len(closers))
	}
	return sh
}

// fakeOpts configures fakeHTTPServer.
type fakeOpts struct {
	sse     bool          // reply with text/event-stream instead of JSON
	session bool          // issue, then require, an Mcp-Session-Id
	gen     *atomic.Int32 // session generation; bump it to expire the session
	token   *atomic.Value // if set, the only bearer accepted
	stall   time.Duration // delay before answering tools/call
	tools   []string      // tool names to advertise (default: just "echo")
}

// fakeHTTPServer answers MCP over streamable HTTP. Every request after
// initialize must carry the MCP-Protocol-Version header.
func fakeHTTPServer(t *testing.T, o fakeOpts) *httptest.Server {
	t.Helper()
	if o.gen == nil {
		o.gen = new(atomic.Int32)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o.token != nil && r.Header.Get("Authorization") != "Bearer "+o.token.Load().(string) {
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
		sid := fmt.Sprintf("sess-%d", o.gen.Load())
		if req.Method != "initialize" {
			if got := r.Header.Get("MCP-Protocol-Version"); got != "2025-06-18" {
				http.Error(w, "bad MCP-Protocol-Version "+got, http.StatusBadRequest)
				return
			}
			if o.session {
				switch got := r.Header.Get("Mcp-Session-Id"); {
				case got == "":
					http.Error(w, "missing session", http.StatusBadRequest)
					return
				case got != sid:
					http.Error(w, "unknown session", http.StatusNotFound)
					return
				}
			}
		}
		if len(req.ID) == 0 { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var res any
		switch req.Method {
		case "initialize":
			if o.session {
				w.Header().Set("Mcp-Session-Id", sid)
			}
			res = map[string]any{
				"protocolVersion": "2025-06-18",
				"serverInfo":      map[string]any{"name": "remote", "version": "1"},
			}
		case "tools/list":
			names := o.tools
			if names == nil {
				names = []string{"echo"}
			}
			var tools []map[string]any
			for _, n := range names {
				tools = append(tools, map[string]any{
					"name":        n,
					"description": "echoes the message. Second sentence.\nMore detail.",
					"inputSchema": json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
				})
			}
			res = map[string]any{"tools": tools}
		case "tools/call":
			time.Sleep(o.stall)
			var p struct {
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			res = map[string]any{"content": []map[string]any{{"type": "text", "text": fmt.Sprint("echo: ", p.Arguments["message"])}}}
		}
		msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
		if o.sse {
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
			srv := fakeHTTPServer(t, fakeOpts{sse: tc.sse, session: tc.session})
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
	srv := fakeHTTPServer(t, fakeOpts{token: &token})
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

// TestHTTPSessionExpiry covers a server that drops the session mid-run: the
// 404 makes the client re-initialize and retry instead of failing for good.
func TestHTTPSessionExpiry(t *testing.T) {
	gen := new(atomic.Int32)
	srv := fakeHTTPServer(t, fakeOpts{session: true, gen: gen})
	defer srv.Close()
	sh := connectHTTP(t, mcp.ServerConfig{URL: srv.URL})

	gen.Add(1) // server forgets sess-0
	v, err := sh.Eval(`remote.echo("again")`)
	if err != nil {
		t.Fatalf("remote.echo after session expiry errored: %v", err)
	}
	if v.Display() != "echo: again" {
		t.Errorf("got %q", v.Display())
	}
}

// TestHTTPTimeout covers a server that never answers: the call must fail
// rather than hang eval.
func TestHTTPTimeout(t *testing.T) {
	defer mcp.SetHTTPTimeout(200 * time.Millisecond)()
	srv := fakeHTTPServer(t, fakeOpts{stall: 700 * time.Millisecond})
	defer srv.Close()
	sh := connectHTTP(t, mcp.ServerConfig{URL: srv.URL})

	start := time.Now()
	_, err := sh.Eval(`remote.echo("slow")`)
	if err == nil {
		t.Fatal("expected a timeout error from a stalled server")
	}
	if !strings.Contains(err.Error(), "Timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("error does not look like a timeout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Errorf("call took %v, want about 200ms", elapsed)
	}
}

// manyTools is 20 agents_*, 5 creative_*, one unprefixed tool, and a tool
// named "agents" that collides with the agents category.
func manyTools() []string {
	var names []string
	for i := 0; i < 20; i++ {
		names = append(names, fmt.Sprintf("agents_%02d", i))
	}
	for i := 0; i < 5; i++ {
		names = append(names, fmt.Sprintf("creative_%d", i))
	}
	return append(names, "lonely", "agents")
}

func evalString(t *testing.T, sh *runtime.Shell, src string) string {
	t.Helper()
	v, err := sh.Eval(src)
	if err != nil {
		t.Fatalf("%s errored: %v", src, err)
	}
	return v.Display()
}

func TestLargeUpstreamIsCategorized(t *testing.T) {
	srv := fakeHTTPServer(t, fakeOpts{tools: manyTools()})
	defer srv.Close()
	sh := connectHTTP(t, mcp.ServerConfig{URL: srv.URL})

	guide := evalString(t, sh, `help("remote")`)
	for _, want := range []string{
		"27 tools in 3 categories",
		`agents_group (21): agents, agents_00, agents_01, … — help("remote.agents_group")`,
		`creative (5)`, `other (1): lonely`,
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("help(\"remote\") missing %q:\n%s", want, guide)
		}
	}
	if strings.Contains(guide, "remote.agents_05(") {
		t.Errorf("help(\"remote\") must not list every tool:\n%s", guide)
	}

	cat := evalString(t, sh, `help("remote.creative")`)
	if !strings.Contains(cat, "remote.creative_3 — echoes the message") || strings.Contains(cat, "agents") {
		t.Errorf("help(\"remote.creative\") = %s", cat)
	}
	// The tool named like its prefix keeps its own detailed help.
	if got := evalString(t, sh, `help("remote.agents")`); !strings.Contains(got, "message") || strings.Contains(got, "commands):") {
		t.Errorf("help(\"remote.agents\") should be the tool's own help, got:\n%s", got)
	}

	all := evalString(t, sh, `help()`)
	if !strings.Contains(all, `remote: 27 commands — help("remote")`) || strings.Contains(all, "remote.agents_00") {
		t.Errorf("help() must collapse the large namespace:\n%s", all)
	}
	for _, compact := range []bool{true, false} {
		if p := sh.ToPrompt(compact); !strings.Contains(p, `remote: 27 commands`) || strings.Contains(p, "remote.agents_00") {
			t.Errorf("ToPrompt(%v) must collapse the large namespace", compact)
		}
	}
	if got := evalString(t, sh, `remote.agents_07("hi")`); got != "echo: hi" {
		t.Errorf("categorized tool call = %q", got)
	}
}

func TestExplicitCategories(t *testing.T) {
	srv := fakeHTTPServer(t, fakeOpts{tools: []string{"tts_speak", "tts_voices", "billing"}})
	defer srv.Close()
	sh := connectHTTP(t, mcp.ServerConfig{URL: srv.URL, Categories: map[string][]string{"voice": {"tts_*"}}})

	guide := evalString(t, sh, `help("remote")`)
	if !strings.Contains(guide, "voice (2): tts_speak, tts_voices") || !strings.Contains(guide, "other (1): billing") {
		t.Errorf("explicit categories not applied:\n%s", guide)
	}
}

func TestSmallUpstreamIsNotCategorized(t *testing.T) {
	srv := fakeHTTPServer(t, fakeOpts{tools: []string{"a_one", "a_two", "b_one", "b_two"}})
	defer srv.Close()
	sh := connectHTTP(t, mcp.ServerConfig{URL: srv.URL})
	if guide := evalString(t, sh, `help("remote")`); strings.Contains(guide, "categories") || !strings.Contains(guide, "remote.b_two(") {
		t.Errorf("a small upstream should list its tools flat:\n%s", guide)
	}
}

func TestNonIdentifierToolNamesShowBracketCall(t *testing.T) {
	srv := fakeHTTPServer(t, fakeOpts{tools: []string{"grafana-search_docs", "grafana-list_alerts"}})
	defer srv.Close()
	sh := connectHTTP(t, mcp.ServerConfig{URL: srv.URL})
	if guide := evalString(t, sh, `help("remote")`); !strings.Contains(guide, `remote["grafana-search_docs"](`) {
		t.Errorf("guide should show the bracket call form:\n%s", guide)
	}
	if got := evalString(t, sh, `help("remote.grafana-search_docs")`); !strings.HasPrefix(got, `remote["grafana-search_docs"](`) {
		t.Errorf("detailed help should show the bracket call form:\n%s", got)
	}
	if got := evalString(t, sh, `remote["grafana-search_docs"]("hi")`); got != "echo: hi" {
		t.Errorf("bracket call = %q", got)
	}
}
