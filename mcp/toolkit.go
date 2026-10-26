package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// ServerConfig describes an upstream MCP server to connect to.
// Exactly one of Command (stdio) or URL (streamable HTTP) is set.
type ServerConfig struct {
	Command     []string
	Env         map[string]string
	URL         string
	Headers     map[string]string   // static HTTP headers
	AuthCmd     string              // shell command printing a bearer token (HTTP only)
	Categories  map[string][]string // category → tool-name globs; nil groups by name prefix
	Unchecked   []string            // tool-name globs whose arguments are sent as given, unchecked
	Label       string
	Description string
}

// NamedServer pairs a mcpshell namespace with an upstream server config.
type NamedServer struct {
	Namespace string
	Config    ServerConfig
}

// reservedToolNames are mcpshell's own tool names. An upstream server exposing
// any of them is treated as a mcpshell-equivalent and skipped.
var reservedToolNames = map[string]bool{"eval": true, "help": true, "prompt": true}

// reservedCollision returns the first reserved tool name an upstream exposes,
// or "" if none collide.
func reservedCollision(tools []ToolInfo) string {
	for _, t := range tools {
		if reservedToolNames[t.Name] {
			return t.Name
		}
	}
	return ""
}

// InstallClients connects to each upstream MCP server and registers its tools
// as namespaced mcpshell commands.
//
// An upstream is skipped — preventing a recursive `eval` loop — when either it
// identifies itself as "mcpshell" in its initialize response, or it exposes a
// tool named eval/help/prompt (mcpshell's own tool names). This makes it safe to
// point mcpshell at a server set that happens to include a mcpshell instance.
func InstallClients(sh *runtime.Shell, servers []NamedServer) []io.Closer {
	closers, _ := ConnectClients(sh, servers)
	return closers
}

// HostedServer reports what hosting one upstream saves: SchemaBytes is the
// JSON of its tools/list entries (name, description, inputSchema per tool)
// that a direct MCP client would put in front of the model, HelpBytes what
// help("<ns>") returns when the model asks.
type HostedServer struct {
	NS          string `json:"ns"`
	Tools       int    `json:"tools"`
	SchemaBytes int    `json:"schemaBytes"`
	HelpBytes   int    `json:"helpBytes"`
}

// ConnectClients is InstallClients that also reports each connected upstream.
func ConnectClients(sh *runtime.Shell, servers []NamedServer) ([]io.Closer, []HostedServer) {
	var closers []io.Closer
	var hosted []HostedServer
	for _, ns := range servers {
		client, info, err := dial(ns.Config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mcpshell mcp: failed to connect upstream %q: %v\n", ns.Namespace, err)
			continue
		}
		if strings.EqualFold(info.Name, "mcpshell") {
			fmt.Fprintf(os.Stderr,
				"mcpshell mcp: skipping upstream %q — it identifies as mcpshell; "+
					"wiring it in would let eval recurse into itself\n", ns.Namespace)
			_ = client.Close()
			continue
		}
		tools, err := client.ListTools()
		if err != nil {
			fmt.Fprintf(os.Stderr, "mcpshell mcp: upstream %q tools/list failed: %v\n", ns.Namespace, err)
			_ = client.Close()
			continue
		}
		if collide := reservedCollision(tools); collide != "" {
			fmt.Fprintf(os.Stderr,
				"mcpshell mcp: skipping upstream %q — it exposes a %q tool (a reserved "+
					"mcpshell tool name); wiring it in risks recursive eval\n", ns.Namespace, collide)
			_ = client.Close()
			continue
		}
		n := registerServerTools(sh, ns, client, tools)
		h := HostedServer{NS: ns.Namespace, Tools: n, SchemaBytes: schemaBytes(tools),
			HelpBytes: len(sh.Commands().Help(ns.Namespace))}
		fmt.Fprintf(os.Stderr, "mcpshell mcp: upstream %q connected (%s) — %d tool(s), schema %d bytes, help %d bytes\n",
			ns.Namespace, info.Name, n, h.SchemaBytes, h.HelpBytes)
		closers = append(closers, client)
		hosted = append(hosted, h)
	}
	return closers, hosted
}

