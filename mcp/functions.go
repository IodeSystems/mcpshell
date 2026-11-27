package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// In-process functions: a Go program that embeds mcpshell registers Go
// functions with JSON Schema parameters as a toolkit, and eval calls them as
// Ns.name({…}). The embedding program is the operator, so no flag gates
// them. See docs/design.md "In-process functions".

// Toolkit is a namespace of Go functions for eval.
type Toolkit struct {
	// Namespace is the eval global, such as "Inventory". The loadable name
	// (for DenyLoadables, mcp --no-load) is the namespace in lower case.
	Namespace string
	// Summary is one line for help() and the prompt's Toolkits line.
	Summary string
	// Guide, if set, is added to Ns.help() after the function list.
	Guide     string
	Functions []Function
}

// Function is one Go function: eval calls it as Ns.Name({…}).
type Function struct {
	Name        string
	Description string // the first line is the brief in listings
	// InputSchema is a JSON Schema object for the one object argument; nil
	// declares no parameters. Arguments are checked against it as an
	// upstream tool's are: required names, lossless type conversion, and,
	// with "additionalProperties": false, undeclared names.
	InputSchema json.RawMessage
	Fn          FuncHandler
}

// FuncHandler runs one call. args holds JSON values: string, bool, nil,
// json.Number (exact), []any, map[string]any. ctx ends when the eval's time
// limit is reached or the eval is cancelled; the eval waits for the handler
// to return, so a long handler should watch ctx. The result is converted
// through JSON (encoding/json rules); a runtime.Value is used as is. An
// error is thrown in the script, where try/catch catches it; a
// *runtime.ShellError is thrown unchanged. Calls from parallel branches
// (Promise.all with functions) can run at the same time.
type FuncHandler func(ctx context.Context, args map[string]any) (any, error)

// toolkitNSRE is a namespace or function name Ns.name can call.
var toolkitNSRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// InstallToolkit registers tk on sh as a loadable: the first reference to
// tk.Namespace installs it, Ns.help() describes it, and the Toolkits line
// lists it. It fails when a name is invalid or the namespace is taken.
func InstallToolkit(sh *runtime.Shell, tk Toolkit) error {
	ns := tk.Namespace
	reg := sh.Commands()
	switch {
	case !toolkitNSRE.MatchString(ns) || jsKeywords[ns]:
		return fmt.Errorf("toolkit %q: the namespace must be an identifier", ns)
	case len(tk.Functions) == 0:
		return fmt.Errorf("toolkit %s: no functions", ns)
	case reg.Get(ns) != nil:
		return fmt.Errorf("toolkit %s: %s is a built-in command", ns, ns)
	case slices.Contains(reg.NamespaceNames(), ns) || reg.LoadableOwning(ns) != "" ||
		slices.Contains(reg.LoadableNames(), strings.ToLower(ns)):
		return fmt.Errorf("toolkit %s: the namespace is taken", ns)
	}
	seen := map[string]bool{}
	for _, f := range tk.Functions {
		switch {
		case !toolkitNSRE.MatchString(f.Name):
			return fmt.Errorf("toolkit %s: function %q: the name must be an identifier", ns, f.Name)
		case f.Name == "help" || f.Name == "unload":
			return fmt.Errorf("toolkit %s: function %q: %s.%s is the toolkit's own", ns, f.Name, ns, f.Name)
		case seen[f.Name]:
			return fmt.Errorf("toolkit %s: function %q is declared twice", ns, f.Name)
		case f.Fn == nil:
			return fmt.Errorf("toolkit %s: function %q has no Fn", ns, f.Name)
		case f.InputSchema != nil && !schemaIsObject(f.InputSchema):
			return fmt.Errorf("toolkit %s: function %q: InputSchema must be a JSON object", ns, f.Name)
		}
		seen[f.Name] = true
	}
	fns := slices.Clone(tk.Functions)
	sh.AddLoadable(runtime.Loadable{
		Name:       strings.ToLower(ns),
		Summary:    tk.Summary,
		Guide:      tk.Guide,
		Namespaces: []string{ns},
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			for _, f := range fns {
				registerFunction(sh, ns, f)
			}
			return nil, nil
		},
	})
	return nil
}

// AddToolkit installs tk on the server's shell (InstallToolkit) and adds a
// line with each function's signature to the eval description. It fails
// after the first tools/list, when the description is fixed.
func (s *Server) AddToolkit(tk Toolkit) error {
	if s.descFixed {
		return fmt.Errorf("toolkit %s: the eval description is fixed (tools/list was answered)", tk.Namespace)
	}
	if err := InstallToolkit(s.shell, tk); err != nil {
		return err
	}
	var line strings.Builder
	line.WriteString(tk.Namespace + " (one object argument; " + tk.Namespace + ".help() describes them): ")
	for i, f := range tk.Functions {
		if i > 0 {
			line.WriteString(" · ")
		}
		line.WriteString(tk.Namespace + "." + f.Name + "(" + functionSignature(f.InputSchema) + ")")
		if d := runtime.Brief(f.Description); d != "" {
			line.WriteString(" — " + d)
		}
	}
	s.toolkitLines = append(s.toolkitLines, line.String())
	return nil
}

