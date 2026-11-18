package runtime

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Loadable is a toolkit that is installed on demand: the first reference to
// one of its namespaces (Graph, Db) loads it. A toolkit that reaches outside
// the process (files, network) is operator tier: the operator flag registers
// it enabled, and without the flag it is registered with Disabled set, so a
// reference names the flag instead of installing it.
type Loadable struct {
	Name    string // bare: no "/" and no "."
	Summary string // one line, shown in the Available list
	Guide   string // help("name") text, shown before and after load
	// Namespaces are the globals Install defines; a reference to one loads
	// the toolkit. Default: Name with the first letter upper-cased.
	Namespaces []string
	// Disabled, if not empty, is the operator flag that enables this toolkit
	// (for example "--web"). The toolkit is then not advertised, and a
	// reference fails naming the flag.
	Disabled string
	// Install registers the toolkit's commands on sh. The closer, if not nil,
	// is closed by unload.
	Install func(sh *Shell) (io.Closer, error)
}

type loadable struct {
	def    Loadable
	loaded *loadState // nil when not loaded
}

// loadState is what one load added, so unload can take it away.
type loadState struct {
	alive    *atomic.Bool
	cmds     []*CommandDef
	guides   []string
	closer   io.Closer
	summary  string // the usage block: Ns.help()
	reminder string // what a second Load returns: one line
}

// loadHook collects what Shell.Register and RegisterGuide add while Install runs.
type loadHook struct {
	name       string
	alive      *atomic.Bool
	cmds       []*CommandDef
	guides     []string
	namespaces []string // defined as globals after Install
}

func (d *Loadable) namespaces() []string {
	if len(d.Namespaces) > 0 {
		return d.Namespaces
	}
	if d.Name == "" {
		return nil
	}
	return []string{strings.ToUpper(d.Name[:1]) + d.Name[1:]}
}

// LoadableNames returns the loadable names an operator has not denied, sorted.
func (r *CommandRegistry) LoadableNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadableNames(func(*loadable) bool { return true })
}

func (r *CommandRegistry) loadableNames(keep func(*loadable) bool) []string {
	var out []string
	for n, l := range r.loadables {
		if !r.denied[n] && l.def.Disabled == "" && keep(l) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// availableLine is the one line that lists loadable toolkits, or "". With
// all false it lists only the toolkits not loaded; with all true it lists
// every allowed one, so its text does not change with load state.
func (r *CommandRegistry) availableLine(all bool) string {
	names := r.loadableNames(func(l *loadable) bool { return all || l.loaded == nil })
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, len(names))
	for i, n := range names {
		d := r.loadables[n].def
		parts[i] = strings.Join(d.namespaces(), ", ") + " — " + d.Summary
	}
	return "Toolkits (load on first use; Ns.help() describes one): " + strings.Join(parts, " · ")
}

// AvailableLine is the Available line for system prompts: every allowed
// loadable, loaded or not.
func (r *CommandRegistry) AvailableLine() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.availableLine(true)
}

// ToolkitsLine names every allowed loadable on one short line, for the MCP
// eval tool description, which clients send on every request: the summaries
// are in help() and help("name").
func (r *CommandRegistry) ToolkitsLine() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var nss []string
	for _, n := range r.loadableNames(func(*loadable) bool { return true }) {
		d := r.loadables[n].def
		nss = append(nss, d.namespaces()...)
	}
	if len(nss) == 0 {
		return ""
	}
	sort.Strings(nss)
	return "Toolkits (Ns.help() describes one): " + strings.Join(nss, ", ")
}

// NotLoadedHint returns the error text for a global that belongs to a
// loadable toolkit that is not loaded, or "".
func (r *CommandRegistry) NotLoadedHint(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ns, ok := movedNames[name]; ok {
		return r.movedHint(name, ns)
	}
	for n, l := range r.loadables {
		if l.loaded != nil || r.denied[n] {
			continue
		}
		if !l.def.owns(name) {
			continue
		}
		if l.def.Disabled != "" {
			return fmt.Sprintf("'%s' is part of the %q toolkit, which the operator did not enable (%s)", name, n, l.def.Disabled)
		}
		return fmt.Sprintf("'%s' is part of the %q toolkit, which is not loaded", name, n)
	}
	// A wrong-case guess at a toolkit's namespace: files.ls for Files.ls,
	// db.query for Db.query.
	for n, l := range r.loadables {
		if r.denied[n] || l.def.Disabled != "" {
			continue
		}
		nss := l.def.namespaces()
		match := strings.EqualFold(name, n)
		for _, ns := range nss {
			match = match || strings.EqualFold(name, ns)
		}
		if !match {
			continue
		}
		return fmt.Sprintf("'%s' is not defined. The %q toolkit's namespace is %s: %s.<command>. %s.help() describes it", name, n, strings.Join(nss, ", "), nss[0], nss[0])
	}
	return ""
}