// schemaBytes is the summed JSON size of each tool's tools/list entry.
func schemaBytes(tools []ToolInfo) int {
	total := 0
	for _, t := range tools {
		b, _ := json.Marshal(t)
		total += len(b)
	}
	return total
}

// dial connects over HTTP when cfg.URL is set, otherwise spawns cfg.Command.
func dial(cfg ServerConfig) (Upstream, ServerInfo, error) {
	if cfg.URL != "" {
		return DialHTTP(cfg.URL, cfg.Headers, cfg.AuthCmd)
	}
	return Dial(cfg.Command, cfg.Env)
}

func registerServerTools(sh *runtime.Shell, ns NamedServer, client Upstream, tools []ToolInfo) int {
	cats := categorize(tools, ns.Config.Categories)
	var summaries []string
	for _, tool := range tools {
		toolName := tool.Name
		params, signature := schemaInfo(tool.InputSchema)
		closed := schemaClosed(tool.InputSchema)
		required := schemaRequired(tool.InputSchema)
		types := schemaTypes(tool.InputSchema)
		unchecked := matchesAny(toolName, ns.Config.Unchecked)
		callName := runtime.MemberRef(ns.Namespace, toolName)
		detail := schemaDetail(tool.InputSchema)
		desc := tool.Description
		if desc == "" {
			desc = toolName
		}
		summaries = append(summaries, "  "+runtime.MemberRef(ns.Namespace, toolName)+"("+signature+") — "+runtime.Brief(desc))
		sh.Register(&runtime.CommandDef{
			Namespace:   ns.Namespace,
			Name:        toolName,
			Signature:   signature,
			Description: desc,
			Category:    cats[toolName],
			Params:      detail,
			Fn: func(args []runtime.Value) runtime.Value {
				defer withParams(callName, detail)
				jsonArgs := argsToJSONMap(args, params)
				if !unchecked {
					checkArgs(callName, ns.Namespace, toolName, jsonArgs, params, closed)
					checkRequired(callName, ns.Namespace, toolName, jsonArgs, required)
					coerceArgs(callName, jsonArgs, types)
				}
				text, isError, err := client.CallTool(toolName, jsonArgs)
				if err != nil {
					panic(toolError(callName, jsonArgs, err.Error()))
				}
				if isError {
					panic(toolError(callName, jsonArgs, text))
				}
				return textToValue(text)
			},
		})
	}

	hasRaw := registerRaw(sh, ns.Namespace, client, tools)

	label := ns.Config.Label
	if label == "" {
		label = ns.Namespace
	}
	var g strings.Builder
	source := ns.Config.URL
	if source == "" {
		source = strings.Join(ns.Config.Command, " ")
	}
	g.WriteString(label + " — MCP server (" + source + ")\n")
	if ns.Config.Description != "" {
		g.WriteString("\n" + ns.Config.Description + "\n")
	}
	if len(cats) > 0 {
		writeCategories(&g, ns.Namespace, tools, cats)
	} else {
		g.WriteString("\nTools:\n")
		for _, s := range summaries {
			g.WriteString(s + "\n")
		}
	}
	if len(cats) > 0 {
		g.WriteString("\nhelp(\"" + ns.Namespace + ".<category>\") lists every tool in a category.")
	}
	g.WriteString("\nhelp(\"<word>\") searches tool names and descriptions, e.g. help(\"alert\").")
	g.WriteString("\ncommands(\"" + ns.Namespace + "\") returns the same list as data: [{name, call, category, signature, description}].")
	if hasRaw {
		g.WriteString("\n" + ns.Namespace + ".raw(\"toolName\", {args}) returns the tool's output as unparsed text — " +
			"write(path, " + ns.Namespace + ".raw(…)) saves it byte-exact without spending output.")
	}
	g.WriteString("\nUse help(\"" + ns.Namespace + ".toolName\") for full details on any tool.")
	sh.RegisterGuide(ns.Namespace, g.String())
	return len(tools)
}

