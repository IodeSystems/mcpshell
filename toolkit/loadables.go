package toolkit

import (
	"io"
	"strings"

	"github.com/iodesystems/mcpshell/runtime"
)

// GraphLoadable is the graph toolkit as load("graph"): in-memory, no I/O, so
// an agent may load it freely.
func GraphLoadable() runtime.Loadable {
	return runtime.Loadable{
		Name:     "graph",
		Summary:  "in-memory property graph (addNode, link, nodes, out, in, …)",
		Guide:    graphGuide,
		Commands: globalNames(InstallGraph),
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			return graphCloser{installGraph(sh)}, nil
		},
	}
}

// WebLoadable is the web toolkit (Web.*, Html.*) as load("web"). Network
// access is operator tier: with enabled false, load("web") fails naming --web.
func WebLoadable(enabled bool) runtime.Loadable {
	l := runtime.Loadable{
		Name:       "web",
		Summary:    "HTTP requests, search, HTML parsing (Web.*, Html.*)",
		Guide:      "Web toolkit — Web.* (HTTP requests, search) and Html.* (CSS selector querying).\n  After load: help(\"Web\"), help(\"Html\").",
		Namespaces: []string{"Web", "Html"},
	}
	if !enabled {
		l.Disabled = "--web"
		return l
	}
	l.Install = func(sh *runtime.Shell) (io.Closer, error) {
		InstallWeb(sh)
		return nil, nil
	}
	return l
}

// BrowserLoadable is the browser toolkit (Browser.*) as load("browser").
// Operator tier: with enabled false, load("browser") fails naming --browser.
// Unload closes the browser.
func BrowserLoadable(enabled bool) runtime.Loadable {
	l := runtime.Loadable{
		Name:    "browser",
		Summary: "headless Chrome automation (Browser.*)",
		Guide:   "Browser toolkit — Browser.* (headless Chrome; needs a Chrome binary).\n  After load: help(\"Browser\").",
	}
	if !enabled {
		l.Disabled = "--browser"
		return l
	}
	l.Install = func(sh *runtime.Shell) (io.Closer, error) {
		return InstallBrowser(sh, true), nil
	}
	return l
}

// FilesLoadable is the file toolkit as load("files"), rooted at root. Disk
// access is operator tier: with root "" the toolkit is disabled and
// load("files") fails naming --files-dir. restore, if not nil, runs on
// unload, after the toolkit's commands are gone (the caller may put a
// narrower reader back there).
func FilesLoadable(root string, readOnly bool, readRoots []string, restore func(sh *runtime.Shell)) runtime.Loadable {
	summary := "read, search, write, edit files under the operator's directory (read, ls, glob, grep, write, edit, …)"
	if readOnly {
		summary = "read and search files under the operator's directory (read, ls, glob, grep, …)"
	}
	l := runtime.Loadable{
		Name:     runtime.FilesToolkit,
		Summary:  summary,
		Guide:    fileGuide(readOnly),
		Commands: globalNames(func(sh *runtime.Shell) *runtime.Shell { return InstallFile(sh, ".", false) }),
	}
	if root == "" {
		l.Disabled = "--files-dir"
		return l
	}
	l.Install = func(sh *runtime.Shell) (io.Closer, error) {
		InstallFile(sh, root, readOnly, readRoots...)
		return closeFunc(func() error {
			sh.SetFileLoader(nil)
			if restore != nil {
				restore(sh)
			}
			return nil
		}), nil
	}
	return l
}

type closeFunc func() error

func (f closeFunc) Close() error { return f() }

// globalNames lists the non-namespaced commands install registers, by
// installing into a scratch shell.
func globalNames(install func(*runtime.Shell) *runtime.Shell) []string {
	scratch := runtime.NewShell()
	install(scratch)
	var out []string
	for _, n := range scratch.Commands().Names() {
		if !strings.Contains(n, ".") {
			out = append(out, n)
		}
	}
	return out
}
