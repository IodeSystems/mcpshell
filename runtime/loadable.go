package runtime

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Loadable is a toolkit that load("name") installs on demand. A toolkit that
// reaches outside the process (files, network) is operator tier: the operator
// flag registers it enabled, and without the flag it is registered with
// Disabled set, so load() names the flag instead of installing it.
type Loadable struct {
	Name    string // bare: no "/" and no "."
	Summary string // one line, shown in the Available list
	Guide   string // help("name") text, shown before and after load
	// Namespaces are the globals Install defines. Calling one before load
	// fails with "call load first". Default: Name with the first letter upper-cased.
	Namespaces []string
	// Commands are the global (non-namespaced) command names Install defines.
	// Calling one before load gives the same "call load first" hint.
	Commands []string
	// Disabled, if not empty, is the operator flag that enables this toolkit
	// (for example "--web"). The toolkit is then not advertised, and load
	// fails naming the flag.
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
	summary  string // what the first load returned: the usage block
	reminder string // what a second load returns: one line
}

// loadHook collects what Shell.Register and RegisterGuide add while Install runs.
type loadHook struct {
	name   string
	alive  *atomic.Bool
	cmds   []*CommandDef
	guides []string
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
		parts[i] = "load(\"" + n + "\") — " + r.loadables[n].def.Summary
	}
	return "Available (load first): " + strings.Join(parts, " · ")
}

// AvailableLine is the Available line for system prompts: every allowed
// loadable, loaded or not.
func (r *CommandRegistry) AvailableLine() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.availableLine(true)
}

// NotLoadedHint returns the error text for a global that belongs to a
// loadable toolkit that is not loaded, or "".
func (r *CommandRegistry) NotLoadedHint(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
		return fmt.Sprintf("'%s' is part of the %q toolkit, which is not loaded: call load(%q) first. help(%q) shows its usage", name, n, n, n)
	}
	// A wrong-case guess at a toolkit's namespace: files.ls after
	// load("files"), db.query for Db.query. Models copy the Db.query
	// pattern to toolkits whose commands are bare.
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
		var msg string
		if len(l.def.Commands) > 0 {
			shown := l.def.Commands
			if len(shown) > 8 {
				shown = shown[:8]
			}
			calls := make([]string, len(shown))
			for i, c := range shown {
				calls[i] = c + "()"
			}
			msg = fmt.Sprintf("'%s' is not defined. The %q toolkit's commands are bare: %s, … (%s.%s also works)", name, n, strings.Join(calls, ", "), nss[0], shown[0])
		} else {
			msg = fmt.Sprintf("'%s' is not defined. The %q toolkit's namespace is %s: %s.<command>", name, n, strings.Join(nss, ", "), nss[0])
		}
		if l.loaded == nil {
			msg += fmt.Sprintf(". Call load(%q) first", n)
		}
		return msg + fmt.Sprintf(". help(%q) shows its usage", n)
	}
	return ""
}

func (d *Loadable) owns(name string) bool {
	for _, ns := range d.namespaces() {
		if ns == name {
			return true
		}
	}
	for _, c := range d.Commands {
		if c == name {
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

// LoadableOwning returns the loadable whose namespaces or commands include
// name exactly, loaded or not, or "". Denied and disabled loadables are
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

// AddLoadable registers a toolkit for load(). It is not installed.
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

// DenyLoadables hides the named loadables and makes load() refuse them. The
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

// IsLoadableName reports whether name is a registered loadable, denied or not.
func (t *Shell) IsLoadableName(name string) bool {
	r := t.commands
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadables[name] != nil
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
// path, Db.loadCsv) call it, so an agent does not pay a turn for load("files").
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

// Load installs a loadable and returns its summary. Loading again does
// nothing and returns the same summary.
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
		return "", fmt.Errorf("load: toolkit %q is not enabled: the operator did not pass %s", name, l.def.Disabled)
	case denied:
		return "", fmt.Errorf("load: toolkit %q is disabled by the operator", name)
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
			t.hook = nil
			if p := recover(); p != nil {
				err = fmt.Errorf("%v", p)
			}
		}()
		closer, err = l.def.Install(t)
	}()
	if err != nil {
		hook.alive.Store(false)
		t.dropRegistered(hook.cmds, hook.guides)
		if closer != nil {
			_ = closer.Close()
		}
		return "", fmt.Errorf("load: %q: %w", name, err)
	}

	names := make([]string, len(hook.cmds))
	for i, c := range hook.cmds {
		names[i] = c.CallName()
	}
	summary := loadUsage(name, hook.cmds, l.def.Guide)
	reminder := fmt.Sprintf("%q is already loaded. help(%q) repeats its usage.", name, name)
	if len(names) > 0 {
		reminder = fmt.Sprintf("%q is already loaded: %s. help(%q) repeats its usage.", name, strings.Join(names, ", "), name)
	}
	st := &loadState{alive: hook.alive, cmds: hook.cmds, guides: hook.guides, closer: closer, summary: summary, reminder: reminder}
	r.mu.Lock()
	l.loaded = st
	r.mu.Unlock()
	return summary, nil
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
			panic(Runtime(fmt.Sprintf("%s: toolkit %q was unloaded: call load(%q) and use the new commands", def.DisplayName(), name, name)))
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
// right call and one example. A model learns a toolkit from its load result
// and its errors; it rarely calls help().
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

// loadUsage is what the first load of a toolkit returns: one line per
// command with its signature, then the toolkit's guide (examples, notes).
// It costs tokens once, in the result of the call that needs them, instead
// of in every prompt; models in benchmark runs did not call help().
func loadUsage(name string, cmds []*CommandDef, guide string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Loaded %q.", name)
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
	fmt.Fprintf(&b, "\nhelp(%q) shows this again.", name)
	return b.String()
}
