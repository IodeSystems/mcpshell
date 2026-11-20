package mcp_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/mcp"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// lockedBuf is a bytes.Buffer safe for the server's background goroutines.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// fakeClient drives a Server over pipes as an MCP client would.
type fakeClient struct {
	t    *testing.T
	w    *io.PipeWriter
	msgs chan map[string]any
	errc chan error
	seen []map[string]any // every message the server sent
}

func startClient(t *testing.T, srv *mcp.Server) *fakeClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &fakeClient{t: t, w: inW, msgs: make(chan map[string]any, 64), errc: make(chan error, 1)}
	go func() {
		err := srv.RunStdio(inR, outW)
		_ = outW.Close()
		c.errc <- err
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("bad server line %q: %v", sc.Text(), err)
				continue
			}
			c.msgs <- m
		}
		close(c.msgs)
	}()
	return c
}

func (c *fakeClient) send(v any) {
	c.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.w.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// next is the server's next message, or nil after wait.
func (c *fakeClient) next(wait time.Duration) map[string]any {
	select {
	case m, ok := <-c.msgs:
		if !ok {
			return nil
		}
		c.seen = append(c.seen, m)
		return m
	case <-time.After(wait):
		return nil
	}
}

func (c *fakeClient) must(what string) map[string]any {
	c.t.Helper()
	m := c.next(5 * time.Second)
	if m == nil {
		c.t.Fatalf("%s: no message from the server", what)
	}
	return m
}

// init runs initialize and initialized; withRoots advertises the roots capability.
func (c *fakeClient) init(withRoots bool) {
	c.t.Helper()
	caps := map[string]any{}
	if withRoots {
		caps["roots"] = map[string]any{"listChanged": true}
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": caps}})
	if m := c.must("initialize"); m["id"] != float64(1) {
		c.t.Fatalf("initialize answer = %v", m)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
}

// expectRootsList waits for the server's roots/list request and returns its id.
func (c *fakeClient) expectRootsList() any {
	c.t.Helper()
	m := c.must("roots/list")
	if m["method"] != "roots/list" {
		c.t.Fatalf("expected roots/list, got %v", m)
	}
	return m["id"]
}

func (c *fakeClient) answerRoots(id any, roots ...map[string]any) {
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"roots": roots}})
}

func (c *fakeClient) listChanged() {
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/roots/list_changed"})
}

// eval sends an eval call and returns its text and isError.
func (c *fakeClient) eval(id int, code string) (string, bool) {
	c.t.Helper()
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": "eval", "arguments": map[string]any{"code": code}}})
	m := c.must("eval")
	if m["id"] != float64(id) {
		c.t.Fatalf("expected the eval answer %d, got %v", id, m)
	}
	return toolText(c.t, m), m["result"].(map[string]any)["isError"].(bool)
}

func (c *fakeClient) close() {
	c.t.Helper()
	_ = c.w.Close()
	for c.next(5*time.Second) != nil {
	}
	if err := <-c.errc; err != nil {
		c.t.Errorf("RunStdio: %v", err)
	}
}

