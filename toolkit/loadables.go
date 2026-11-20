package toolkit

import (
	"io"

	"github.com/iodesystems/mcpshell/runtime"
)

// GraphLoadable is the graph toolkit (Graph.*): in-memory, no I/O, so a
// reference loads it freely.
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

// WebLoadable is the web toolkit (Web.*, Html.*). Network access is
// operator tier: with enabled false, a reference to Web fails naming --web.
func WebLoadable(enabled bool) runtime.Loadable {
	l := runtime.Loadable{
		Name:       "web",
		Summary:    "HTTP requests, search, HTML parsing (Web.*, Html.*)",
		Guide:      "Web toolkit — Web.* (HTTP requests, search) and Html.* (CSS selector querying).\n  help(\"Web\") and help(\"Html\") list the commands.",
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

// BrowserLoadable is the browser toolkit (Browser.*). Operator tier: with
// enabled false, a reference to Browser fails naming --browser. Unload closes
// the browser.
func BrowserLoadable(enabled bool) runtime.Loadable {
	l := runtime.Loadable{
		Name:    "browser",
		Summary: "headless Chrome automation (Browser.*)",
		Guide:   "Browser toolkit — Browser.* (headless Chrome; needs a Chrome binary).\n  help(\"Browser\") lists the commands.",
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

// FilesLoadable is the file toolkit (Files.*) on a root set. Disk access is
// operator tier: with roots nil the toolkit is disabled and a reference to
// Files fails naming --files-dir.
func FilesLoadable(roots *FileRoots) runtime.Loadable {
	readOnly := roots != nil && roots.ReadOnly()
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
	if roots == nil {
		l.Disabled = "--files-dir"
		return l
	}
	l.Install = func(sh *runtime.Shell) (io.Closer, error) {
		InstallFileRoots(sh, roots)
		return closeFunc(func() error {
			sh.SetFileLoader(nil)
			ClearFileConfiner(sh)
			return nil
		}), nil
	}
	return l
}

type closeFunc func() error

func (f closeFunc) Close() error { return f() }