func schemaIsObject(schema json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(schema, &m) == nil && m != nil
}

func functionSignature(schema json.RawMessage) string {
	if _, sig := schemaInfo(schema); sig != "" {
		return sig
	}
	return "args?: object"
}

// registerFunction adds Ns.<name>.
func registerFunction(sh *runtime.Shell, ns string, f Function) {
	params, _ := schemaInfo(f.InputSchema)
	sig := functionSignature(f.InputSchema)
	required := schemaRequired(f.InputSchema)
	types := schemaTypes(f.InputSchema)
	closed := schemaClosed(f.InputSchema)
	detail := schemaDetail(f.InputSchema)
	callName := ns + "." + f.Name
	desc := f.Description
	if desc == "" {
		desc = f.Name
	}
	sh.Register(&runtime.CommandDef{
		Namespace:   ns,
		Name:        f.Name,
		Signature:   sig,
		Description: desc,
		Params:      detail,
		ParamNames:  params,
		Fn: func(args []runtime.Value) runtime.Value {
			// A failed check carries the parameter list; an error from the
			// handler is the program's own message, for catch (e).
			jsonArgs := func() map[string]any {
				defer withParams(callName, detail)
				if params == nil && len(args) > 0 {
					if _, ok := args[0].(*runtime.ObjectVal); !ok && args[0] != runtime.Null {
						panic(runtime.Runtime(callName + " takes one object argument: " + callName + "({…})"))
					}
				}
				jsonArgs := argsToJSONMap(args, params)
				if closed {
					checkArgs(callName, ns, f.Name, jsonArgs, params, true)
				}
				checkRequired(callName, ns, f.Name, jsonArgs, required)
				coerceArgs(callName, jsonArgs, types)
				return jsonArgs
			}()
			return callFunction(sh.Limits(), callName, f.Fn, jsonArgs)
		},
	})
}

// funcPoll is how often a running function's context checks for a
// cancelled eval.
const funcPoll = 25 * time.Millisecond

var (
	errFnEvalTimeout = errors.New("the eval's time limit was reached")
	errFnCancelled   = errors.New("the eval was cancelled")
)

// callFunction runs fn with a context that ends at the eval's time limit or
// on ExecutionLimits.Cancel, and converts its result.
func callFunction(lm *runtime.ExecutionLimits, callName string, fn FuncHandler, args map[string]any) runtime.Value {
	remaining := lm.Remaining()
	if remaining <= 0 {
		panic(runtime.Runtime(fmt.Sprintf("%s: no time left in this eval's time limit (%dms); %s", callName, lm.TimeoutMs, lm.RaiseHint("timeout", lm.TimeoutMs*2))))
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	timer := time.AfterFunc(remaining, func() { cancel(errFnEvalTimeout) })
	defer timer.Stop()
	go func() {
		t := time.NewTicker(funcPoll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if lm.Cancelled() {
					cancel(errFnCancelled)
					return
				}
			}
		}
	}()

	if args == nil {
		args = map[string]any{}
	}
	res, err := runHandler(ctx, callName, fn, args)
	if err != nil {
		var se *runtime.ShellError
		if errors.As(err, &se) {
			panic(se)
		}
		switch context.Cause(ctx) {
		case errFnEvalTimeout:
			msg := fmt.Sprintf("%s: still running at the eval's time limit (%dms); its context was cancelled.", callName, lm.TimeoutMs)
			if lm.TimeoutMs >= lm.CapTimeoutMs {
				msg += " The limit is at the operator maximum."
			} else {
				msg += fmt.Sprintf(" If it needs longer, %s (operator maximum: %d)", lm.RaiseHint("timeout", min(lm.TimeoutMs*2, lm.CapTimeoutMs)), lm.CapTimeoutMs)
			}
			panic(runtime.Runtime(msg))
		case errFnCancelled:
			panic(runtime.Runtime(callName + ": Execution cancelled"))
		}
		panic(runtime.Runtime(callName + " failed: " + err.Error()))
	}
	switch v := res.(type) {
	case nil:
		return runtime.Null
	case runtime.Value:
		return v
	case string:
		return runtime.Str(v)
	}
	b, err := json.Marshal(res)
	if err != nil {
		panic(runtime.Runtime(fmt.Sprintf("%s: the result cannot be converted: %v", callName, err)))
	}
	return toolkit.ParseJSON(string(b))
}

// runHandler calls fn and turns a Go panic in it into an error: the bug is
// the embedding program's, not mcpshell's.
func runHandler(ctx context.Context, callName string, fn FuncHandler, args map[string]any) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if se, ok := r.(*runtime.ShellError); ok {
				err = se
				return
			}
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(ctx, args)
}
