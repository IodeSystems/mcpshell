package toolkit

import (
	"io"

	"github.com/iodesystems/mcpshell/runtime"
)

// GraphLoadable is the graph toolkit as load("graph"): in-memory, no I/O, so
// an agent may load it freely.
func GraphLoadable() runtime.Loadable {
	return runtime.Loadable{
		Name:       "graph",
		Summary:    "graph store (Graph.addNode, link, out, …) and algorithms on edge rows: paths, components, topo order, critical path (CPM), max/min-cost flow, matching, MST (Graph.*)",
		Guide:      graphGuide,
		Namespaces: []string{"Graph"},
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
	summary := "read, search, write, edit files under the operator's directory (Files.read, ls, glob, grep, write, edit, …)"
	if readOnly {
		summary = "read and search files under the operator's directory (Files.read, ls, glob, grep, …)"
	}
	l := runtime.Loadable{
		Name:       runtime.FilesToolkit,
		Summary:    summary,
		Guide:      fileGuide(readOnly),
		Namespaces: []string{"Files"},
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