// movedNames are the graph and file commands that were bare globals before
// they became namespace-only (docs/design.md "Loadable toolkits"). A call of
// the old name says where it went; the name itself is free for variables.
var movedNames = map[string]string{
	"root": "Graph", "node": "Graph", "nodes": "Graph", "edges": "Graph", "addNode": "Graph",
	"link": "Graph", "unlink": "Graph", "removeNode": "Graph", "setProps": "Graph",
	"out": "Graph", "inbound": "Graph", "both": "Graph", "outE": "Graph", "inE": "Graph",
	"glob": "Files", "read": "Files", "readLines": "Files", "exists": "Files", "ls": "Files",
	"tree": "Files", "grep": "Files", "write": "Files", "append": "Files", "edit": "Files",
	"deletePath": "Files", "mv": "Files", "mkdir": "Files",
}

// movedHint is the error for an old bare name; "" when the toolkit that has
// the namespace is denied (it is not advertised). r.mu is held.
func (r *CommandRegistry) movedHint(name, ns string) string {
	msg := fmt.Sprintf("'%s' is not defined — use %s.%s", name, ns, name)
	if _, ok := r.namespaces[ns]; ok {
		return msg
	}
	for n, l := range r.loadables {
		if !l.def.owns(ns) {
			continue
		}
		switch {
		case r.denied[n]:
			return ""
		case l.def.Disabled != "":
			return msg + fmt.Sprintf(" (the %q toolkit, which the operator did not enable: %s)", n, l.def.Disabled)
		}
	}
	return msg
}

func (d *Loadable) owns(name string) bool {
	for _, ns := range d.namespaces() {
		if ns == name {
			return true
		}
	}
	return false
}

