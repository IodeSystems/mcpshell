package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Host functions: the MCP client offers functions that eval calls as
// Host.<name>(args); each call is a request from the server to the client.
// Both sides opt in: the operator with --host-functions (WithHostFunctions),
// the client by declaring the functions in initialize. See docs/design.md
// "Host functions".

// hostCapability is the key under capabilities.experimental in the client's
// initialize params: {"functions": [{name, description, inputSchema, timeoutMs?}]}.
const hostCapability = "mcpshell/host"

// hostMethod is the request the server sends for one Host call, with params
// {"name", "arguments"}.
const hostMethod = "mcpshell/host/call"

// hostNamespace is the eval namespace of the host functions.
const hostNamespace = "Host"

// hostNameRE is a function name that Host.<name> can call.
var hostNameRE = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// hostFunction is one function a client declares.
type hostFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	TimeoutMs   int64           `json:"timeoutMs"`
}

// WithHostFunctions lets the client declare host functions in initialize
// (--host-functions). Without it a declaration is ignored.
func (s *Server) WithHostFunctions() *Server {
	s.hostAllowed = true
	return s
}

// initHost reads the client's host declaration (raw, the value of
// capabilities.experimental["mcpshell/host"]) and registers the Host
// toolkit when the operator allowed it and the client declared at least one
// function. It runs once, at the first initialize.
func (s *Server) initHost(raw json.RawMessage) {
	declared := len(raw) > 0 && string(raw) != "null"
	if !declared {
		return
	}
	if !s.hostAllowed {
		fmt.Fprintln(serverLog, "mcpshell mcp: the client declares host functions; ignored (the operator did not pass --host-functions)")
		return
	}
	if s.descFixed {
		fmt.Fprintln(serverLog, "mcpshell mcp: host functions declared after tools/list; ignored (the eval description is fixed)")
		return
	}
	var decl struct {
		Functions []hostFunction `json:"functions"`
	}
	if err := json.Unmarshal(raw, &decl); err != nil {
		fmt.Fprintf(serverLog, "mcpshell mcp: host functions ignored: bad declaration: %v\n", err)
		return
	}
	var fns []hostFunction
	seen := map[string]bool{}
	for _, f := range decl.Functions {
		switch {
		case !hostNameRE.MatchString(f.Name):
			fmt.Fprintf(serverLog, "mcpshell mcp: host function %q ignored: the name must be an identifier\n", f.Name)
			continue
		case f.Name == "help" || f.Name == "unload":
			fmt.Fprintf(serverLog, "mcpshell mcp: host function %q ignored: Host.%s is the toolkit's own\n", f.Name, f.Name)
			continue
		case seen[f.Name]:
			fmt.Fprintf(serverLog, "mcpshell mcp: host function %q ignored: declared twice\n", f.Name)
			continue
		case f.TimeoutMs < 0:
			f.TimeoutMs = 0
		}
		seen[f.Name] = true
		fns = append(fns, f)
	}
	if len(fns) == 0 {
		return
	}
	s.installHost(fns)
	names := make([]string, len(fns))
	for i, f := range fns {
		names[i] = f.Name
	}
	fmt.Fprintf(serverLog, "mcpshell mcp: host functions: %s\n", strings.Join(names, ", "))
}

// installHost registers the Host toolkit (loaded on first reference, like
// the other loadables), the eval description's Host line, and the shell
// command hint that points at the functions.
func (s *Server) installHost(fns []hostFunction) {
	var line, guide strings.Builder
	line.WriteString("Host functions (the client runs them; one object argument; Host.help() describes them): ")
	guide.WriteString("Host — functions the MCP client provides. A call blocks until the client answers; it counts against the eval's time limit. An error from the host is thrown (try/catch catches it).\n")
	for i, f := range fns {
		_, sig := schemaInfo(f.InputSchema)
		if i > 0 {
			line.WriteString(" · ")
		}
		line.WriteString(hostNamespace + "." + f.Name + "(" + sig + ")")
		if d := runtime.Brief(f.Description); d != "" {
			line.WriteString(" — " + d)
		}
	}
	s.hostLine = line.String()

	hint := "eval runs no shell commands itself (no processes, no cd)."
	if seen := hostNames(fns); seen["exec"] {
		hint += " The host provides Host.exec: Host.help() describes it."
	} else {
		hint += " The host provides " + strings.Join(hostCalls(fns), ", ") + " (Host.help() describes them); for other commands use the host's exec tool."
	}
	s.shell.Commands().SetShellHint(hint)

	s.shell.AddLoadable(runtime.Loadable{
		Name:       "host",
		Summary:    "functions the MCP client provides: " + strings.Join(hostCalls(fns), ", "),
		Guide:      guide.String(),
		Namespaces: []string{hostNamespace},
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			for _, f := range fns {
				s.registerHostFunction(sh, f)
			}
			return nil, nil
		},
	})
}

