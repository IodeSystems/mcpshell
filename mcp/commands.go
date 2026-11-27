package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iodesystems/mcpshell/runtime"
)

// Declared commands: the operator declares programs with typed arguments
// (--commands <file.json>), and eval runs them as ns.method({…}). argv is
// built from the typed arguments and the program is started directly: no
// shell reads any value. See docs/design.md "Declared commands".

// CommandsLoadable is the loadable name of the declared commands (for
// --no-load).
const CommandsLoadable = "commands"

// Defaults of a command's limits.
const (
	defaultCommandOutput = 1_000_000 // bytes per stream
	commandWaitDelay     = 2 * time.Second
)

// commandNSRE is a command namespace: lower-case first letter, so it never
// takes a built-in toolkit's name (Files, Db, Web, Host, …).
var commandNSRE = regexp.MustCompile(`^[a-z][A-Za-z0-9_]*$`)

// envNameRE is an environment variable name in an env entry.
var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// jsKeywords cannot be a namespace: `for.x()` does not parse.
var jsKeywords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true,
	"debugger": true, "default": true, "delete": true, "do": true, "else": true, "export": true,
	"extends": true, "false": true, "finally": true, "for": true, "function": true, "if": true,
	"import": true, "in": true, "instanceof": true, "let": true, "new": true, "null": true,
	"return": true, "super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, "void": true, "while": true, "with": true,
	"yield": true, "await": true, "async": true, "undefined": true,
}

// CommandOptions says where a declared command runs.
type CommandOptions struct {
	// Dir is the default working directory, and a relative cwd resolves
	// against it (the --files-dir workspace, else the process directory).
	Dir string
	// Roots, when not empty, are the directories a cwd must be inside (the
	// operator's file roots).
	Roots []string
}

// Command is one declared program: a namespace whose methods run it.
type Command struct {
	Namespace   string
	Program     string
	Description string
	Args        []string
	Dir         string
	Env         []string // the child's environment, NAME=value
	TimeoutMs   int64
	MaxOutput   int
	Methods     []*CommandMethod
}

// CommandMethod is one callable: fixed args and typed parameters.
type CommandMethod struct {
	Name        string
	Description string
	Args        []string
	Params      []CommandParam
	DashDash    bool // "--" goes before the positionals, which may then start with "-"
	TimeoutMs   int64
	schema      json.RawMessage
}

// CommandParam is one typed parameter, mapped to a flag or a positional.
type CommandParam struct {
	Name        string
	Type        string // string, integer, number, boolean, array (of string)
	Description string
	Flag        string // "-n" (value as the next argv element) or "--n=" (joined)
	Positional  bool
	Required    bool
}

type commandFile struct {
	Commands json.RawMessage `json:"commands"`
}

type commandDecl struct {
	Program        string          `json:"program"`
	Description    string          `json:"description"`
	Args           []string        `json:"args"`
	Cwd            string          `json:"cwd"`
	Env            []string        `json:"env"`
	TimeoutMs      int64           `json:"timeoutMs"`
	MaxOutputBytes int             `json:"maxOutputBytes"`
	Methods        json.RawMessage `json:"methods"`
}

type methodDecl struct {
	Description string          `json:"description"`
	Args        []string        `json:"args"`
	Params      json.RawMessage `json:"params"`
	DashDash    bool            `json:"dashdash"`
	TimeoutMs   int64           `json:"timeoutMs"`
}

type paramDecl struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Items       *struct {
		Type string `json:"type"`
	} `json:"items"`
	Flag       string `json:"flag"`
	Positional bool   `json:"positional"`
	Required   bool   `json:"required"`
}

// strictUnmarshal decodes data into v and rejects unknown keys, so a typo
// such as "positonal" is a startup error and not an ignored setting.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("text after the JSON value")
	}
	return nil
}