// loadedNames returns the loaded loadables, sorted.
func (r *CommandRegistry) loadedNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for n, l := range r.loadables {
		if l.loaded != nil {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// LoadableOwning returns the loadable whose namespaces include name exactly, loaded or not, or "". Denied and disabled loadables are
// skipped: they cannot be loaded.
func (r *CommandRegistry) LoadableOwning(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for n, l := range r.loadables {
		if !r.denied[n] && l.def.Disabled == "" && l.def.owns(name) {
			return n
		}
	}
	return ""
}

// remove drops commands, their emptied namespaces and guides. It returns the
// namespaces it emptied.
func (r *CommandRegistry) remove(cmds []*CommandDef, guides []string) (emptied []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gone := map[*CommandDef]bool{}
	for _, c := range cmds {
		gone[c] = true
		key := c.DisplayName()
		if r.commands[key] == c {
			delete(r.commands, key)
			r.order = removeString(r.order, key)
		}
		if c.Namespace == "" {
			continue
		}
		var list []*CommandDef
		for _, o := range r.namespaces[c.Namespace] {
			if !gone[o] {
				list = append(list, o)
			}
		}
		if len(list) == 0 {
			if _, ok := r.namespaces[c.Namespace]; ok {
				emptied = append(emptied, c.Namespace)
			}
			delete(r.namespaces, c.Namespace)
			r.nsOrder = removeString(r.nsOrder, c.Namespace)
		} else {
			r.namespaces[c.Namespace] = list
		}
	}
	for _, g := range guides {
		delete(r.guides, g)
		r.guideOrder = removeString(r.guideOrder, g)
	}
	return emptied
}

func removeString(list []string, s string) []string {
	var out []string
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// loaders is the shell's load state. loadMu serializes load and unload;
// hook is set only while an Install runs.
type loaders struct {
	loadMu     sync.Mutex
	fileMu     sync.Mutex // guards fileLoader; separate from loadMu because Install and unload set it while loadMu is held
	hook       *loadHook
	fileLoader func(path string) Value
}

// AddLoadable registers a toolkit; a reference to its namespace installs it.
func (t *Shell) AddLoadable(l Loadable) *Shell {
	if l.Name == "" || strings.ContainsAny(l.Name, "/.") || (l.Install == nil && l.Disabled == "") {
		panic("AddLoadable: name must be bare (no / or .) and Install set: " + l.Name)
	}
	r := t.commands
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadables[l.Name] = &loadable{def: l}
	return t
}

// DenyLoadables hides the named loadables and makes a reference refuse them. The
// order against AddLoadable does not matter.
func (t *Shell) DenyLoadables(names ...string) *Shell {
	r := t.commands
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range names {
		r.denied[strings.TrimSpace(n)] = true
	}
	return t
}

// SetFileLoader sets how load("path") evaluates a file; the file toolkit
// sets it, confined to its root. Without it the path form of load errors.
func (t *Shell) SetFileLoader(fn func(path string) Value) {
	t.fileMu.Lock()
	defer t.fileMu.Unlock()
	t.fileLoader = fn
}

func (t *Shell) getFileLoader() func(path string) Value {
	t.fileMu.Lock()
	defer t.fileMu.Unlock()
	return t.fileLoader
}

// FilesToolkit is the loadable that provides the file loader.
const FilesToolkit = "files"

// EnsureFiles loads the file toolkit when the operator permitted it and it is
// not loaded. It returns "" when the file toolkit can be used, or the reason
// it cannot: the operator flag that enables it, or "" for disabled with no
// flag known. Commands that read files through the file toolkit (load of a
// path, Db.loadCsv) call it, so an agent does not pay a turn for a Files reference.
func (t *Shell) EnsureFiles() (enabled bool, flag string) {
	if t.getFileLoader() != nil {
		return true, ""
	}
	r := t.commands
	r.mu.RLock()
	l := r.loadables[FilesToolkit]
	denied := r.denied[FilesToolkit]
	r.mu.RUnlock()
	if l == nil || denied {
		return false, ""
	}
	if l.def.Disabled != "" {
		return false, l.def.Disabled
	}
	if _, err := t.Load(FilesToolkit); err != nil {
		panic(Runtime(err.Error()))
	}
	return t.getFileLoader() != nil, ""
}

// LoadFile evaluates a .mcpshell file through the file toolkit. If the
// operator permitted that toolkit but it is not loaded, LoadFile loads it.
func (t *Shell) LoadFile(path string) Value {
	ok, flag := t.EnsureFiles()
	if !ok {
		msg := "load: '" + path + "' is not a loadable toolkit, and no file toolkit is installed to load it as a file"
		if flag != "" {
			msg = "load: '" + path + "' is a file path, and the operator did not enable the file toolkit (" + flag + ")"
		}
		if names := t.commands.LoadableNames(); len(names) > 0 {
			msg += ". Toolkits: " + strings.Join(names, ", ")
		}
		panic(Runtime(msg))
	}
	return t.getFileLoader()(path)
}

// NotPermittedError is a load the operator did not permit: a toolkit without
// its flag, or one --no-load denied.
type NotPermittedError struct{ msg string }

func (e *NotPermittedError) Error() string { return e.msg }

// Load installs a loadable and returns its usage. Loading again does
// nothing and returns one line. Scripts do not call it: a reference to the
// toolkit's namespace does (autoLoad).
func (t *Shell) Load(name string) (string, error) {
	t.loadMu.Lock()
	defer t.loadMu.Unlock()
	r := t.commands
	r.mu.RLock()
	l, denied := r.loadables[name], r.denied[name]
	var done *loadState
	if l != nil {
		done = l.loaded
	}
	r.mu.RUnlock()
	switch {
	case l == nil:
		return "", fmt.Errorf("load: no toolkit named %q", name)
	case l.def.Disabled != "":
		return "", &NotPermittedError{fmt.Sprintf("toolkit %q is not enabled: the operator did not pass %s", name, l.def.Disabled)}
	case denied:
		return "", &NotPermittedError{fmt.Sprintf("toolkit %q is disabled by the operator", name)}
	case done != nil:
		return done.reminder, nil
	}

	hook := &loadHook{name: name, alive: &atomic.Bool{}}
	hook.alive.Store(true)
	t.hook = hook
	var closer io.Closer
	var err error
	func() {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("%v", p)
			}
		}()
		closer, err = l.def.Install(t)
		if err == nil {
			for _, ns := range hook.namespaces {
				t.registerUnload(ns, name)
			}
		}
	}()
	t.hook = nil
	if err != nil {
		hook.alive.Store(false)
		t.dropRegistered(hook.cmds, hook.guides)
		if closer != nil {
			_ = closer.Close()
		}
		return "", fmt.Errorf("toolkit %q failed to load: %w", name, err)
	}
	for _, ns := range hook.namespaces {
		if obj := r.BuildNamespaceObject(ns); obj != nil {
			t.globals.DefineBuiltin(ns, obj)
		}
	}

	nss := l.def.namespaces()
	summary := loadUsage(name, nss, hook.cmds, l.def.Guide)
	reminder := fmt.Sprintf("%q is already loaded. %s.help() describes it.", name, nss[0])
	st := &loadState{alive: hook.alive, cmds: hook.cmds, guides: hook.guides, closer: closer, summary: summary, reminder: reminder}
	r.mu.Lock()
	l.loaded = st
	r.mu.Unlock()
	return summary, nil
}

// registerUnload adds the hidden ns.unload() of a loading toolkit.
func (t *Shell) registerUnload(ns, name string) {
	t.Register(&CommandDef{Namespace: ns, Name: "unload", Hidden: true,
		Description: fmt.Sprintf("unloads the %q toolkit and frees what it holds; the next reference to %s loads it again", name, ns),
		Fn: func([]Value) Value {
			msg, err := t.Unload(name)
			if err != nil {
				panic(Runtime(err.Error()))
			}
			return Str(msg)
		}})
}

// autoLoad loads the toolkit that has the namespace ns, for a reference to
// ns that is not defined. owned is false when no loadable has ns; a toolkit
// that is loaded already is owned with no error.
func (t *Shell) autoLoad(ns string) (owned bool, err error) {
	r := t.commands
	r.mu.RLock()
	name := ""
	for n, l := range r.loadables {
		if l.def.owns(ns) {
			name = n
			break
		}
	}
	r.mu.RUnlock()
	if name == "" {
		return false, nil
	}
	_, err = t.Load(name)
	return true, err
}

// ToolkitUsage is what ns.help() gives for a loaded toolkit's namespace: its
// commands with signatures, then its guide. "" when no loaded toolkit has ns.
func (r *CommandRegistry) ToolkitUsage(ns string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.loadables {
		if l.loaded != nil && l.def.owns(ns) {
			return l.loaded.summary
		}
	}
	return ""
}

// ToolkitNamespace returns the first namespace of the loadable name, or "".
func (r *CommandRegistry) ToolkitNamespace(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if l := r.loadables[name]; l != nil {
		return l.def.namespaces()[0]
	}
	return ""
}

// Unload removes a loaded toolkit's commands and closes its closer. Held
// references to its commands fail with a message instead of running.
func (t *Shell) Unload(name string) (string, error) {
	t.loadMu.Lock()
	defer t.loadMu.Unlock()
	r := t.commands
	r.mu.Lock()
	l := r.loadables[name]
	var st *loadState
	if l != nil {
		st, l.loaded = l.loaded, nil
	}
	r.mu.Unlock()
	if l == nil {
		return "", fmt.Errorf("unload: no loadable toolkit named %q", name)
	}
	if st == nil {
		return fmt.Sprintf("%q is not loaded.", name), nil
	}
	st.alive.Store(false)
	t.dropRegistered(st.cmds, st.guides)
	if st.closer != nil {
		if err := st.closer.Close(); err != nil {
			return "", fmt.Errorf("unload: %q: close: %w", name, err)
		}
	}
	return fmt.Sprintf("Unloaded %q.", name), nil
}

func (t *Shell) dropRegistered(cmds []*CommandDef, guides []string) {
	emptied := t.commands.remove(cmds, guides)
	if len(emptied) > 0 {
		names := make(map[string]struct{}, len(emptied))
		for _, n := range emptied {
			names[n] = struct{}{}
		}
		t.globals.Remove(names)
	}
}

// guard makes a command registered during Install fail after unload, and
// records it for unload.
func (h *loadHook) guard(def *CommandDef) {
	orig, alive, name := def.Fn, h.alive, h.name
	def.Fn = func(args []Value) Value {
		if !alive.Load() {
			panic(Runtime(fmt.Sprintf("%s: toolkit %q was unloaded: a new reference to its namespace loads it again; use the new commands", def.DisplayName(), name)))
		}
		defer func() {
			if p := recover(); p != nil {
				if se, ok := p.(*ShellError); ok && !se.Internal() && !strings.Contains(se.Message, "\n  Usage: ") {
					se.Message += usageLine(def, !strings.Contains(se.Message, "("+def.Signature+")"))
				}
				panic(p)
			}
		}()
		return orig(args)
	}
	h.cmds = append(h.cmds, def)
}

// usageLine is what an error from a loaded toolkit's command ends with: the
// right call and one example. A model learns a toolkit from its errors; it
// rarely calls help().
func usageLine(def *CommandDef, withSignature bool) string {
	var b strings.Builder
	if withSignature {
		b.WriteString("\n\n  Usage: " + def.CallName() + "(" + def.Signature + ")")
	}
	if len(def.Examples) > 0 {
		if !withSignature {
			b.WriteString("\n")
		}
		b.WriteString("\n  Usage: e.g. " + def.Examples[0])
	}
	return b.String()
}

// loadUsage is what Ns.help() gives for a loaded toolkit: one line per
// command with its signature, then the toolkit's guide (examples, notes).
func loadUsage(name string, nss []string, cmds []*CommandDef, guide string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The %q toolkit (%s).", name, strings.Join(nss, ", "))
	if len(cmds) > 0 {
		b.WriteString(" Commands:\n")
		for _, c := range cmds {
			if c.Hidden {
				continue
			}
			b.WriteString("  " + c.CallName() + "(" + c.Signature + ") — " + Brief(c.Description) + "\n")
		}
	}
	if g := strings.TrimSpace(guide); g != "" {
		b.WriteString("\n" + g + "\n")
	}
	fmt.Fprintf(&b, "\n%s.unload() unloads it; the next reference loads it again.", nss[0])
	return b.String()
}