func hostNames(fns []hostFunction) map[string]bool {
	m := make(map[string]bool, len(fns))
	for _, f := range fns {
		m[f.Name] = true
	}
	return m
}

func hostCalls(fns []hostFunction) []string {
	out := make([]string, len(fns))
	for i, f := range fns {
		out[i] = hostNamespace + "." + f.Name
	}
	return out
}

// registerHostFunction adds Host.<name>. Arguments are checked as an
// upstream tool's are, for required names and lossless type conversion;
// names the schema does not declare pass (the host decides).
func (s *Server) registerHostFunction(sh *runtime.Shell, f hostFunction) {
	params, sig := schemaInfo(f.InputSchema)
	if sig == "" {
		sig = "args?: object"
	}
	required := schemaRequired(f.InputSchema)
	types := schemaTypes(f.InputSchema)
	detail := schemaDetail(f.InputSchema)
	callName := hostNamespace + "." + f.Name
	desc := f.Description
	if desc == "" {
		desc = "host function " + f.Name
	}
	sh.Register(&runtime.CommandDef{
		Namespace:   hostNamespace,
		Name:        f.Name,
		Signature:   sig,
		Description: desc,
		Params:      detail,
		ParamNames:  params,
		Fn: func(args []runtime.Value) runtime.Value {
			defer withParams(callName, detail)
			if params == nil && len(args) > 0 {
				if _, ok := args[0].(*runtime.ObjectVal); !ok && args[0] != runtime.Null {
					panic(runtime.Runtime(callName + " takes one object argument: " + callName + "({…})"))
				}
			}
			jsonArgs := argsToJSONMap(args, params)
			checkRequired(callName, hostNamespace, f.Name, jsonArgs, required)
			coerceArgs(callName, jsonArgs, types)
			res := s.hostCall(sh.Limits(), callName, f, jsonArgs)
			if len(res) == 0 {
				return runtime.Null
			}
			return toolkit.ParseJSON(string(res))
		},
	})
}

// hostPoll is how often a host call checks for a cancelled eval.
const hostPoll = 25 * time.Millisecond

// Causes of a host call that ended without an answer; the reason text of
// its notifications/cancelled.
var (
	errHostEvalTimeout = errors.New("the eval's time limit was reached")
	errHostTimeout     = errors.New("the function's timeoutMs was reached")
	errHostCancelled   = errors.New("the eval was cancelled")
)

// hostCall sends one mcpshell/host/call and waits for the answer: up to the
// function's timeoutMs, capped by the time left in the eval. One call at a
// time: a parallel branch waits for the call before it. A call that ends
// without an answer sends notifications/cancelled, and the eval gets an error.
func (s *Server) hostCall(lm *runtime.ExecutionLimits, callName string, f hostFunction, args map[string]any) json.RawMessage {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	remaining := lm.Remaining()
	if remaining <= 0 {
		panic(runtime.Runtime(fmt.Sprintf("%s: no time left in this eval's time limit (%dms); %s", callName, lm.TimeoutMs, lm.RaiseHint("timeout", lm.TimeoutMs*2))))
	}
	wait, cause := remaining, errHostEvalTimeout
	if own := time.Duration(f.TimeoutMs) * time.Millisecond; own > 0 && own < remaining {
		wait, cause = own, errHostTimeout
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	timer := time.AfterFunc(wait, func() { cancel(cause) })
	defer timer.Stop()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(hostPoll)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if lm.Cancelled() {
					cancel(errHostCancelled)
					return
				}
			}
		}
	}()

	if args == nil {
		args = map[string]any{}
	}
	id, res, err := s.request(ctx, hostMethod, map[string]any{"name": f.Name, "arguments": args})
	if err == nil {
		return res
	}
	var ce *clientError
	if errors.As(err, &ce) {
		panic(runtime.Runtime(callName + " failed: " + ce.Message))
	}
	_ = s.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled",
		"params": map[string]any{"requestId": id, "reason": err.Error()}})
	switch err {
	case errHostEvalTimeout:
		msg := fmt.Sprintf("%s: no answer within the eval's time limit (%dms); the call was cancelled.", callName, lm.TimeoutMs)
		if lm.TimeoutMs >= lm.CapTimeoutMs {
			msg += " The limit is at the operator maximum; the operator raises it with --max-limits and --limits timeout=…"
		} else {
			msg += fmt.Sprintf(" If the host needs longer, %s (operator maximum: %d)", lm.RaiseHint("timeout", min(lm.TimeoutMs*2, lm.CapTimeoutMs)), lm.CapTimeoutMs)
		}
		panic(runtime.Runtime(msg))
	case errHostTimeout:
		panic(runtime.Runtime(fmt.Sprintf("%s: no answer within its timeout (%dms, set by the host); the call was cancelled", callName, f.TimeoutMs)))
	case errHostCancelled:
		panic(runtime.Runtime(callName + ": Execution cancelled"))
	}
	panic(runtime.Runtime(callName + " failed: " + err.Error()))
}