// registerRaw adds ns.raw(tool, args?), which returns a tool's text output
// unparsed, so it can be written to a file byte-exact. Skipped when the
// upstream has its own tool named raw.
func registerRaw(sh *runtime.Shell, namespace string, client Upstream, tools []ToolInfo) bool {
	known := make(map[string]bool, len(tools))
	for _, t := range tools {
		known[t.Name] = true
	}
	if known["raw"] {
		return false
	}
	sh.Register(&runtime.CommandDef{
		Namespace: namespace,
		Name:      "raw",
		Signature: "tool: string, args?: object",
		Description: "calls an upstream tool and returns its text output unparsed (byte-exact), " +
			"e.g. for write(path, " + namespace + ".raw(\"tool\", {…}))",
		Fn: func(args []runtime.Value) runtime.Value {
			var name *runtime.StringVal
			if len(args) > 0 {
				name, _ = args[0].(*runtime.StringVal)
			}
			if name == nil {
				panic(runtime.Runtime(namespace + ".raw: first argument must be a tool name string"))
			}
			if !known[name.V] {
				panic(runtime.Runtime(namespace + ".raw: no tool '" + name.V + "' — help(\"" + namespace + "\") lists tools"))
			}
			jsonArgs := argsToJSONMap(args[1:], nil)
			callName := namespace + ".raw(\"" + name.V + "\")"
			text, isError, err := client.CallTool(name.V, jsonArgs)
			if err != nil {
				panic(toolError(callName, jsonArgs, err.Error()))
			}
			if isError {
				panic(toolError(callName, jsonArgs, text))
			}
			return runtime.Str(text)
		},
	})
	return true
}

// otherCategory holds tools no glob matched, or whose name prefix is unique.
const otherCategory = "other"

// categorize maps tool name → category. With explicit globs, a tool takes the
// first matching category (by sorted name), else "other". Without, a server
// over runtime.CollapseNamespaceAt tools is grouped by name prefix (the part
// before the first '_', '.' or '-'), singletons going to "other". It returns
// nil when grouping does not apply or yields fewer than two categories.
func categorize(tools []ToolInfo, globs map[string][]string) map[string]string {
	out := make(map[string]string, len(tools))
	if len(globs) > 0 {
		names := make([]string, 0, len(globs))
		for name := range globs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, t := range tools {
			out[t.Name] = otherCategory
		find:
			for _, cat := range names {
				for _, g := range globs[cat] {
					if ok, _ := path.Match(g, t.Name); ok {
						out[t.Name] = cat
						break find
					}
				}
			}
		}
	} else {
		if len(tools) <= runtime.CollapseNamespaceAt {
			return nil
		}
		size := make(map[string]int)
		for _, t := range tools {
			p := t.Name
			if i := strings.IndexAny(p, "_.-"); i > 0 {
				p = p[:i]
			}
			out[t.Name] = p
			size[p]++
		}
		for name, p := range out {
			if size[p] == 1 {
				out[name] = otherCategory
			}
		}
	}

	distinct := make(map[string]bool)
	for _, c := range out {
		distinct[c] = true
	}
	if len(distinct) < 2 {
		return nil
	}
	// A category named like a tool would hide that tool's own help("ns.name").
	for _, t := range tools {
		if distinct[t.Name] {
			for name, c := range out {
				if c == t.Name {
					out[name] = c + "_group"
				}
			}
		}
	}
	return out
}

// writeCategories lists each category with its size and a few tool names.
func writeCategories(g *strings.Builder, namespace string, tools []ToolInfo, cats map[string]string) {
	members := make(map[string][]string)
	for _, t := range tools {
		members[cats[t.Name]] = append(members[cats[t.Name]], t.Name)
	}
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(g, "\n%d tools in %d categories:\n", len(tools), len(names))
	for _, cat := range names {
		m := members[cat]
		sort.Strings(m)
		sample := m
		if len(sample) > 3 {
			sample = append(append([]string(nil), m[:3]...), "…")
		}
		fmt.Fprintf(g, "  %s (%d): %s — help(\"%s.%s\")\n", cat, len(m), strings.Join(sample, ", "), namespace, cat)
	}
}

// textToValue parses tool output as JSON when it is valid JSON, else returns it
// as a string. Numbers stay exact and object keys keep their order.
func textToValue(text string) runtime.Value {
	if json.Valid([]byte(text)) {
		return toolkit.ParseJSON(text)
	}
	return runtime.Str(text)
}

