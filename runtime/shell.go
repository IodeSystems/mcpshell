package runtime

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Shell is the top-level facade: it owns a command registry, global scope,
// and execution limits, and evaluates source against them.
type Shell struct {
	commands *CommandRegistry
	limits   *ExecutionLimits
	globals  *Environment
	diag     Diagnostics // from the last eval

	resultSeq int      // last $rN handed out
	kept      []string // $rN names still bound, oldest first

	loaders
}

// KeptResults is how many truncated results the shell keeps as $r1, $r2, …
const KeptResults = 5

// TruncationNote keeps a value whose display was cut at shown of total bytes
// under the next $rN name, and returns the note to append after the shown
// part: the name, the value's inferred type, and how to query it. The next
// eval reads the whole value from $rN instead of running the eval again —
// which, with upstream tools, would repeat every call.
func (t *Shell) TruncationNote(v Value, shown, total int) string {
	t.resultSeq++
	name := "$r" + strconv.Itoa(t.resultSeq)
	t.globals.DefineOrSet(name, v)
	t.kept = append(t.kept, name)
	if len(t.kept) > KeptResults {
		t.globals.Remove(map[string]struct{}{t.kept[0]: {}})
		t.kept = t.kept[1:]
	}
	size := ""
	var query string
	switch x := v.(type) {
	case *ArrayVal:
		size = ", length " + strconv.Itoa(len(x.Elements))
		query = name + " |> limit(5)   " + name + " |> filter(x => …) |> len()"
	case *ObjectVal:
		size = ", " + strconv.Itoa(x.Len()) + " keys"
		query = "keys(" + name + ")"
		if keys := x.Keys(); len(keys) > 0 {
			query += "   " + MemberRef(name, keys[0])
		}
	case *StringVal:
		size = ", " + strconv.Itoa(len([]rune(x.V))) + " characters"
		query = name + " |> substring(" + strconv.Itoa(shown) + ", " + strconv.Itoa(2*shown) + ")   " +
			name + ` |> lines() |> filter(l => l.includes("…"))`
	default:
		query = name
	}
	return fmt.Sprintf("\n\n... OUTPUT TRUNCATED at %d of %d bytes. The full value is %s, kept for the next evals (the last %d are kept):\n"+
		"  %s: %s%s\n"+
		"  Query it instead of re-running: %s",
		shown, total, name, KeptResults, name, InferType(v), size, query)
}

// KeptResultNames returns the $rN names still kept, oldest first.
func (t *Shell) KeptResultNames() []string { return slices.Clone(t.kept) }

// Forget drops a kept result ($rN) or a session global (a top-level
// declaration) and returns the bytes it held, by estimate. A builtin or an
// unknown name is an error.
func (t *Shell) Forget(name string) (int64, error) {
	if strings.HasPrefix(name, "$r") {
		return t.ForgetResult(name)
	}
	v, builtin, ok := t.globals.Binding(name)
	switch {
	case builtin:
		return 0, fmt.Errorf("forget: %q is a builtin, not a session global", name)
	case !ok:
		return 0, fmt.Errorf("forget: %q is not a session global or a kept result ($r1, …)", name)
	}
	n, _ := EstimateSize(v, memWalkNodes)
	t.globals.Remove(map[string]struct{}{name: {}})
	return n, nil
}

// Warnings returns the last eval's warnings, after success or error. Each
// starts with "line N:".
func (t *Shell) Warnings() []string { return t.diag.Warnings }

// Note records a toolkit notice for the running eval. It reaches the agent in
// the result's diagnostics block as "note: …"; a repeated text is kept once.
// Toolkits call it for facts the agent should act on, such as a resource near
// its cap.
func (t *Shell) Note(msg string) { t.limits.Note(msg) }

// Diagnostics returns the last eval's diagnostics, after success or error.
func (t *Shell) Diagnostics() Diagnostics { return t.diag }

// EvalOptions are the per-eval settings of the eval tool and the CLI.
type EvalOptions struct {
	Profile bool      // attribute whole-eval steps to source lines
	Limits  LimitSpec // limits for this eval only, checked against the caps
}

// Diagnostics is everything an eval reports besides its value: one block at
// the end of the result, one typed line per fact.
type Diagnostics struct {
	Warnings []string
	Notes    []string
	Profile  []LineCost // highest first; nil unless profiling was asked for
	Steps    int64      // total steps while profiling
	Usage    []string   // limits hit or at 80% or more
}

