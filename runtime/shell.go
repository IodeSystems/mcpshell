package runtime

import "strings"

// Shell is the top-level facade: it owns a command registry, global scope,
// and execution limits, and evaluates source against them.
type Shell struct {
	commands *CommandRegistry
	limits   *ExecutionLimits
	globals  *Environment
}

// NewShell builds a shell with default limits and the builtin `help` command.
func NewShell() *Shell {
	t := &Shell{
		commands: NewCommandRegistry(),
		limits:   NewExecutionLimits(),
		globals:  NewEnvironment(nil),
	}
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
	return t
}

// Commands exposes the registry (for toolkit installation).
func (t *Shell) Commands() *CommandRegistry { return t.commands }

// Limits exposes the execution limits.
func (t *Shell) Limits() *ExecutionLimits { return t.limits }

// Register adds a command, auto-lifting the namespace object into globals.
func (t *Shell) Register(def *CommandDef) *Shell {
	t.commands.Register(def)
	if def.Namespace != "" {
		if nsObj := t.commands.BuildNamespaceObject(def.Namespace); nsObj != nil {
			t.globals.DefineOrSet(def.Namespace, nsObj)
		}
	}
	return t
}

// RegisterGuide adds a named help guide.
func (t *Shell) RegisterGuide(name, content string) *Shell {
	t.commands.RegisterGuide(name, content)
	return t
}

// Eval parses and evaluates source. A mcpshell error is returned as err; bugs
// (non-ShellError panics) propagate.
func (t *Shell) Eval(source string) (result Value, err error) {
	if t.limits.ResetOnEval {
		t.limits.Reset()
	}
	defer func() {
		if r := recover(); r != nil {
			if te, ok := r.(*ShellError); ok {
				result, err = Null, te
				return
			}
			panic(r)
		}
	}()
	return NewInterpreter(t.commands, t.globals, t.limits).Eval(source).Value, nil
}

// EvalExported evaluates source in an isolated child scope, promoting only
// names marked with `export` into globals. On error nothing is promoted.
func (t *Shell) EvalExported(source string, vars map[string]Value) (result Value, err error) {
	if t.limits.ResetOnEval {
		t.limits.Reset()
	}
	childEnv := t.globals.Child()
	for name, value := range vars {
		childEnv.Define(name, value)
	}
	defer func() {
		if r := recover(); r != nil {
			if te, ok := r.(*ShellError); ok {
				result, err = Null, te
				return
			}
			panic(r)
		}
	}()
	res := NewInterpreter(t.commands, childEnv, t.limits).Eval(source)
	for name := range res.ExportedNames {
		if value := childEnv.Get(name); value != nil {
			t.globals.DefineOrSet(name, value)
		}
	}
	return res.Value, nil
}

// EvalTransactional evaluates source, restoring global state on error.
func (t *Shell) EvalTransactional(source string) (result Value, err error) {
	if t.limits.ResetOnEval {
		t.limits.Reset()
	}
	snapshot := t.globals.Snapshot()
	defer func() {
		if r := recover(); r != nil {
			t.globals.Restore(snapshot)
			if te, ok := r.(*ShellError); ok {
				result, err = Null, te
				return
			}
			panic(r)
		}
	}()
	return NewInterpreter(t.commands, t.globals, t.limits).Eval(source).Value, nil
}

// GetState returns all global bindings as an object.
func (t *Shell) GetState() *ObjectVal {
	obj := NewObject()
	for name, value := range t.globals.AllBindings() {
		obj.Set(name, value)
	}
	return obj
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
	return b.String()
}

// PromptSyntax is the syntax reference embedded in ToPrompt.
const PromptSyntax = `## Syntax: JS subset
let/const/var, function, =>, if/else (parens optional with braces), while (parens optional with braces),
for/for..of/for..in, switch/case,
try/catch/finally, throw, break, continue, destructuring, spread, ?., ??, ternary,
template strings, regex (/pattern/g for global), typeof, delete, ===, **, bitwise. All work as expected.
Loops support braceless bodies: for (let i = 0; i < n; i++) sum += i;
Not supported: class, new, this, import, yield, async/await, generators.
Last expression is the output. ` + "`let`" + ` returns null — end with the value you want.

## Key differences from JS
export let x = 10            // persists across eval calls; without export, discarded
fn(name: "Alice", age: 30)  // named args
[1,2,3] |> map(n => n * 10) // pipe: passes left as first arg
[1,2,3] |* (x => x * 2) |> reduce((a, x) => a + x, 0)  // scatter then pipe
"hello".toUpperCase()        // JS methods auto-resolve to commands

## Composition
all(() => a(), () => b()) // parallel   race() // first success
chain(() => getData(), d => transform(d)) // sequential`

// ToolDescription is the short tool-call schema description for LLM clients.
const ToolDescription = "Execute mcpshell code (JS subset with pipes). " +
	"IMPORTANT: LLMs are bad at double-escaping. Never embed complex strings (paths, regex, user data) " +
	"as literals in code. Instead pass them via vars. " +
	"vars are bound as constants before execution. " +
	"For inline strings with backslashes use r\"...\" or r`...` (raw strings). " +
	"export persists values across calls; without export, state is discarded. " +
	"help() lists commands."