// waitRoot waits until the root set holds path: an answer is applied off
// the read loop, after the client sent it.
func waitRoot(t *testing.T, roots *toolkit.FileRoots, path string) {
	t.Helper()
	for range 200 {
		for _, r := range roots.All() {
			if r.Path == path {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("root %s never applied", path)
}

func (c *fakeClient) sentRootsList() bool {
	for _, m := range c.seen {
		if m["method"] == "roots/list" {
			return true
		}
	}
	return false
}

func fileURI(p string) string { return (&url.URL{Scheme: "file", Path: filepath.ToSlash(p)}).String() }

// rootsServer is a server whose Files has workspace ws; clientRoots wires
// the client's roots in (--files-client-roots).
func rootsServer(t *testing.T, clientRoots bool) (*mcp.Server, *toolkit.FileRoots, string) {
	t.Helper()
	ws := t.TempDir()
	roots, err := toolkit.NewFileRoots(ws, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := toolkit.InstallCore(runtime.NewShell())
	sh.AddLoadable(toolkit.FilesLoadable(roots))
	srv := mcp.NewServer(sh, 16000)
	if clientRoots {
		srv.WithClientRoots(roots.SetClient)
	}
	return srv, roots, ws
}

// grantedDirs are two directories outside the workspace, each with a.txt;
// the first has a space in its name, so its URI has a percent-escape.
func grantedDirs(t *testing.T) (string, string) {
	base := t.TempDir()
	a, b := filepath.Join(base, "my notes"), filepath.Join(base, "scratch")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "a.txt"), []byte(filepath.Base(d)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return a, b
}

func q(p string) string { b, _ := json.Marshal(p); return string(b) }

func TestClientRoots(t *testing.T) {
	log := &lockedBuf{}
	defer mcp.SetServerLog(log)()
	srv, roots, _ := rootsServer(t, true)
	notes, scratch := grantedDirs(t)
	c := startClient(t, srv)
	c.init(true)

	id := c.expectRootsList()
	c.answerRoots(id, map[string]any{"uri": fileURI(notes), "name": "notes"},
		map[string]any{"uri": "https://example.com/x"})
	waitRoot(t, roots, notes)
	if got, isErr := c.eval(10, `Files.readLines(`+q(filepath.Join(notes, "a.txt"))+`)`); isErr || got != `["my notes"]` {
		t.Errorf("read in client root = %q, %v", got, isErr)
	}
	if got, isErr := c.eval(11, `Files.write(`+q(filepath.Join(notes, "b.txt"))+`, "x")`); !isErr ||
		!strings.Contains(got, "Read-only root: "+notes+" (read-only)") {
		t.Errorf("write to read-only client root = %q", got)
	}
	if !strings.Contains(log.String(), `"https://example.com/x" ignored: not a file:// URI`) {
		t.Errorf("log lacks the ignored URI:\n%s", log)
	}

	// list_changed mid-session: the new set replaces the old one.
	c.listChanged()
	id = c.expectRootsList()
	c.answerRoots(id, map[string]any{"uri": fileURI(scratch), "_meta": map[string]any{"mcpshell/access": "write"}})
	waitRoot(t, roots, scratch)
	if got, isErr := c.eval(12, `Files.write(`+q(filepath.Join(scratch, "b.txt"))+`, "w"); Files.readLines(`+q(filepath.Join(scratch, "b.txt"))+`)`); isErr || got != `["w"]` {
		t.Errorf("write in writable client root = %q, %v", got, isErr)
	}
	if got, isErr := c.eval(13, `Files.read(`+q(filepath.Join(notes, "a.txt"))+`)`); !isErr || !strings.Contains(got, "Access denied") ||
		!strings.Contains(got, scratch+" (read-write)") {
		t.Errorf("read in a removed root = %q", got)
	}
	c.close()
}

// A change applies inside a running eval: the eval polls a path until the
// client grants its root, which the server asks for while the eval runs.
func TestClientRootsDuringEval(t *testing.T) {
	defer mcp.SetServerLog(&lockedBuf{})()
	srv, _, _ := rootsServer(t, true)
	notes, _ := grantedDirs(t)
	c := startClient(t, srv)
	c.init(true)
	c.answerRoots(c.expectRootsList()) // no roots yet

	c.send(map[string]any{"jsonrpc": "2.0", "id": 20, "method": "tools/call", "params": map[string]any{"name": "eval",
		"arguments": map[string]any{"limits": map[string]any{"steps": 20000000},
			"code": `let r = null; let n = 0; while (r == null && n < 3000000) { n++; try { r = Files.readLines(` +
				q(filepath.Join(notes, "a.txt")) + `) } catch (e) {} }; r`}}})
	time.Sleep(50 * time.Millisecond)
	c.listChanged()
	id := c.expectRootsList() // sent while the eval runs: the read loop is not blocked
	c.answerRoots(id, map[string]any{"uri": fileURI(notes)})
	m := c.must("eval")
	if m["id"] != float64(20) || toolText(t, m) != `["my notes"]` {
		t.Errorf("eval during the grant = %v", m)
	}
	c.close()
}

// Without --files-client-roots, client roots are ignored: no roots/list.
func TestClientRootsDisabled(t *testing.T) {
	srv, _, _ := rootsServer(t, false)
	notes, _ := grantedDirs(t)
	c := startClient(t, srv)
	c.init(true)
	c.listChanged()
	if got, isErr := c.eval(10, `Files.exists(`+q(filepath.Join(notes, "a.txt"))+`)`); !isErr || !strings.Contains(got, "Access denied") {
		t.Errorf("client root without the flag = %q", got)
	}
	c.close()
	if c.sentRootsList() {
		t.Error("roots/list sent without --files-client-roots")
	}
}

// A client without the roots capability is never asked.
func TestClientRootsNoCapability(t *testing.T) {
	srv, _, _ := rootsServer(t, true)
	c := startClient(t, srv)
	c.init(false)
	c.listChanged()
	if got, isErr := c.eval(10, `1 + 1`); isErr || got != "2" {
		t.Errorf("eval = %q", got)
	}
	c.close()
	if c.sentRootsList() {
		t.Error("roots/list sent to a client without the roots capability")
	}
}

// A client that does not answer: the request times out, the previous set
// stays, and the server keeps answering.
func TestClientRootsTimeout(t *testing.T) {
	log := &lockedBuf{}
	defer mcp.SetServerLog(log)()
	defer mcp.SetRootsTimeout(100 * time.Millisecond)()
	srv, roots, _ := rootsServer(t, true)
	notes, _ := grantedDirs(t)
	c := startClient(t, srv)
	c.init(true)
	c.answerRoots(c.expectRootsList(), map[string]any{"uri": fileURI(notes)})
	waitRoot(t, roots, notes)

	c.listChanged()
	late := c.expectRootsList() // not answered in time
	for i := 0; i < 100 && !strings.Contains(log.String(), "no answer"); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(log.String(), "no answer in 100ms; keeping the previous client roots") {
		t.Errorf("log lacks the timeout:\n%s", log)
	}
	c.answerRoots(late) // too late: dropped
	if got, isErr := c.eval(10, `Files.readLines(`+q(filepath.Join(notes, "a.txt"))+`)`); isErr || got != `["my notes"]` {
		t.Errorf("previous root after a timeout = %q, %v", got, isErr)
	}
	c.close()
}
