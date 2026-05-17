package toolkit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func webShell() *runtime.Shell {
	return toolkit.InstallWeb(toolkit.InstallCore(runtime.NewShell()))
}

// evalWeb runs src with `url` bound to the test server address.
func evalWeb(t *testing.T, sh *runtime.Shell, url, src string) runtime.Value {
	t.Helper()
	v, err := sh.EvalExported(src, map[string]runtime.Value{"url": runtime.Str(url)})
	if err != nil {
		t.Fatalf("eval(%q) errored:\n%v", src, err)
	}
	return v
}

func TestWebFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"hello": "world", "n": 42}`))
		case "/echo":
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			_, _ = w.Write(append([]byte(r.Method+":"), body...))
		default:
			_, _ = w.Write([]byte("plain text"))
		}
	}))
	defer srv.Close()

	sh := webShell()

	if v := evalWeb(t, sh, srv.URL, `Web.fetch(url).status`); v.Display() != "200" {
		t.Errorf("status = %q, want 200", v.Display())
	}
	if v := evalWeb(t, sh, srv.URL, `Web.fetch(url).body`); v.Display() != "plain text" {
		t.Errorf("body = %q, want %q", v.Display(), "plain text")
	}
	if v := evalWeb(t, sh, srv.URL+"/json", `Web.fetch(url, {parse: "json"}).body.hello`); v.Display() != "world" {
		t.Errorf("parsed json body.hello = %q, want world", v.Display())
	}
	if v := evalWeb(t, sh, srv.URL+"/echo", `Web.fetch(url, {method: "POST", body: "ping"}).body`); v.Display() != "POST:ping" {
		t.Errorf("POST echo = %q, want POST:ping", v.Display())
	}
	// Second GET should hit the cache and still succeed.
	if v := evalWeb(t, sh, srv.URL, `Web.fetch(url).status`); v.Display() != "200" {
		t.Errorf("cached status = %q, want 200", v.Display())
	}
	if v, err := sh.Eval(`Web.clearCache()`); err != nil || v.Display() == "0" {
		t.Errorf("clearCache should report cleared entries, got %v (err %v)", v, err)
	}
}

func TestHtmlNamespace(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"select", `Html.select("<div><p>hello</p><p>world</p></div>", "p") |> map(e => e.text)`, `["hello", "world"]`},
		{"select tag", `Html.select("<p>hi</p>", "p") |> map(e => e.tag)`, `["p"]`},
		{"select attrs", `Html.select(r1, "a") |> map(e => e.attrs.href)`, `["/x"]`},
		{"text", `Html.text("<p>Hello <b>world</b></p>")`, "Hello world"},
		{"text strips script", `Html.text("<p>keep</p><script>drop()</script>")`, "keep"},
		{"links", `Html.links(r1) |> map(l => l.href)`, `["/x"]`},
		{"links text", `Html.links(r1) |> map(l => l.text)`, `["go"]`},
		{"table", `Html.table(r2) |> map(row => row.A)`, `["1", "3"]`},
		{"table headers", `Html.table(r2)[0].B`, "2"},
	}
	sh := webShell()
	vars := map[string]runtime.Value{
		"r1": runtime.Str(`<nav><a href="/x">go</a></nav>`),
		"r2": runtime.Str(`<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr><tr><td>3</td><td>4</td></tr></table>`),
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := sh.EvalExported(c.src, vars)
			if err != nil {
				t.Fatalf("eval(%q) errored:\n%v", c.src, err)
			}
			if got := v.Display(); got != c.want {
				t.Errorf("eval(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}