// argsToJSONMap maps mcpshell call arguments to a JSON arguments object: a single
// object argument supplies named fields, otherwise args map positionally to the
// tool's parameter names.
func argsToJSONMap(args []runtime.Value, paramNames []string) map[string]any {
	if len(args) == 1 {
		if obj, ok := args[0].(*runtime.ObjectVal); ok {
			out := make(map[string]any, obj.Len())
			for _, k := range obj.Keys() {
				v, _ := obj.Get(k)
				out[k] = valueToJSON(v)
			}
			return out
		}
	}
	out := make(map[string]any)
	for i, name := range paramNames {
		if i < len(args) {
			out[name] = valueToJSON(args[i])
		}
	}
	return out
}

// valueToJSON converts a mcpshell value to a JSON-encodable Go value.
func valueToJSON(v runtime.Value) any {
	switch x := v.(type) {
	case *runtime.NullVal:
		return nil
	case *runtime.BoolVal:
		return x.V
	case *runtime.NumberVal:
		if math.IsInf(x.V, 0) || math.IsNaN(x.V) {
			return nil
		}
		return json.Number(x.Display()) // exact: big integers and long decimals keep every digit
	case *runtime.StringVal:
		return x.V
	case *runtime.ArrayVal:
		out := make([]any, len(x.Elements))
		for i, e := range x.Elements {
			out[i] = valueToJSON(e)
		}
		return out
	case *runtime.ObjectVal:
		out := make(map[string]any, x.Len())
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			out[k] = valueToJSON(e)
		}
		return out
	default:
		return v.Display()
	}
}

// schemaClosed reports whether a tool's input schema sets
// additionalProperties: false, so the server rejects undeclared arguments.
func schemaClosed(schema json.RawMessage) bool {
	var s struct {
		AdditionalProperties *bool `json:"additionalProperties"`
	}
	return json.Unmarshal(schema, &s) == nil && s.AdditionalProperties != nil && !*s.AdditionalProperties
}

// checkArgs fails a call before it goes upstream when an argument name is
// wrong. A closed schema rejects every undeclared name. An open schema lets
// undeclared names through — servers accept hidden options such as a confirm
// nonce — unless the name is a likely mistake for a declared one: the same
// name in another case style, or one edit away. ns.raw skips this check.
func checkArgs(callName, namespace, tool string, args map[string]any, params []string, closed bool) {
	declared := make(map[string]bool, len(params))
	for _, p := range params {
		declared[p] = true
	}
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if declared[name] {
			continue
		}
		near := likelyMeant(name, params)
		if near == "" && !closed {
			continue
		}
		msg := callName + ": unknown argument '" + name + "'"
		if near != "" {
			msg += " — did you mean '" + near + "'?"
		}
		if len(params) > 0 {
			msg += "\n\n  Valid arguments: " + strings.Join(params, ", ")
		} else {
			msg += "\n\n  " + callName + " takes no arguments"
		}
		if !closed {
			msg += "\n\n  If '" + name + "' is a hidden option, call " + namespace +
				".raw(\"" + tool + "\", {…}), which skips this check."
		}
		panic(runtime.Runtime(msg))
	}
}

// matchesAny reports whether name matches one of the globs.
func matchesAny(name string, globs []string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}

// checkRequired fails a call before it goes upstream when required arguments
// are missing, and names all of them at once. ns.raw skips this check.
func checkRequired(callName, namespace, tool string, args map[string]any, required []string) {
	var missing []string
	for _, r := range required {
		if _, ok := args[r]; !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return
	}
	noun := "argument"
	if len(missing) > 1 {
		noun = "arguments"
	}
	panic(runtime.Runtime(callName + ": missing required " + noun + ": " + strings.Join(missing, ", ") +
		"\n\n  help(\"" + namespace + "." + tool + "\") shows every argument"))
}

// withParams adds the tool's parameter detail to an error raised by a call to
// it — a failed check or an upstream failure — so the next call can be fixed
// without a help() turn. Deferred in each tool's Fn.
func withParams(callName, detail string) {
	r := recover()
	if r == nil {
		return
	}
	if se, ok := r.(*runtime.ShellError); ok && detail != "" && !strings.Contains(se.Message, "  parameters:") {
		panic(runtime.Runtime(se.Message + "\n\n  " + callName + " parameters:\n" + detail))
	}
	panic(r)
}

