package toolkit_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
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

	if _, err := sh.EvalExported(`Browser.open(url)`,
		map[string]runtime.Value{"url": runtime.Str(srv.URL)}); err != nil {
		t.Fatalf("Browser.open errored: %v", err)
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

	// Interaction: set an input value, read it back.
	if _, err := sh.Eval(`Browser.type("#q", "typed text")`); err != nil {
		t.Fatalf("Browser.type errored: %v", err)
	}
	check(`Browser.eval("document.getElementById('q').value")`, "typed text")
}
