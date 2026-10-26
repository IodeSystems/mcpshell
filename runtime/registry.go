package runtime

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// CommandDef is a single registered command.
type CommandDef struct {
	Name        string
	Namespace   string // "" = global
	Signature   string
	Description string
	Examples    []string
	Hidden      bool
	Category    string // optional group within the namespace, listed by help("Ns.category")
	Params      string // per-parameter detail, one line each; shown by help("name") only
	Fn          NativeFn
}

// MaxArgs is how many arguments the signature lists, or -1 when it does not
// say: an empty signature, or a rest parameter (...values).
func (c *CommandDef) MaxArgs() int {
	sig := strings.TrimSpace(c.Signature)
	if sig == "" || strings.Contains(sig, "...") {
		return -1
	}
	n, depth := 1, 0
	for i, r := range sig {
		switch r {
		case '(', '[', '{', '<':
			depth++
		case '>':
			if i > 0 && sig[i-1] == '=' { // the arrow in a function type
				continue
			}
			depth--
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

// CollapseNamespaceAt is the command count above which help() and the prompt
// list a namespace as one line instead of every command.
const CollapseNamespaceAt = 20

// Brief shortens a description to its first line and sentence, capped at 80 runes.
func Brief(desc string) string {
	s := desc
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// collapsedLine is the one-line stand-in for a namespace too large to list.
func collapsedLine(ns string, n int) string {
	return fmt.Sprintf("%s: %d commands — help(\"%s\")", ns, n, ns)
}

// DisplayName is "Ns.name" when namespaced, else "name".
func (c *CommandDef) DisplayName() string {
	if c.Namespace != "" {
		return c.Namespace + "." + c.Name
	}
	return c.Name
}

// CallName is how a script calls the command: Ns.name, or Ns["a-b"] when the
// name is not an identifier.
func (c *CommandDef) CallName() string {
	if c.Namespace != "" {
		return MemberRef(c.Namespace, c.Name)
	}
	return c.Name
}

// CommandRegistry holds every command, guide, and namespace available to a shell.
type CommandRegistry struct {
	commands   map[string]*CommandDef
	order      []string // command keys in registration order
	guides     map[string]string
	guideOrder []string
	namespaces map[string][]*CommandDef
	nsOrder    []string
}

// NewCommandRegistry builds an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands:   make(map[string]*CommandDef),
		guides:     make(map[string]string),
		namespaces: make(map[string][]*CommandDef),
	}
}

// Register adds a command. namespace "" registers a global command.
func (r *CommandRegistry) Register(def *CommandDef) {
	key := def.DisplayName()
	if _, exists := r.commands[key]; !exists {
		r.order = append(r.order, key)
	}
	r.commands[key] = def
	if def.Namespace != "" {
		if _, ok := r.namespaces[def.Namespace]; !ok {
			r.nsOrder = append(r.nsOrder, def.Namespace)
		}
		r.namespaces[def.Namespace] = append(r.namespaces[def.Namespace], def)
	}
}

// RegisterGuide adds a named help guide.
func (r *CommandRegistry) RegisterGuide(name, content string) {
	if _, ok := r.guides[name]; !ok {
		r.guideOrder = append(r.guideOrder, name)
	}
	r.guides[name] = content
}

// Get looks up a command by key ("name" or "Ns.name").
func (r *CommandRegistry) Get(name string) *CommandDef { return r.commands[name] }

// Names returns every command key in registration order.
func (r *CommandRegistry) Names() []string {
	return append([]string(nil), r.order...)
}

// NamespaceNames returns every registered namespace.
func (r *CommandRegistry) NamespaceNames() []string {
	return append([]string(nil), r.nsOrder...)
}

// NamespaceCommands returns the commands in a namespace, registration-ordered.
func (r *CommandRegistry) NamespaceCommands(namespace string) []*CommandDef {
	return r.namespaces[namespace]
}

// BuildNamespaceObject builds the TObject of TFunctions for a namespace, or nil.
func (r *CommandRegistry) BuildNamespaceObject(namespace string) *ObjectVal {
	cmds, ok := r.namespaces[namespace]
	if !ok {
		return nil
	}
	obj := NewObject()
	for _, c := range cmds {
		obj.Set(c.Name, &FuncVal{
			Name: c.DisplayName(),
			Body: &NativeBody{Fn: c.Fn, Cmd: c},
		})
	}
	return obj
}

func (r *CommandRegistry) visible() []*CommandDef {
	out := make([]*CommandDef, 0, len(r.order))
	for _, k := range r.order {
		if c := r.commands[k]; c != nil && !c.Hidden {
			out = append(out, c)
		}
	}
	return out
}

// Help renders help text. An empty search lists everything; otherwise it
// resolves a guide, an exact command, a namespace, or a fuzzy match.
func (r *CommandRegistry) Help(search string) string {
	if search == "" {
		return r.helpAll()
	}
	if g, ok := r.guides[search]; ok {
		return g
	}
	if exact := r.commands[search]; exact != nil {
		return detailedHelp(exact)
	}
	if out, ok := r.categoryHelp(search); ok {
		return out
	}
	if nsCmds, ok := r.namespaces[search]; ok {
		sorted := sortByName(nsCmds)
		var b strings.Builder
		b.WriteString(search + ":\n\n")
		for i, c := range sorted {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("  " + c.CallName() + "(" + c.Signature + ") — " + c.Description)
		}
		b.WriteString("\n\n  Use help(\"" + search + ".commandName\") for detailed help.")
		return b.String()
	}

	matches := r.search(search)

	if len(matches) == 0 {
		var all []string
		all = append(all, r.order...)
		all = append(all, r.nsOrder...)
		type scored struct {
			name string
			dist int
		}
		var near []scored
		for _, n := range all {
			if d := levenshtein(search, n); d <= 3 {
				near = append(near, scored{n, d})
			}
		}
		sort.SliceStable(near, func(i, j int) bool { return near[i].dist < near[j].dist })
		if len(near) > 3 {
			near = near[:3]
		}
		suggestion := ""
		if len(near) > 0 {
			var b strings.Builder
			b.WriteString("\n\n  Did you mean?\n")
			for i, s := range near {
				if i > 0 {
					b.WriteByte('\n')
				}
				b.WriteString("    help(\"" + s.name + "\")")
			}
			suggestion = b.String()
		}
		return "No commands matching '" + search + "'." + suggestion +
			"\n\n  Call help() to see all commands."
	}

	if len(matches) == 1 {
		return detailedHelp(matches[0])
	}
	var b strings.Builder
	b.WriteString("Commands matching '" + search + "', best first:\n\n")
	for i, c := range matches {
		if i == maxSearchResults {
			b.WriteString(fmt.Sprintf("  … %d more — use more words to narrow the search\n", len(matches)-i))
			break
		}
		b.WriteString("  " + c.CallName() + "(" + c.Signature + ") — " + Brief(c.Description) + "\n")
	}
	b.WriteString("\n  Use help(\"commandName\") for detailed help on a specific command.")
	return b.String()
}

// Find returns the commands help(search) would describe: all visible
// commands for "", then an exact command, a "ns.category", a namespace, or a
// ranked word search.
func (r *CommandRegistry) Find(search string) []*CommandDef {
	if search == "" {
		return r.visible()
	}
	if exact := r.commands[search]; exact != nil {
		return []*CommandDef{exact}
	}
	if i := strings.LastIndexByte(search, '.'); i >= 0 {
		var cmds []*CommandDef
		for _, c := range r.namespaces[search[:i]] {
			if c.Category == search[i+1:] && !c.Hidden {
				cmds = append(cmds, c)
			}
		}
		if len(cmds) > 0 {
			return sortByName(cmds)
		}
	}
	if nsCmds, ok := r.namespaces[search]; ok {
		return sortByName(nsCmds)
	}
	return r.search(search)
}

// maxSearchResults caps the commands one help(word) search lists.
const maxSearchResults = 25

// search returns the commands matching query, best first. The query is split
// into words on spaces, '-', '_' and '.'. A word found in a command's name
// counts more than a word found only in its description or signature, so
// "query" ranks a tool named for querying above one that mentions queries.
// When no command matches every word, commands matching the most words are
// returned.
func (r *CommandRegistry) search(query string) []*CommandDef {
	words := strings.FieldsFunc(strings.ToLower(query), func(c rune) bool {
		return c == ' ' || c == '-' || c == '_' || c == '.'
	})
	whole := NormalizeName(query)
	type scored struct {
		c          *CommandDef
		hits, name int
		exact      bool
	}
	var found []scored
	best := 0
	for _, k := range r.order {
		c := r.commands[k]
		if c.Hidden {
			continue
		}
		name := NormalizeName(c.DisplayName())
		text := strings.ToLower(c.Description + " " + c.Signature)
		sc := scored{c: c, exact: whole != "" && strings.Contains(name, whole)}
		for _, w := range words {
			switch {
			case strings.Contains(name, w):
				sc.hits++
				sc.name++
			case strings.Contains(text, w):
				sc.hits++
			}
		}
		if sc.hits == 0 && !sc.exact {
			continue
		}
		best = max(best, sc.hits)
		found = append(found, sc)
	}
	var out []scored
	for _, sc := range found {
		if sc.exact || sc.hits == best {
			out = append(out, sc)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.exact != b.exact {
			return a.exact
		}
		if a.name != b.name {
			return a.name > b.name
		}
		return a.c.DisplayName() < b.c.DisplayName()
	})
	cmds := make([]*CommandDef, len(out))
	for i, sc := range out {
		cmds[i] = sc.c
	}
	return cmds
}

// categoryHelp lists the commands of "Ns.category", one brief line each.
func (r *CommandRegistry) categoryHelp(search string) (string, bool) {
	i := strings.LastIndexByte(search, '.')
	if i < 0 {
		return "", false
	}
	ns, cat := search[:i], search[i+1:]
	var cmds []*CommandDef
	for _, c := range r.namespaces[ns] {
		if c.Category == cat && !c.Hidden {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) == 0 {
		return "", false
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s (%d commands):\n\n", search, len(cmds)))
	for _, c := range sortByName(cmds) {
		b.WriteString("  " + c.CallName() + " — " + Brief(c.Description) + "\n")
	}
	b.WriteString("\n  Use help(\"" + ns + ".commandName\") for signature and details.")
	return b.String(), true
}

func (r *CommandRegistry) helpAll() string {
	visible := r.visible()
	if len(visible) == 0 {
		return "No commands available."
	}
	var b strings.Builder

	globals := sortByName(filterNamespace(visible, ""))
	for _, c := range globals {
		b.WriteString("  " + c.Name + "(" + c.Signature + ") — " + c.Description + "\n")
	}

	nsNames := namespacesOf(visible)
	for i, ns := range nsNames {
		if len(globals) > 0 || i > 0 {
			b.WriteByte('\n')
		}
		cmds := filterNamespace(visible, ns)
		if len(cmds) > CollapseNamespaceAt {
			b.WriteString("  " + collapsedLine(ns, len(cmds)) + "\n")
			continue
		}
		b.WriteString("  " + ns + ":\n")
		for _, c := range sortByName(cmds) {
			b.WriteString("    " + ns + "." + c.Name + "(" + c.Signature + ") — " + c.Description + "\n")
		}
	}

	if len(r.guides) > 0 {
		b.WriteByte('\n')
		b.WriteString("Guides:\n")
		names := append([]string(nil), r.guideOrder...)
		sort.Strings(names)
		for _, n := range names {
			b.WriteString("  help(\"" + n + "\") — " + n + " usage patterns\n")
		}
	}
	return strings.TrimRight(b.String(), "\n ")
}

// Prompt renders the compact per-command listing for system prompts.
func (r *CommandRegistry) Prompt() string {
	visible := r.visible()
	if len(visible) == 0 {
		return "No commands available."
	}
	nsCount := make(map[string]int)
	for _, c := range visible {
		nsCount[c.Namespace]++
	}
	var sorted []*CommandDef
	for _, c := range visible {
		if c.Namespace == "" || nsCount[c.Namespace] <= CollapseNamespaceAt {
			sorted = append(sorted, c)
		}
	}
	var collapsed []string
	for _, ns := range namespacesOf(visible) {
		if nsCount[ns] > CollapseNamespaceAt {
			collapsed = append(collapsed, "  "+collapsedLine(ns, nsCount[ns]))
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].DisplayName() < sorted[j].DisplayName()
	})
	var lines []string
	for _, c := range sorted {
		sig := promptInputPrefix1.ReplaceAllString(c.Signature, "")
		sig = promptInputPrefix2.ReplaceAllString(sig, "")
		lines = append(lines, "  "+c.CallName()+"("+sig+") — "+c.Description)
	}
	lines = append(lines, collapsed...)
	return strings.Join(lines, "\n")
}

// CompactPrompt renders the minimal name-only listing for prompt-weight-sensitive use.
func (r *CommandRegistry) CompactPrompt() string {
	visible := r.visible()
	if len(visible) == 0 {
		return "No commands available."
	}
	var b strings.Builder

	globals := sortByName(filterNamespace(visible, ""))
	if len(globals) > 0 {
		names := make([]string, len(globals))
		for i, c := range globals {
			names[i] = c.Name
		}
		b.WriteString(strings.Join(names, ", ") + "\n")
	}

	for _, ns := range namespacesOf(visible) {
		cmds := sortByName(filterNamespace(visible, ns))
		if len(cmds) > CollapseNamespaceAt {
			b.WriteString(collapsedLine(ns, len(cmds)) + "\n")
			continue
		}
		names := make([]string, len(cmds))
		for i, c := range cmds {
			names[i] = c.Name
		}
		b.WriteString(ns + ": " + strings.Join(names, ", ") + "\n")
	}

	if len(r.guides) > 0 {
		guides := append([]string(nil), r.guideOrder...)
		sort.Strings(guides)
		for i := range guides {
			guides[i] = "help(\"" + guides[i] + "\")"
		}
		b.WriteString("\nGuides: " + strings.Join(guides, ", ") + "\n")
	}

	b.WriteString("\nhelp(\"name\") for signatures and examples. help() lists all.")
	return strings.TrimRight(b.String(), "\n ")
}

var (
	promptInputPrefix1 = regexp.MustCompile(`^input:\s*\w+(\|\w+)*,\s*`)
	promptInputPrefix2 = regexp.MustCompile(`^input:\s*\w+(\|\w+)*$`)
)

func detailedHelp(c *CommandDef) string {
	var b strings.Builder
	b.WriteString(c.CallName() + "(" + c.Signature + ")\n\n")
	b.WriteString("  " + c.Description + "\n")
	if c.Params != "" {
		b.WriteString("\n  Parameters:\n" + c.Params + "\n")
	}
	if len(c.Examples) > 0 {
		b.WriteString("\n  Examples:\n")
		for _, ex := range c.Examples {
			b.WriteString("    " + ex + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n ")
}

func filterNamespace(cmds []*CommandDef, ns string) []*CommandDef {
	out := make([]*CommandDef, 0, len(cmds))
	for _, c := range cmds {
		if c.Namespace == ns {
			out = append(out, c)
		}
	}
	return out
}

func sortByName(cmds []*CommandDef) []*CommandDef {
	out := append([]*CommandDef(nil), cmds...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// namespacesOf returns the sorted unique namespaces present in cmds.
func namespacesOf(cmds []*CommandDef) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, c := range cmds {
		if c.Namespace == "" {
			continue
		}
		if _, ok := seen[c.Namespace]; !ok {
			seen[c.Namespace] = struct{}{}
			out = append(out, c.Namespace)
		}
	}
	sort.Strings(out)
	return out
}