// schemaDetail renders each property of an input schema on its own line:
// name, type or enum values, required or default, and description.
//
//	status?: "paid" | "open" | "overdue" — payment status
//	page_size?: integer, default 20 — rows per page, 1-25
func schemaDetail(schema json.RawMessage) string {
	var s struct {
		Properties json.RawMessage `json:"properties"`
		Required   []string        `json:"required"`
	}
	if json.Unmarshal(schema, &s) != nil || len(s.Properties) == 0 {
		return ""
	}
	var props map[string]struct {
		Type        json.RawMessage   `json:"type"`
		Enum        []json.RawMessage `json:"enum"`
		Default     json.RawMessage   `json:"default"`
		Description string            `json:"description"`
		Items       struct {
			Type string `json:"type"`
		} `json:"items"`
	}
	if json.Unmarshal(s.Properties, &props) != nil {
		return ""
	}
	required := map[string]bool{}
	for _, r := range s.Required {
		required[r] = true
	}
	var lines []string
	for _, name := range orderedKeys(s.Properties) {
		p := props[name]
		typ := strings.Trim(string(p.Type), `"`)
		if strings.HasPrefix(typ, "[") { // ["string","null"]
			var many []string
			_ = json.Unmarshal(p.Type, &many)
			typ = strings.Join(many, " | ")
		}
		if typ == "array" && p.Items.Type != "" {
			typ = p.Items.Type + "[]"
		}
		if len(p.Enum) > 0 {
			vals := make([]string, len(p.Enum))
			for i, e := range p.Enum {
				vals[i] = string(e)
			}
			typ = strings.Join(vals, " | ")
		}
		if typ == "" {
			typ = "any"
		}
		line := "    " + name
		if !required[name] {
			line += "?"
		}
		line += ": " + typ
		if len(p.Default) > 0 {
			line += ", default " + string(p.Default)
		}
		if d := strings.TrimSpace(p.Description); d != "" {
			if i := strings.IndexByte(d, '\n'); i >= 0 {
				d = d[:i]
			}
			if r := []rune(d); len(r) > 160 {
				d = string(r[:160]) + "…"
			}
			line += " — " + d
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// toolError builds the error for a failed upstream call: the tool, the
// arguments sent, and the upstream message.
func toolError(callName string, args map[string]any, text string) *runtime.ShellError {
	if strings.TrimSpace(text) == "" {
		text = "(no message)"
	}
	sent, _ := json.Marshal(args)
	if len(sent) > 500 {
		sent = append(sent[:500], "…"...)
	}
	return runtime.Runtime(callName + " failed: " + text + "\n\n  arguments sent: " + string(sent))
}

// likelyMeant returns the declared parameter that name is probably a mistake
// for: equal after case/separator normalization, or one edit away when both
// are at least 5 characters. Returns "" when there is none.
func likelyMeant(name string, params []string) string {
	n := runtime.NormalizeName(name)
	for _, p := range params {
		np := runtime.NormalizeName(p)
		if np == n || len(n) >= 5 && len(np) >= 5 && runtime.EditDistance(n, np) == 1 {
			return p
		}
	}
	return ""
}

// schemaRequired returns the required argument names of a tool's input schema.
func schemaRequired(schema json.RawMessage) []string {
	var s struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(schema, &s)
	return s.Required
}

// schemaInfo extracts ordered parameter names and a display signature from a
// tool's JSON Schema.
func schemaInfo(schema json.RawMessage) (params []string, signature string) {
	var s struct {
		Properties json.RawMessage `json:"properties"`
		Required   []string        `json:"required"`
	}
	if json.Unmarshal(schema, &s) != nil || len(s.Properties) == 0 {
		return nil, ""
	}
	params = orderedKeys(s.Properties)
	required := make(map[string]bool, len(s.Required))
	for _, r := range s.Required {
		required[r] = true
	}
	var props map[string]struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(s.Properties, &props)

	parts := make([]string, len(params))
	for i, p := range params {
		typ := props[p].Type
		if typ == "" {
			typ = "any"
		}
		opt := "?"
		if required[p] {
			opt = ""
		}
		parts[i] = p + opt + ": " + typ
	}
	return params, strings.Join(parts, ", ")
}

// orderedKeys returns the keys of a JSON object in declaration order.
func orderedKeys(obj json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(obj))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := kt.(string)
		keys = append(keys, key)
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			break
		}
	}
	return keys
}
