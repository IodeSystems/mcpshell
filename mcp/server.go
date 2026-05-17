// Package mcp exposes a mcpshell shell as a Model Context Protocol server over
// stdio. The protocol is line-delimited JSON-RPC 2.0; this implementation is
// self-contained (no SDK dependency).
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"github.com/iodesystems/mcpshell/runtime"
)

// protocolVersion is the MCP revision this server advertises.
const protocolVersion = "2025-06-18"

// Server adapts a runtime.Shell into an MCP stdio server.
type Server struct {
	shell          *runtime.Shell
	maxOutputBytes int
}

// NewServer builds an MCP server. maxOutputBytes caps the eval tool's textual
// output (0 selects the 16 KB default).
func NewServer(shell *runtime.Shell, maxOutputBytes int) *Server {
	if maxOutputBytes <= 0 {
		maxOutputBytes = 16_000
	}
	return &Server{shell: shell, maxOutputBytes: maxOutputBytes}
}

// --- JSON-RPC framing --------------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func result(id json.RawMessage, r any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: r}
}

func errorResp(id json.RawMessage, code int, msg string) *rpcResponse {
	if id == nil {
		id = json.RawMessage("null")
	}
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// RunStdio serves MCP requests from in, writing responses to out. It returns
// when in reaches EOF.
func (s *Server) RunStdio(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(errorResp(nil, -32700, "parse error: "+err.Error()))
			continue
		}
		resp := s.handle(&req)
		if resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

// handle processes one message, returning the response or nil for notifications.
func (s *Server) handle(req *rpcRequest) *rpcResponse {
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		return result(req.ID, s.initializeResult(req.Params))
	case "notifications/initialized", "notifications/cancelled":
		return nil
	case "ping":
		return result(req.ID, struct{}{})
	case "tools/list":
		return result(req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		return result(req.ID, s.callTool(req.Params))
	default:
		if isNotification {
			return nil
		}
		return errorResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) initializeResult(params json.RawMessage) map[string]any {
	version := protocolVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		version = p.ProtocolVersion // echo the client's requested revision
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "mcpshell", "version": "0.1.0"},
	}
}

// --- Tools -------------------------------------------------------------------

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func toolDefs() []toolDef {
	return []toolDef{
		{
			Name:        "eval",
			Description: runtime.ToolDescription,
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"code":{"type":"string","description":"mcpshell source code"},` +
				`"vars":{"type":"object","description":"RECOMMENDED for file paths, regex patterns, and user data. Bound as constants before execution — avoids double-escaping errors.","additionalProperties":true}` +
				`},"required":["code"]}`),
		},
		{
			Name:        "help",
			Description: "List available mcpshell commands or get detailed help for a specific command",
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"search":{"type":"string","description":"command name to search for"}}}`),
		},
		{
			Name:        "prompt",
			Description: "Get the mcpshell language reference. Default: compact (names only, use help() for details). detail=true for full signatures.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"detail":{"type":"boolean","description":"true for full signatures; false (default) for a compact listing"}}}`),
		},
	}
}

// toolResult builds an MCP tools/call result with a single text block.
func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func (s *Server) callTool(params json.RawMessage) map[string]any {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolResult("ERROR: invalid tool-call params: "+err.Error(), true)
	}
	switch p.Name {
	case "eval":
		return s.evalTool(p.Arguments)
	case "help":
		search, _ := p.Arguments["search"].(string)
		return toolResult(s.shell.Commands().Help(search), false)
	case "prompt":
		detail, _ := p.Arguments["detail"].(bool)
		return toolResult(s.shell.ToPrompt(!detail), false)
	default:
		return toolResult("ERROR: unknown tool '"+p.Name+"'", true)
	}
}

func (s *Server) evalTool(args map[string]any) map[string]any {
	code, ok := args["code"].(string)
	if !ok {
		return toolResult("ERROR: missing 'code' argument", true)
	}
	vars := map[string]runtime.Value{}
	if rawVars, ok := args["vars"].(map[string]any); ok {
		for k, v := range rawVars {
			vars[k] = goToValue(v)
		}
	}
	value, err := s.shell.EvalExported(code, vars)
	if err != nil {
		return toolResult("ERROR: "+err.Error(), true)
	}
	out := value.Display()
	if len(out) > s.maxOutputBytes {
		out = out[:s.maxOutputBytes] +
			"\n\n... OUTPUT TRUNCATED (limit " + strconv.Itoa(s.maxOutputBytes) + " bytes). " +
			"Use limit(), filter(), or read(path, start, lines) to reduce output."
	}
	return toolResult(out, false)
}

// goToValue converts a decoded JSON value (from `vars`) into a mcpshell value.
func goToValue(v any) runtime.Value {
	switch x := v.(type) {
	case nil:
		return runtime.Null
	case bool:
		return runtime.Bln(x)
	case float64:
		return runtime.Num(x)
	case string:
		return runtime.Str(x)
	case []any:
		elems := make([]runtime.Value, len(x))
		for i, e := range x {
			elems[i] = goToValue(e)
		}
		return &runtime.ArrayVal{Elements: elems}
	case map[string]any:
		obj := runtime.NewObject()
		for k, e := range x {
			obj.Set(k, goToValue(e))
		}
		return obj
	default:
		return runtime.Null
	}
}
