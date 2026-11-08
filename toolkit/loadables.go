package toolkit

import (
	"io"
	"strings"

	"github.com/iodesystems/mcpshell/runtime"
)

// GraphLoadable is the graph toolkit as load("graph"): in-memory, no I/O, so
// an agent may load it freely.
func GraphLoadable() runtime.Loadable {
	names := globalNames(InstallGraph)
	return runtime.Loadable{
		Name:       "graph",
		Summary:    "property graph store (addNode, link, nodes, out, …) and algorithms on edge rows: paths, components, topo, critical path, max/min-cost flow, matching, MST (Graph.*)",
		Guide:      graphGuide,
		Namespaces: []string{"Graph"},
		Commands:   names,
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			c := graphCloser{installGraph(sh)}
			aliasNamespace(sh, "Graph", names)
			return c, nil
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
		Name:       runtime.FilesToolkit,
		Summary:    summary,
		Guide:      fileGuide(readOnly),
		Namespaces: []string{"Files"},
		Commands:   globalNames(func(sh *runtime.Shell) *runtime.Shell { return InstallFile(sh, ".", readOnly) }),
	}
	if root == "" {
		l.Disabled = "--files-dir"
		return l
	}
	l.Install = func(sh *runtime.Shell) (io.Closer, error) {
		InstallFile(sh, root, readOnly, readRoots...)
		aliasNamespace(sh, "Files", l.Commands)
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
	base := map[string]bool{}
	for _, n := range runtime.NewShell().Commands().Names() {
		base[n] = true
	}
	scratch := runtime.NewShell()
	install(scratch)
	var out []string
	for _, n := range scratch.Commands().Names() {
		if !strings.Contains(n, ".") && !base[n] {
			out = append(out, n)
		}
	}
	return out
}

// aliasNamespace registers ns.name for each bare command name, hidden from
// help. Models copy the Db.query pattern and write Files.read or
// Graph.addNode; the alias runs what they meant instead of costing a turn.
func aliasNamespace(sh *runtime.Shell, ns string, names []string) {
	for _, n := range names {
		def := sh.Commands().Get(n)
		if def == nil {
			continue
		}
		alias := *def
		alias.Namespace = ns
		alias.Hidden = true
		sh.Register(&alias)
	}
}
