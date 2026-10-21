package toolkit_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func chromeAvailable() bool {
	for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "headless-shell"} {
		if _, err := exec.LookPath(n); err == nil {
			return true
		}
	}
	return false
}

const browserTestPage = `<!doctype html><html><head><title>Test Page</title></head>
<body>
  <h1>Hello mcpshell</h1>
  <ul><li class="item">Alpha</li><li class="item">Beta</li></ul>
  <input id="q" type="text">
</body></html>`

func TestBrowserToolkit(t *testing.T) {
	if !chromeAvailable() {
		t.Skip("no Chrome/Chromium binary on PATH")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, browserTestPage)
	}))
	defer srv.Close()

	sh := toolkit.InstallCore(runtime.NewShell())
	bt := toolkit.InstallBrowser(sh, true)
	defer bt.Close()

	page, err := sh.EvalExported(`Browser.open(url).title`,
		map[string]runtime.Value{"url": runtime.Str(srv.URL)})
	if err != nil {
		t.Fatalf("Browser.open errored: %v", err)
	}
	if page.Display() != "Test Page" {
		t.Errorf("Browser.open(url).title = %q, want %q", page.Display(), "Test Page")
	}

	check := func(src, want string) {
		t.Helper()
		v, err := sh.Eval(src)
		if err != nil {
			t.Fatalf("eval(%q) errored: %v", src, err)
		}
		if got := v.Display(); got != want {
			t.Errorf("eval(%q) = %q, want %q", src, got, want)
		}
	}

	check(`Browser.title()`, "Test Page")
	check(`Browser.text("h1")`, "Hello mcpshell")
	check(`Browser.select(".item") |> map(e => e.text)`, `["Alpha", "Beta"]`)
	check(`Browser.select(".item")[0].tag`, "li")
	check(`Browser.eval("1 + 1")`, "2")
	check(`Browser.eval("document.querySelectorAll('.item').length")`, "2")
	check(`Browser.url()`, srv.URL+"/")
	check(`Browser.html("h1")`, "Hello mcpshell")
	check(`Browser.html().startsWith("<html>")`, "true")
	check(`Browser.eval("undefined")`, "null")
	check(`Browser.eval("null")`, "null")
	check(`Browser.eval("({a: [1, 'x']})").a[1]`, "x")
	check(`Browser.wait(".item")`, "null")
	check(`Browser.wait(".missing", {state: "detached"})`, "null")
	if _, err := sh.Eval(`Browser.wait(".missing", {timeout: 200})`); err == nil {
		t.Errorf("Browser.wait on a missing element should time out")
	}
	shot := filepath.Join(t.TempDir(), "page.png")
	if _, err := sh.EvalExported(`Browser.screenshot(p, {fullPage: true})`,
		map[string]runtime.Value{"p": runtime.Str(shot)}); err != nil {
		t.Fatalf("Browser.screenshot errored: %v", err)
	}
	if _, err := os.Stat(shot); err != nil {
		t.Errorf("screenshot not written: %v", err)
	}

	// Interaction: set an input value, read it back.
	if _, err := sh.Eval(`Browser.type("#q", "typed text")`); err != nil {
		t.Fatalf("Browser.type errored: %v", err)
	}
	check(`Browser.eval("document.getElementById('q').value")`, "typed text")
}