// profileLines is how many lines of the profile the block shows.
const profileLines = 5

// Format renders the block to append to a tool result, or "" when empty:
// "warning:", "note:", "profile:" and "limits:" lines after a blank line.
func (d Diagnostics) Format() string {
	var lines []string
	for _, w := range d.Warnings {
		lines = append(lines, "warning: "+w)
	}
	for _, n := range d.Notes {
		lines = append(lines, "note: "+n)
	}
	if d.Profile != nil || d.Steps > 0 {
		lines = append(lines, fmt.Sprintf("profile: %d steps", d.Steps))
		for i, lc := range d.Profile {
			if i == profileLines {
				break
			}
			pct := lc.Steps * 100 / max(d.Steps, 1)
			if pct == 0 {
				break // sorted: the rest cost less than 1%
			}
			lines = append(lines, fmt.Sprintf("profile: line %d: %d steps (%d%%)", lc.Line, lc.Steps, pct))
		}
	}
	if len(d.Usage) > 0 {
		lines = append(lines, "limits: "+strings.Join(d.Usage, ", "))
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(lines, "\n")
}

// beginEval resets the limits, clears the last eval's reports and applies the
// options. A limit over an operator maximum is an error before the code runs.
func (t *Shell) beginEval(opts EvalOptions) error {
	if t.limits.ResetOnEval {
		t.limits.Reset()
	}
	t.limits.ClearWarnings()
	t.diag = Diagnostics{}
	if !opts.Limits.Empty() {
		if err := t.limits.Set("limits", opts.Limits); err != nil {
			return err
		}
	}
	t.limits.applyGCTarget()
	if opts.Profile {
		t.limits.StartProfile()
	}
	return nil
}

// endEval collects the eval's diagnostics.
func (t *Shell) endEval(itp *Interpreter, opts EvalOptions) {
	d := Diagnostics{Warnings: itp.Warnings(), Notes: t.limits.Notes(), Usage: t.limits.Usage()}
	if note := t.limits.MemoryNote(); note != "" {
		d.Notes = append(d.Notes, note)
	}
	if opts.Profile {
		d.Profile, d.Steps = t.limits.StopProfile()
		if d.Profile == nil {
			d.Profile = []LineCost{}
		}
	}
	t.diag = d
}

// FormatWarnings renders warnings as a block to append to a tool result, or
// "" when there are none.
func FormatWarnings(warnings []string) string {
	return Diagnostics{Warnings: warnings}.Format()
}

// NewShell builds a shell with default limits and the builtin `help` command.
func NewShell() *Shell {
	t := &Shell{
		commands: NewCommandRegistry(),
		limits:   NewExecutionLimits(),
		globals:  NewEnvironment(nil),
	}
	t.limits.memReport = t.memoryReport
	t.commands.Register(&CommandDef{
		Name:        "help",
		Signature:   "search?: string",
		Description: "list commands; search by name",
		Examples:    []string{"help()", `help("file")`, `help("graph")`},
		Fn: func(args []Value) Value {
			search := ""
			if len(args) > 0 {
				if s, ok := args[0].(*StringVal); ok {
					search = s.V
				}
			}
			return &StringVal{V: t.commands.Help(search)}
		},
	})
	t.commands.Register(&CommandDef{
		Name:        "commands",
		Signature:   "search?: string",
		Description: "commands as data: [{name, call, category, signature, description}]; takes what help() takes",
		Examples:    []string{`commands("file")`, `commands("server") |> filter(c => c.category == "ci")`},
		Fn: func(args []Value) Value {
			search := ""
			if len(args) > 0 {
				if s, ok := args[0].(*StringVal); ok {
					search = s.V
				}
			}
			cmds := t.commands.Find(search)
			out := make([]Value, len(cmds))
			for i, c := range cmds {
				o := NewObject()
				o.Set("name", Str(c.Name))
				o.Set("call", Str(c.CallName()))
				o.Set("category", Str(c.Category))
				o.Set("signature", Str(c.Signature))
				o.Set("description", Str(c.Description))
				out[i] = o
			}
			return &ArrayVal{Elements: out}
		},
	})
	return t
}

// Commands exposes the registry (for toolkit installation).
func (t *Shell) Commands() *CommandRegistry { return t.commands }

// Limits exposes the execution limits.
func (t *Shell) Limits() *ExecutionLimits { return t.limits }

// Register adds a command, auto-lifting the namespace object into globals.
func (t *Shell) Register(def *CommandDef) *Shell {
	if t.hook != nil {
		t.hook.guard(def)
	}
	t.commands.Register(def)
	if def.Namespace != "" {
		if nsObj := t.commands.BuildNamespaceObject(def.Namespace); nsObj != nil {
			t.globals.DefineBuiltin(def.Namespace, nsObj)
		}
	}
	return t
}

// RegisterGuide adds a named help guide.
func (t *Shell) RegisterGuide(name, content string) *Shell {
	if t.hook != nil {
		t.hook.guides = append(t.hook.guides, name)
	}
	t.commands.RegisterGuide(name, content)
	return t
}

// Eval parses and evaluates source. A mcpshell error is returned as err; bugs
// (non-ShellError panics) propagate.
func (t *Shell) Eval(source string) (Value, error) { return t.EvalWith(source, EvalOptions{}) }

// EvalWith is Eval with per-eval options.
func (t *Shell) EvalWith(source string, opts EvalOptions) (result Value, err error) {
	if err := t.beginEval(opts); err != nil {
		return Null, err
	}
	itp := NewInterpreter(t.commands, t.globals, t.limits)
	defer func() {
		t.endEval(itp, opts)
		if r := recover(); r != nil {
			result, err = Null, AsShellError(r)
		}
	}()
	return itp.Eval(source).Value, nil
}

// EvalExported evaluates source as one eval of a session (the MCP server,
// the REPL): its top-level declarations become session globals that later
// evals see, as in a notebook. A declaration of a name an earlier eval
// declared replaces it; a top-level declaration of a builtin, command or
// toolkit name is an error. `export` is accepted and means the same.
// Declarations that ran stay also when the eval fails later. vars are bound
// for this eval only.
func (t *Shell) EvalExported(source string, vars map[string]Value) (Value, error) {
	return t.EvalExportedWith(source, vars, EvalOptions{})
}

// EvalExportedWith is EvalExported with per-eval options.
func (t *Shell) EvalExportedWith(source string, vars map[string]Value, opts EvalOptions) (result Value, err error) {
	if err := t.beginEval(opts); err != nil {
		return Null, err
	}
	g := t.globals
	g.MarkEarlier()
	itp := NewInterpreter(t.commands, g, t.limits)
	itp.evalScoped = true
	// vars bind after the interpreter's builtins, so a var named like a
	// builtin (all, sum) holds the caller's value for this eval. What the
	// name held before comes back after.
	type saved struct {
		value       Value
		builtin, ok bool
	}
	prior := make(map[string]saved, len(vars))
	for name, value := range vars {
		v, b, ok := g.Binding(name)
		prior[name] = saved{v, b, ok}
		g.Rebind(name, value, false, true)
	}
	defer func() {
		for name, p := range prior {
			g.Rebind(name, p.value, p.builtin, p.ok)
		}
		t.endEval(itp, opts)
		if r := recover(); r != nil {
			result, err = Null, AsShellError(r)
		}
	}()
	return itp.Eval(source).Value, nil
}

// EvalTransactional evaluates source, restoring global state on error.
func (t *Shell) EvalTransactional(source string) (result Value, err error) {
	if err := t.beginEval(EvalOptions{}); err != nil {
		return Null, err
	}
	snapshot := t.globals.Snapshot()
	itp := NewInterpreter(t.commands, t.globals, t.limits)
	defer func() {
		t.endEval(itp, EvalOptions{})
		if r := recover(); r != nil {
			t.globals.Restore(snapshot)
			result, err = Null, AsShellError(r)
		}
	}()
	return itp.Eval(source).Value, nil
}

// GetState returns all global bindings as an object.
func (t *Shell) GetState() *ObjectVal {
	obj := NewObject()
	for name, value := range t.globals.AllBindings() {
		obj.Set(name, value)
	}
	return obj
}

// SetBuiltin binds a builtin global, such as a namespace object; a script's
// `let` of the same name reports that it is a builtin.
func (t *Shell) SetBuiltin(name string, value Value) {
	t.globals.DefineBuiltin(name, value)
}

// SetState injects bindings into the global scope.
func (t *Shell) SetState(state map[string]Value) {
	for name, value := range state {
		t.globals.DefineOrSet(name, value)
	}
}

// RemoveGlobals removes named bindings from the global scope.
func (t *Shell) RemoveGlobals(names map[string]struct{}) {
	t.globals.Remove(names)
}

// ToPrompt generates the system prompt describing mcpshell syntax and commands.
// In compact mode it lists command names only and defers detail to help().
func (t *Shell) ToPrompt(compact bool) string {
	var b strings.Builder
	b.WriteString("# mcpshell — language reference\n")
	b.WriteString("JS subset with pipes. Type annotations are accepted but ignored. Only commands listed below exist.\n\n")
	b.WriteString(PromptSyntax)
	b.WriteString("\n\n")
	if compact {
		b.WriteString("## Commands\n")
		b.WriteString(t.commands.CompactPrompt())
	} else {
		b.WriteString("## Commands (first arg is pipe input unless noted)\n")
		b.WriteString(t.commands.Prompt())
	}
	// Lists every allowed loadable, loaded or not, so the text does not change
	// after a load (a changed prompt invalidates the KV cache).
	if line := t.commands.AvailableLine(); line != "" {
		b.WriteString("\n\n" + line)
	}
	return b.String()
}

// PromptSyntax is the syntax reference embedded in ToPrompt.
const PromptSyntax = `## Syntax: JS subset
let/const/var, function, =>, if/else (parens optional with braces), while (parens optional with braces),
for/for..of/for..in, switch/case,
try/catch/finally, throw, break, continue, labels on loops, destructuring (with defaults and ...rest),
rest params, spread, ?., ??, ??=, ||=, &&=, ternary,
template strings, regex (/pattern/g for global), typeof, delete, ===, **, bitwise. All work as expected.
Loops support braceless bodies: for (let i = 0; i < n; i++) sum += i;
class (extends, super, static, get/set, #private), this, new, instanceof, prototypes, call/apply/bind,
Map, Set, WeakMap, WeakSet, Error and its subclasses work as in JS. Not supported: import, yield, generators, Symbol, Proxy.
async/await are allowed and do nothing — calls return values directly, run in order. Promise.all/allSettled/race/any([...]),
Promise.resolve/reject and .then/.catch/.finally work on these values; a failed async call is a rejection for them to catch.
For parallel calls put functions in the array: Promise.all([() => a(), () => b()]).
catch (e): e is what was thrown (throw new NotFound(…) gives the object back); an error mcpshell raises is its message string (e.message also works).
Last expression is the output. ` + "`let`" + ` returns null — end with the value you want.

## Key differences from JS
let x = 5                    // REQUIRED: assigning to an undeclared name is an error
let rows = f()               // top-level let/const/function last for later evals; naming one like a builtin or command (all, keys) is an error
fn(name: "Alice", age: 30)  // named args
[1,2,3] |> map(n => n * 10) // pipe: passes left as first arg
[1,2,3] |* (x => x * 2) |> reduce((a, x) => a + x, 0)  // scatter then pipe
"hello".toUpperCase()        // JS methods auto-resolve to commands

## Numbers are exact (no float64 rounding)
2 ** 1000, 100!, and big sums keep every digit; 0.1 + 0.2 === 0.3; 1/3 + 1/3 + 1/3 === 1.
Bitwise ops (& | ^ ~ << >> >>>) follow JS ToInt32 within ±2^53 and are exact above it, so 64-bit values XOR correctly.
BigInt(s), 123n, 0xff, 0b101 work; BigInt.asUintN(64, x) wraps to 64 bits.
Only sqrt/sin/log/non-integer powers are floating-point. So compute exactly — don't hedge about precision.
Date as in JS (local = UTC); Dates.add/diff/startOf/range/format/parse/businessDays; n.toLocaleString(), Intl.NumberFormat: help("dates"), help("format").

## Composition
all(() => a(), () => b()) // parallel   race() // first success
allSettled(() => a(), () => b()) // parallel, never throws: [{ok: true, status: "fulfilled", value} | {ok: false, status: "rejected", error, reason}]
chain(() => getData(), d => transform(d)) // sequential`

// ToolDescription is the short tool-call schema description for LLM clients.
const ToolDescription = "Execute mcpshell code (JS subset with pipes). " +
	"IMPORTANT: LLMs are bad at double-escaping. Never embed complex strings (paths, regex, user data) " +
	"as literals in code. Instead pass them via vars. " +
	"vars are bound as constants before execution. " +
	"For inline strings with backslashes use r\"...\" or r`...` (raw strings). " +
	"Top-level let/const/function/class persist across calls (no export needed). " +
	"help() lists commands."