// ParseCommands reads a declaration: {"commands": {"<ns>": {program, args?,
// description?, cwd?, env?, timeoutMs?, maxOutputBytes?, methods: {"<name>":
// {description?, args?, params?, dashdash?, timeoutMs?}}}}}. Each param is
// {type, description?, items?, flag | positional, required?}.
func ParseCommands(data []byte, opts CommandOptions) ([]*Command, error) {
	var f commandFile
	if err := strictUnmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("invalid JSON: %v", err)
	}
	if len(f.Commands) == 0 || string(f.Commands) == "null" {
		return nil, errors.New(`expected a "commands" object`)
	}
	var decls map[string]json.RawMessage
	if err := json.Unmarshal(f.Commands, &decls); err != nil {
		return nil, fmt.Errorf("commands: %v", err)
	}
	base := opts.Dir
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		base = wd
	}
	var out []*Command
	for _, ns := range orderedKeys(f.Commands) {
		c, err := parseCommand(ns, decls[ns], base, opts.Roots)
		if err != nil {
			return nil, fmt.Errorf("commands.%s: %v", ns, err)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.New("commands: no command declared")
	}
	return out, nil
}

func parseCommand(ns string, raw json.RawMessage, base string, roots []string) (*Command, error) {
	if !commandNSRE.MatchString(ns) || jsKeywords[ns] {
		return nil, errors.New("the name must be an identifier that starts with a lower-case letter (upper-case names are the built-in toolkits')")
	}
	var d commandDecl
	if err := strictUnmarshal(raw, &d); err != nil {
		return nil, err
	}
	if d.Program == "" {
		return nil, errors.New(`"program" is required`)
	}
	if d.TimeoutMs < 0 || d.MaxOutputBytes < 0 {
		return nil, errors.New("timeoutMs and maxOutputBytes must not be negative")
	}
	c := &Command{Namespace: ns, Program: d.Program, Description: d.Description, Args: d.Args,
		TimeoutMs: d.TimeoutMs, MaxOutput: d.MaxOutputBytes}
	if c.MaxOutput == 0 {
		c.MaxOutput = defaultCommandOutput
	}
	// A relative program path with a separator is relative to the
	// declaration's base, not to the cwd the child starts in.
	if strings.ContainsRune(c.Program, filepath.Separator) && !filepath.IsAbs(c.Program) {
		c.Program = filepath.Join(base, c.Program)
	}
	if _, err := exec.LookPath(c.Program); err != nil {
		fmt.Fprintf(serverLog, "mcpshell: commands.%s: program %q not found now; a call fails until it is installed\n", ns, d.Program)
	}

	dir := d.Cwd
	if dir == "" {
		dir = base
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, dir)
	}
	dir = filepath.Clean(dir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("cwd %s is not a directory", dir)
	}
	if len(roots) > 0 && !insideAny(dir, roots) {
		return nil, fmt.Errorf("cwd %s is outside the file roots (%s)", dir, strings.Join(roots, ", "))
	}
	c.Dir = dir

	env, err := commandEnv(d.Env)
	if err != nil {
		return nil, err
	}
	c.Env = env

	if len(d.Methods) == 0 || string(d.Methods) == "null" {
		return nil, errors.New(`"methods" is required: {"<name>": {args, params}}`)
	}
	var methods map[string]json.RawMessage
	if err := json.Unmarshal(d.Methods, &methods); err != nil {
		return nil, fmt.Errorf("methods: %v", err)
	}
	for _, name := range orderedKeys(d.Methods) {
		m, err := parseMethod(name, methods[name])
		if err != nil {
			return nil, fmt.Errorf("methods.%s: %v", name, err)
		}
		c.Methods = append(c.Methods, m)
	}
	if len(c.Methods) == 0 {
		return nil, errors.New("no method declared")
	}
	return c, nil
}

