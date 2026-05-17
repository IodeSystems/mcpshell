package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iodesystems/mcpshell/runtime"
)

// ServerConfig describes an upstream MCP server to connect to.
type ServerConfig struct {
	Command     []string
	Env         map[string]string
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
	var closers []io.Closer
	for _, ns := range servers {
		client, info, err := Dial(ns.Config.Command, ns.Config.Env)
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
		fmt.Fprintf(os.Stderr, "mcpshell mcp: upstream %q connected (%s) — %d tool(s)\n",
			ns.Namespace, info.Name, n)
		closers = append(closers, client)
	}
	return closers
}

func registerServerTools(sh *runtime.Shell, ns NamedServer, client *Client, tools []ToolInfo) int {
	var summaries []string
	for _, tool := range tools {
		toolName := tool.Name
		params, signature := schemaInfo(tool.InputSchema)
		desc := tool.Description
		if desc == "" {
			desc = toolName
		}
		summaries = append(summaries, "  "+ns.Namespace+"."+toolName+"("+signature+") — "+brief(desc))
		sh.Register(&runtime.CommandDef{
			Namespace:   ns.Namespace,
			Name:        toolName,
			Signature:   signature,
			Description: desc,
			Fn: func(args []runtime.Value) runtime.Value {
				text, isError, err := client.CallTool(toolName, argsToJSONMap(args, params))
				if err != nil {
					panic(runtime.Runtime("MCP tool error: " + err.Error()))
				}
				if isError {
					panic(runtime.Runtime("MCP tool error: " + text))
				}
				return textToValue(text)
			},
		})
	}

	label := ns.Config.Label
	if label == "" {
		label = ns.Namespace
	}
	var g strings.Builder
	g.WriteString(label + " — MCP server (" + strings.Join(ns.Config.Command, " ") + ")\n")
	if ns.Config.Description != "" {
		g.WriteString("\n" + ns.Config.Description + "\n")
	}
	g.WriteString("\nTools:\n")
	for _, s := range summaries {
		g.WriteString(s + "\n")
	}
	g.WriteString("\nUse help(\"" + ns.Namespace + ".toolName\") for full details on any tool.")
	sh.RegisterGuide(ns.Namespace, g.String())
	return len(tools)
}

// brief shortens a description to its first sentence, capped at 80 runes.
func brief(desc string) string {
	s := desc
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// textToValue parses tool output as JSON when possible, else returns it as a string.
func textToValue(text string) runtime.Value {
	var decoded any
	if json.Unmarshal([]byte(text), &decoded) == nil {
		return goToValue(decoded)
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
		return x.V
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