// insideAny reports whether dir is one of roots or below one.
func insideAny(dir string, roots []string) bool {
	for _, r := range roots {
		rel, err := filepath.Rel(r, dir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// commandEnv builds a child's environment: PATH, then each entry, "NAME"
// (copied from mcpshell's environment when set) or "NAME=value".
func commandEnv(entries []string) ([]string, error) {
	vals := map[string]string{}
	var order []string
	set := func(k, v string) {
		if _, ok := vals[k]; !ok {
			order = append(order, k)
		}
		vals[k] = v
	}
	if p, ok := os.LookupEnv("PATH"); ok {
		set("PATH", p)
	}
	for _, e := range entries {
		name, value, fixed := strings.Cut(e, "=")
		if !envNameRE.MatchString(name) {
			return nil, fmt.Errorf("env %q: expected NAME or NAME=value", e)
		}
		if fixed {
			set(name, value)
		} else if v, ok := os.LookupEnv(name); ok {
			set(name, v)
		}
	}
	out := make([]string, len(order))
	for i, k := range order {
		out[i] = k + "=" + vals[k]
	}
	return out, nil
}

func parseMethod(name string, raw json.RawMessage) (*CommandMethod, error) {
	if !hostNameRE.MatchString(name) {
		return nil, errors.New("the name must be an identifier")
	}
	if name == "help" || name == "unload" {
		return nil, fmt.Errorf("%s is the toolkit's own", name)
	}
	var d methodDecl
	if err := strictUnmarshal(raw, &d); err != nil {
		return nil, err
	}
	if d.TimeoutMs < 0 {
		return nil, errors.New("timeoutMs must not be negative")
	}
	m := &CommandMethod{Name: name, Description: d.Description, Args: d.Args, DashDash: d.DashDash, TimeoutMs: d.TimeoutMs}
	var params map[string]json.RawMessage
	if len(d.Params) > 0 && string(d.Params) != "null" {
		if err := json.Unmarshal(d.Params, &params); err != nil {
			return nil, fmt.Errorf("params: %v", err)
		}
	}
	optionalPositional := ""
	for _, pn := range orderedKeys(d.Params) {
		p, err := parseParam(pn, params[pn])
		if err != nil {
			return nil, fmt.Errorf("params.%s: %v", pn, err)
		}
		if p.Positional {
			// A missing optional positional would shift the next one
			// into its place.
			if optionalPositional != "" {
				return nil, fmt.Errorf("params.%s: a positional after the optional positional %q; make %q required or a flag", pn, optionalPositional, optionalPositional)
			}
			if !p.Required {
				optionalPositional = pn
			}
		}
		m.Params = append(m.Params, p)
	}
	m.schema = methodSchema(m.Params)
	return m, nil
}

func parseParam(name string, raw json.RawMessage) (CommandParam, error) {
	var d paramDecl
	if !hostNameRE.MatchString(name) {
		return CommandParam{}, errors.New("the name must be an identifier")
	}
	if err := strictUnmarshal(raw, &d); err != nil {
		return CommandParam{}, err
	}
	p := CommandParam{Name: name, Type: d.Type, Description: d.Description, Flag: d.Flag, Positional: d.Positional, Required: d.Required}
	switch d.Type {
	case "string", "integer", "number", "boolean":
		if d.Items != nil {
			return p, errors.New(`"items" is only for type array`)
		}
	case "array":
		if d.Items == nil || d.Items.Type != "string" {
			return p, errors.New(`an array must have "items": {"type": "string"}`)
		}
	default:
		return p, fmt.Errorf("type %q: expected string, integer, number, boolean or array", d.Type)
	}
	switch {
	case d.Positional && d.Flag != "":
		return p, errors.New(`"flag" and "positional" exclude each other`)
	case !d.Positional && d.Flag == "":
		return p, errors.New(`needs "flag" ("-n", or "--name=" for the joined form) or "positional": true`)
	case d.Positional && d.Type == "boolean":
		return p, errors.New("a boolean is a flag, not a positional")
	case d.Flag != "" && (!strings.HasPrefix(d.Flag, "-") || d.Flag == "-" || d.Flag == "--"):
		return p, fmt.Errorf("flag %q must start with -", d.Flag)
	case d.Type == "boolean" && strings.HasSuffix(d.Flag, "="):
		return p, errors.New(`a boolean flag has no value: drop the "="`)
	case strings.ContainsAny(d.Flag, " \t\n\x00"):
		return p, fmt.Errorf("flag %q has a space", d.Flag)
	}
	return p, nil
}

// methodSchema is the JSON Schema of a method's parameters, in declaration
// order, for the shared argument checks and the signature. Its keywords are
// inside Gemini's OpenAPI subset: plain type strings, items, required.
func methodSchema(params []CommandParam) json.RawMessage {
	var b bytes.Buffer
	b.WriteString(`{"type":"object","properties":{`)
	var required []string
	for i, p := range params {
		if i > 0 {
			b.WriteByte(',')
		}
		prop := map[string]any{"type": p.Type}
		if p.Type == "array" {
			prop["items"] = map[string]any{"type": "string"}
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		name, _ := json.Marshal(p.Name)
		val, _ := json.Marshal(prop)
		b.Write(name)
		b.WriteByte(':')
		b.Write(val)
		if p.Required {
			required = append(required, p.Name)
		}
	}
	b.WriteString(`}`)
	if len(required) > 0 {
		r, _ := json.Marshal(required)
		b.WriteString(`,"required":`)
		b.Write(r)
	}
	b.WriteString(`}`)
	return b.Bytes()
}

// BuildArgv is the argv after the program for one call: the command's fixed
// args, the method's, each flag in declaration order, then "--" (dashdash)
// and the positionals. args are checked and converted (checkMethodArgs).
// A positional that starts with "-" is an error unless dashdash is set.
func (m *CommandMethod) BuildArgv(c *Command, args map[string]any) ([]string, error) {
	argv := slices.Concat(c.Args, m.Args)
	var positionals []string
	for _, p := range m.Params {
		v, ok := args[p.Name]
		if !ok || v == nil {
			continue
		}
		var vals []string
		switch p.Type {
		case "boolean":
			if v.(bool) {
				argv = append(argv, p.Flag)
			}
			continue
		case "array":
			for i, e := range v.([]any) {
				s, ok := argText(e)
				if !ok {
					return nil, fmt.Errorf("argument '%s'[%d] must be a string, got %s", p.Name, i, jsonKind(e))
				}
				vals = append(vals, s)
			}
		case "integer":
			// 5.0 passes the integer check; the program gets 5.
			s, _ := argText(v)
			if r, ok := new(big.Rat).SetString(s); ok && r.IsInt() {
				s = r.Num().String()
			}
			vals = []string{s}
		default:
			s, _ := argText(v)
			vals = []string{s}
		}
		for _, s := range vals {
			if strings.IndexByte(s, 0) >= 0 {
				return nil, fmt.Errorf("argument '%s' has a NUL byte", p.Name)
			}
		}
		if p.Positional {
			for _, s := range vals {
				if strings.HasPrefix(s, "-") && !m.DashDash {
					return nil, fmt.Errorf("argument '%s' is %q: a positional value may not start with \"-\" (the program would read it as an option)", p.Name, s)
				}
			}
			positionals = append(positionals, vals...)
			continue
		}
		for _, s := range vals {
			if strings.HasSuffix(p.Flag, "=") {
				argv = append(argv, p.Flag+s)
			} else {
				argv = append(argv, p.Flag, s)
			}
		}
	}
	if m.DashDash && len(positionals) > 0 {
		argv = append(argv, "--")
	}
	return append(argv, positionals...), nil
}

// argText is a string or number argument as argv text.
func argText(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	}
	return "", false
}

// Commands is what InstallCommands registered, for the server's eval
// description and shell hint.
type Commands struct {
	line  string
	calls []string
}

// InstallCommands registers the declared commands as one loadable,
// "commands", with a namespace for each command, and sets the shell hint
// to name them. It fails when a namespace is taken.
func InstallCommands(sh *runtime.Shell, cmds []*Command) (*Commands, error) {
	reg := sh.Commands()
	seen := map[string]bool{}
	var nss, calls []string
	var line strings.Builder
	line.WriteString("Commands (programs the operator declared, run without a shell; one object argument; result {stdout, stderr, exit}, a nonzero exit is a value; ns.help() describes them): ")
	for _, c := range cmds {
		switch {
		case seen[c.Namespace]:
			return nil, fmt.Errorf("commands: %s is declared twice", c.Namespace)
		case reg.Get(c.Namespace) != nil:
			return nil, fmt.Errorf("commands: %s is a built-in command", c.Namespace)
		case slices.Contains(reg.NamespaceNames(), c.Namespace) || reg.LoadableOwning(c.Namespace) != "":
			return nil, fmt.Errorf("commands: the namespace %s is taken", c.Namespace)
		}
		seen[c.Namespace] = true
		nss = append(nss, c.Namespace)
		for _, m := range c.Methods {
			call := c.Namespace + "." + m.Name
			_, sig := schemaInfo(m.schema)
			if len(calls) > 0 {
				line.WriteString(" · ")
			}
			calls = append(calls, call)
			line.WriteString(call + "(" + sig + ")")
			if d := runtime.Brief(m.Description); d != "" {
				line.WriteString(" — " + d)
			}
		}
	}
	guide := "Declared commands — programs the operator allows. A call runs the program directly (no shell: quotes, $, | and globs in a value are plain text) " +
		"with argv built from the typed arguments, and returns {stdout, stderr, exit}. A nonzero exit is a value, not an error; " +
		"a program that cannot start is an error. A call counts against the eval's time limit and is killed at its timeout. " +
		"Output over the command's cap is cut (a note says so).\n"
	sh.AddLoadable(runtime.Loadable{
		Name:       CommandsLoadable,
		Summary:    "programs the operator declared, run without a shell, each returns {stdout, stderr, exit}: " + strings.Join(calls, ", "),
		Guide:      guide,
		Namespaces: nss,
		Install: func(sh *runtime.Shell) (io.Closer, error) {
			for _, c := range cmds {
				for _, m := range c.Methods {
					registerCommandMethod(sh, c, m)
				}
			}
			return nil, nil
		},
	})
	cs := &Commands{line: line.String(), calls: calls}
	reg.SetShellHint(shellHint(cs, nil))
	return cs, nil
}

// Namespaces returns the declared namespaces.
func (c *Commands) Namespaces() []string {
	var out []string
	for _, call := range c.calls {
		ns, _, _ := strings.Cut(call, ".")
		if !slices.Contains(out, ns) {
			out = append(out, ns)
		}
	}
	return out
}

// WithCommands adds the declared commands' line to the eval description and
// keeps them in the shell hint once host functions are declared.
func (s *Server) WithCommands(c *Commands) *Server {
	s.commands = c
	return s
}

// shellHint is the error text for a shell-command name (exec, Shell, …):
// the declared commands and host functions eval can call, then the host's
// exec tool for anything else.
func shellHint(cs *Commands, fns []hostFunction) string {
	hint := "eval runs no shell commands itself (no processes, no cd)."
	if cs != nil && len(cs.calls) > 0 {
		hint += " The operator declared these commands: " + strings.Join(cs.calls, ", ") +
			" (each returns {stdout, stderr, exit}; " + cs.Namespaces()[0] + ".help() describes them)."
	}
	if len(fns) > 0 {
		if hostNames(fns)["exec"] {
			return hint + " The host provides Host.exec: Host.help() describes it."
		}
		return hint + " The host provides " + strings.Join(hostCalls(fns), ", ") + " (Host.help() describes them); for other commands use the host's exec tool."
	}
	return hint + " For other commands use the host's exec tool."
}

func registerCommandMethod(sh *runtime.Shell, c *Command, m *CommandMethod) {
	params, sig := schemaInfo(m.schema)
	required := schemaRequired(m.schema)
	types := schemaTypes(m.schema)
	detail := schemaDetail(m.schema)
	callName := c.Namespace + "." + m.Name
	desc := m.Description
	if desc == "" {
		desc = "runs " + strings.Join(slices.Concat([]string{filepath.Base(c.Program)}, c.Args, m.Args), " ")
	}
	sh.Register(&runtime.CommandDef{
		Namespace:   c.Namespace,
		Name:        m.Name,
		Signature:   sig,
		Description: desc,
		Params:      detail,
		ParamNames:  params,
		Fn: func(args []runtime.Value) runtime.Value {
			defer withParams(callName, detail)
			if len(args) > 0 {
				if _, ok := args[0].(*runtime.ObjectVal); !ok && args[0] != runtime.Null && params == nil {
					panic(runtime.Runtime(callName + " takes no arguments"))
				}
			}
			jsonArgs := argsToJSONMap(args, params)
			checkArgs(callName, c.Namespace, m.Name, jsonArgs, params, true)
			checkRequired(callName, c.Namespace, m.Name, jsonArgs, required)
			coerceArgs(callName, jsonArgs, types)
			argv, err := m.BuildArgv(c, jsonArgs)
			if err != nil {
				panic(runtime.Runtime(callName + ": " + err.Error()))
			}
			return runCommand(sh, callName, c, m, argv)
		},
	})
}

// commandPoll is how often a running command checks for a cancelled eval.
const commandPoll = 25 * time.Millisecond

var (
	errCmdEvalTimeout = errors.New("the eval's time limit")
	errCmdTimeout     = errors.New("the command's timeoutMs")
	errCmdCancelled   = errors.New("the eval was cancelled")
)

// cappedBuffer keeps the first max bytes written and counts the rest.
type cappedBuffer struct {
	buf   bytes.Buffer
	max   int
	total int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.total += len(p)
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// text is the kept bytes, cut back to a whole UTF-8 character when cut.
func (b *cappedBuffer) text() string {
	data := b.buf.Bytes()
	if b.total > len(data) {
		for i := 0; i < utf8.UTFMax && len(data) > 0 && !utf8.Valid(data); i++ {
			data = data[:len(data)-1]
		}
	}
	return string(data)
}

// runCommand starts the program and waits up to the method's (else the
// command's) timeoutMs, capped by the eval's time left. A timeout or a
// cancelled eval kills the process group and is an error; any exit is a
// value.
func runCommand(sh *runtime.Shell, callName string, c *Command, m *CommandMethod, argv []string) runtime.Value {
	lm := sh.Limits()
	remaining := lm.Remaining()
	if remaining <= 0 {
		panic(runtime.Runtime(fmt.Sprintf("%s: no time left in this eval's time limit (%dms); %s", callName, lm.TimeoutMs, lm.RaiseHint("timeout", lm.TimeoutMs*2))))
	}
	timeoutMs := m.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = c.TimeoutMs
	}
	wait, cause := remaining, errCmdEvalTimeout
	if own := time.Duration(timeoutMs) * time.Millisecond; own > 0 && own < remaining {
		wait, cause = own, errCmdTimeout
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	timer := time.AfterFunc(wait, func() { cancel(cause) })
	defer timer.Stop()
	go func() {
		t := time.NewTicker(commandPoll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if lm.Cancelled() {
					cancel(errCmdCancelled)
					return
				}
			}
		}
	}()

	cmd := exec.CommandContext(ctx, c.Program, argv...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	stdout := &cappedBuffer{max: c.MaxOutput}
	stderr := &cappedBuffer{max: c.MaxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	setProcessGroup(cmd)
	cmd.WaitDelay = commandWaitDelay
	if err := cmd.Start(); err != nil {
		panic(runtime.Runtime(fmt.Sprintf("%s: cannot start %s: %v", callName, c.Program, err)))
	}
	err := cmd.Wait()
	// A program that exits as the timer fires still has its exit status.
	if err != nil {
		killedBy(ctx, callName, lm, timeoutMs)
	}
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			panic(runtime.Runtime(fmt.Sprintf("%s: %s failed: %v", callName, c.Program, err)))
		}
		exit = ee.ExitCode() // -1 when a signal ended it
	}
	for _, s := range []struct {
		name string
		b    *cappedBuffer
	}{{"stdout", stdout}, {"stderr", stderr}} {
		if s.b.total > s.b.buf.Len() {
			sh.Note(fmt.Sprintf("%s: %s cut at %d of %d bytes (the command's maxOutputBytes)", callName, s.name, s.b.buf.Len(), s.b.total))
		}
	}
	res := runtime.NewObject()
	res.Set("stdout", runtime.Str(stdout.text()))
	res.Set("stderr", runtime.Str(stderr.text()))
	res.Set("exit", runtime.Num(float64(exit)))
	return res
}

// killedBy raises the error for a call whose process was killed because
// ctx ended (the eval's time limit, the command's timeout, a cancel), and
// returns when ctx has not ended.
func killedBy(ctx context.Context, callName string, lm *runtime.ExecutionLimits, timeoutMs int64) {
	switch context.Cause(ctx) {
	case errCmdEvalTimeout:
		msg := fmt.Sprintf("%s: still running at the eval's time limit (%dms); the process was killed.", callName, lm.TimeoutMs)
		if lm.TimeoutMs >= lm.CapTimeoutMs {
			msg += " The limit is at the operator maximum; the operator raises it with --max-limits and --limits timeout=…"
		} else {
			msg += fmt.Sprintf(" If it needs longer, %s (operator maximum: %d)", lm.RaiseHint("timeout", min(lm.TimeoutMs*2, lm.CapTimeoutMs)), lm.CapTimeoutMs)
		}
		panic(runtime.Runtime(msg))
	case errCmdTimeout:
		panic(runtime.Runtime(fmt.Sprintf("%s: still running at its timeout (%dms, set by the operator); the process was killed", callName, timeoutMs)))
	case errCmdCancelled:
		panic(runtime.Runtime(callName + ": Execution cancelled; the process was killed"))
	}
}
