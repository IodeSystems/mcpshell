package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Version is reported as serverInfo.version; the CLI sets it from build info.
var Version = "dev"

// protocolVersion is the MCP revision this server advertises.
const protocolVersion = "2025-06-18"

// Server adapts a runtime.Shell into an MCP stdio server.
type Server struct {
	shell          *runtime.Shell
	maxOutputBytes int
	overflowDir    string // where over-limit eval output is saved; "" disables
	hosted         []HostedServer
	descOnce       sync.Once
	descFixed      bool // the eval description is built; it does not change after
	toolDesc       string

	// setRoots receives the client's file roots; nil ignores client roots.
	setRoots    func([]toolkit.FileRoot)
	clientRoots atomic.Bool // the client advertised the roots capability

	// Host functions: hostAllowed is --host-functions; the client declares
	// the functions in initialize (see host.go).
	hostAllowed bool
	hostInit    bool   // the first initialize was handled; later ones change nothing
	hostLine    string // the eval description's Host line, "" without Host
	hostMu      sync.Mutex
	evalIDMu    sync.Mutex
	evalID      json.RawMessage // the running eval's JSON-RPC id, raw; nil without one (EvalText)

	// commands are the operator's declared commands (commands.go); nil
	// without --commands.
	commands *Commands

	// toolkitLines are the eval description's lines for in-process
	// toolkits (AddToolkit, functions.go).
	toolkitLines []string

	// RunStdio state: one writer, the server's own requests to the client.
	writeMu      sync.Mutex
	enc          *json.Encoder
	pendMu       sync.Mutex
	pending      map[int64]chan *rpcRequest // request id → its response
	nextID       int64
	rootsMu      sync.Mutex
	rootsApplied int64 // id of the roots/list whose answer is the current set
	done         chan struct{}
	bg           sync.WaitGroup
}

// rootsTimeout caps the wait for the client's roots/list answer. A var so
// tests can shorten it.
var rootsTimeout = 5 * time.Second

// serverLog receives the server's notes (client roots); tests replace it.
var serverLog io.Writer = os.Stderr

// NewServer builds an MCP server. maxOutputBytes caps the eval tool's textual
// output (0 selects the 16 KB default).
func NewServer(shell *runtime.Shell, maxOutputBytes int) *Server {
	if maxOutputBytes <= 0 {
		maxOutputBytes = 16_000
	}
	shell.Limits().Hint = runtime.HintEvalArg
	shell.Limits().GCTarget = true
	return &Server{shell: shell, maxOutputBytes: maxOutputBytes}
}

// WithOverflowDir saves each eval output over the limit to a new file in dir
// and points the truncated response at it. dir is created if missing.
func (s *Server) WithOverflowDir(dir string) *Server {
	s.overflowDir = dir
	return s
}

// WithHosted reports the hosted upstreams to the client once it is
// initialized, as a notifications/message with logger "mcpshell/hosted" and
// data {"servers": [HostedServer…]}.
func (s *Server) WithHosted(hosted []HostedServer) *Server {
	s.hosted = hosted
	return s
}

// WithClientRoots asks a client that has the MCP roots capability for its
// roots after initialization and on each notifications/roots/list_changed,
// and passes each answer to set as one set. A root is read-only unless its
// _meta has {"mcpshell/access": "write"}; only file:// URIs are used.
func (s *Server) WithClientRoots(set func([]toolkit.FileRoot)) *Server {
	s.setRoots = set
	return s
}

// hostedNotice is the notifications/message that reports s.hosted.
func (s *Server) hostedNotice() map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/message",
		"params": map[string]any{
			"level":  "info",
			"logger": "mcpshell/hosted",
			"data":   map[string]any{"servers": s.hosted},
		},
	}
}

// --- JSON-RPC framing --------------------------------------------------------

// rpcRequest is a message from the client: a request, a notification, or a
// response (no method) to a request of the server's.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
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
//
// One goroutine reads; one worker handles the client's requests in order,
// so evals still run one at a time. The reader routes responses to the
// server's own requests (roots/list) and roots/list_changed itself, so they
// are handled while an eval runs.
func (s *Server) RunStdio(in io.Reader, out io.Writer) error {
	s.enc = json.NewEncoder(out)
	s.pending = map[int64]chan *rpcRequest{}
	s.done = make(chan struct{})
	queue := make(chan *rpcRequest, 64)
	workerDone := make(chan struct{})
	var werr error
	go func() {
		defer close(workerDone)
		for req := range queue {
			if werr = s.serve(req); werr != nil {
				return
			}
		}
	}()

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
read:
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		req := &rpcRequest{}
		if err := json.Unmarshal(line, req); err != nil {
			_ = s.write(errorResp(nil, -32700, "parse error: "+err.Error()))
			continue
		}
		switch {
		case req.Method == "" && len(req.ID) > 0:
			s.deliver(req)
			continue
		case req.Method == "notifications/roots/list_changed":
			s.refreshRoots()
			continue
		}
		select {
		case queue <- req:
		case <-workerDone:
			break read
		}
	}
	rerr := scanner.Err()
	close(queue)
	// done before the worker ends: an eval waiting on a host call (on the
	// worker) must see the input end, or the worker never returns.
	close(s.done)
	<-workerDone
	s.bg.Wait()
	if werr != nil {
		return werr
	}
	return rerr
}

// serve handles one client request or notification and writes what it sends back.
func (s *Server) serve(req *rpcRequest) error {
	if resp := s.handle(req); resp != nil {
		if err := s.write(resp); err != nil {
			return err
		}
	}
	if req.Method == "notifications/initialized" {
		if len(s.hosted) > 0 {
			if err := s.write(s.hostedNotice()); err != nil {
				return err
			}
		}
		s.refreshRoots()
	}
	return nil
}

// write sends one message; the worker and the roots requests share out.
func (s *Server) write(v any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.enc.Encode(v)
}

// --- Requests to the client --------------------------------------------------

// clientError is a JSON-RPC error response from the client to a request
// of the server's.
type clientError struct {
	Code    int
	Message string
}

func (e *clientError) Error() string { return fmt.Sprintf("error %d: %s", e.Code, e.Message) }

// errConnClosed is request's error when the client's input ended.
var errConnClosed = errors.New("connection closed")

// request sends a request to the client and waits for its result until ctx
// is done (the error is then context.Cause(ctx)) or the input ends
// (errConnClosed). params nil sends none. A JSON-RPC error answer is a
// *clientError. It runs off the read loop, which delivers the response; id
// is the request's id, for a notifications/cancelled.
func (s *Server) request(ctx context.Context, method string, params any) (id int64, res json.RawMessage, err error) {
	ch := make(chan *rpcRequest, 1)
	s.pendMu.Lock()
	s.nextID++
	id = s.nextID
	s.pending[id] = ch
	s.pendMu.Unlock()
	defer func() {
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
	}()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := s.write(msg); err != nil {
		return id, nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return id, nil, &clientError{Code: m.Error.Code, Message: m.Error.Message}
		}
		return id, m.Result, nil
	case <-ctx.Done():
		return id, nil, context.Cause(ctx)
	case <-s.done:
		return id, nil, errConnClosed
	}
}

// deliver passes a client response to the request waiting for it. A
// response nobody waits for (timed out, unknown id) is dropped.
func (s *Server) deliver(m *rpcRequest) {
	var id int64
	if json.Unmarshal(m.ID, &id) != nil {
		return
	}
	s.pendMu.Lock()
	ch := s.pending[id]
	s.pendMu.Unlock()
	if ch != nil {
		select {
		case ch <- m:
		default:
		}
	}
}

// refreshRoots asks the client for its roots in the background, when client
// roots are enabled and the client has the capability.
func (s *Server) refreshRoots() {
	if s.setRoots == nil || !s.clientRoots.Load() {
		return
	}
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ctx, cancel := context.WithTimeoutCause(context.Background(), rootsTimeout, fmt.Errorf("no answer in %s", rootsTimeout))
		defer cancel()
		id, res, err := s.request(ctx, "roots/list", nil)
		if err != nil {
			fmt.Fprintf(serverLog, "mcpshell mcp: roots/list: %v; keeping the previous client roots\n", err)
			return
		}
		roots, err := parseRoots(res)
		if err != nil {
			fmt.Fprintf(serverLog, "mcpshell mcp: roots/list: %v; keeping the previous client roots\n", err)
			return
		}
		s.rootsMu.Lock()
		defer s.rootsMu.Unlock()
		if id < s.rootsApplied { // a later request's answer is already in place
			return
		}
		s.rootsApplied = id
		s.setRoots(roots)
		desc := make([]string, len(roots))
		for i, r := range roots {
			mode := "read-only"
			if r.Write {
				mode = "read-write"
			}
			desc[i] = r.Path + " (" + mode + ")"
		}
		if len(desc) == 0 {
			desc = []string{"none"}
		}
		fmt.Fprintf(serverLog, "mcpshell mcp: client file roots: %s\n", strings.Join(desc, ", "))
	}()
}

// rootsWriteMeta is the _meta key and value that make a client root writable.
const rootsWriteMeta = "mcpshell/access"

// parseRoots reads a roots/list result. A root that is not a local file://
// URI is left out, with a note on serverLog.
func parseRoots(res json.RawMessage) ([]toolkit.FileRoot, error) {
	var r struct {
		Roots []struct {
			URI  string         `json:"uri"`
			Meta map[string]any `json:"_meta"`
		} `json:"roots"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, fmt.Errorf("bad result: %v", err)
	}
	out := []toolkit.FileRoot{}
	for _, e := range r.Roots {
		u, err := url.Parse(e.URI)
		switch {
		case err != nil || u.Scheme != "file":
			fmt.Fprintf(serverLog, "mcpshell mcp: client root %q ignored: not a file:// URI\n", e.URI)
			continue
		case u.Host != "" && u.Host != "localhost":
			fmt.Fprintf(serverLog, "mcpshell mcp: client root %q ignored: not on this host\n", e.URI)
			continue
		}
		p := filepath.FromSlash(u.Path)
		if !filepath.IsAbs(p) {
			fmt.Fprintf(serverLog, "mcpshell mcp: client root %q ignored: not an absolute path\n", e.URI)
			continue
		}
		out = append(out, toolkit.FileRoot{Path: filepath.Clean(p), Write: e.Meta[rootsWriteMeta] == "write"})
	}
	return out, nil
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
		return result(req.ID, map[string]any{"tools": s.toolDefs()})
	case "tools/call":
		return result(req.ID, s.callTool(req.ID, req.Params))
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
		Capabilities    struct {
			Roots        json.RawMessage            `json:"roots"`
			Experimental map[string]json.RawMessage `json:"experimental"`
		} `json:"capabilities"`
	}
	if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		version = p.ProtocolVersion // echo the client's requested revision
	}
	r := p.Capabilities.Roots
	s.clientRoots.Store(len(r) > 0 && string(r) != "null")
	if !s.hostInit {
		s.hostInit = true
		s.initHost(p.Capabilities.Experimental[hostCapability])
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}, "logging": map[string]any{}},
		"serverInfo":      map[string]any{"name": "mcpshell", "version": Version},
	}
}

// --- Tools -------------------------------------------------------------------

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// evalDescription is the eval tool description: ToolDescription plus the
// Toolkits line (the names of the allowed loadable toolkits, loaded or not;
// help() has their summaries). It is fixed
// at the first tools/list, so a load never changes it. Clients that show the
// model only this description learn from it which toolkits it can reference.
func (s *Server) evalDescription() string {
	s.descOnce.Do(func() {
		s.descFixed = true
		s.toolDesc = runtime.ToolDescription
		if line := s.shell.Commands().ToolkitsLine(); line != "" {
			s.toolDesc += "\n" + line
		}
		if s.commands != nil {
			s.toolDesc += "\n" + s.commands.line
		}
		for _, line := range s.toolkitLines {
			s.toolDesc += "\n" + line
		}
		if s.hostLine != "" {
			s.toolDesc += "\n" + s.hostLine
		}
	})
	return s.toolDesc
}

func (s *Server) toolDefs() []toolDef {
	return []toolDef{
		{
			Name:        "eval",
			Description: s.evalDescription(),
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"code":{"type":"string","description":"mcpshell source code; top-level let/const/function persist to later calls"},` +
				`"vars":{"type":"string","description":"JSON object encoded as a string, e.g. {\"path\":\"a.txt\"}. RECOMMENDED for file paths, regex patterns, and user data. Bound as constants before execution — avoids double-escaping errors."},` +
				`"profile":{"type":"boolean","description":"Add per-line step costs to the result."},` +
				`"limits":{"type":"object","description":"Limits for this eval only; a limit-hit error names the value to use. timeout in ms.","properties":{"steps":{"type":"integer"},"timeout":{"type":"integer"},"callDepth":{"type":"integer"},"outputBytes":{"type":"integer"},"memory":{"type":"string","description":"a size such as \"64MB\" or \"2GB\", or a plain byte count such as \"1048576\""}}}` +
				`,"clear":{"type":"boolean","description":"Reset the session before code (code is optional then): unload toolkits, drop globals and kept results."}` +
				`}}`),
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

func (s *Server) callTool(id, params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolResult("ERROR: invalid tool-call params: "+err.Error(), true)
	}
	switch p.Name {
	case "eval":
		// The worker runs one tools/call at a time, so one id is current.
		s.setEvalID(id)
		defer s.setEvalID(nil)
		return s.evalTool(p.Arguments)
	default:
		return toolResult("ERROR: unknown tool '"+p.Name+"'", true)
	}
}

func (s *Server) setEvalID(id json.RawMessage) {
	s.evalIDMu.Lock()
	s.evalID = id
	s.evalIDMu.Unlock()
}

// EvalText runs code as one eval call (no vars, limits or clear) and returns
// the response text, with the output cap, overflow file and warnings of an
// MCP eval call. isError reports an error result. Used by `mcpshell repl`.
func (s *Server) EvalText(code string) (text string, isError bool) {
	raw, _ := json.Marshal(map[string]string{"code": code})
	res := s.evalTool(raw)
	content := res["content"].([]map[string]any)
	return content[0]["text"].(string), res["isError"].(bool)
}

func (s *Server) evalTool(rawArgs json.RawMessage) map[string]any {
	var args struct {
		Code    *string         `json:"code"`
		Vars    json.RawMessage `json:"vars"`
		Profile bool            `json:"profile"`
		Limits  json.RawMessage `json:"limits"`
		Clear   bool            `json:"clear"`
	}
	if json.Unmarshal(rawArgs, &args) != nil || args.Code == nil && !args.Clear {
		return toolResult("ERROR: missing 'code' argument (or clear: true to reset the session)", true)
	}
	code := ""
	if args.Code != nil {
		code = *args.Code
	}
	// vars are parsed like tool output: exact numbers, source key order.
	vars := map[string]runtime.Value{}
	varsRaw, err := varsJSON(args.Vars)
	if err != nil {
		return toolResult("ERROR: "+err.Error(), true)
	}
	if len(varsRaw) > 0 {
		if obj, ok := textToValue(varsRaw).(*runtime.ObjectVal); ok {
			for _, k := range obj.Keys() {
				vars[k], _ = obj.Get(k)
			}
		}
	}
	opts := runtime.EvalOptions{Profile: args.Profile}
	if len(args.Limits) > 0 && string(args.Limits) != "null" {
		spec, err := parseLimitsArg(args.Limits)
		if err != nil {
			return toolResult("ERROR: "+err.Error(), true)
		}
		opts.Limits = spec
	}
	// clear runs before the code, so the code starts from the start state.
	cleared := ""
	if args.Clear {
		cleared = s.shell.Clear()
		if args.Code == nil {
			return toolResult(cleared, false)
		}
	}
	value, err := s.shell.EvalExportedWith(code, vars, opts)
	warnings := s.shell.Diagnostics().Format()
	if cleared != "" {
		if warnings == "" {
			warnings = "\n"
		}
		warnings += "\nnote: " + cleared
	}
	if se, ok := err.(*runtime.ShellError); ok && se.Internal() {
		logInternalError(se, code, args.Vars)
	}
	if err != nil {
		return toolResult("ERROR: "+err.Error()+warnings, true)
	}
	out := value.Display()
	if len(out) > s.maxOutputBytes {
		head := strings.ToValidUTF8(out[:s.maxOutputBytes], "")
		note := s.shell.TruncationNote(value, len(head), len(out))
		content, format := overflowContent(value)
		if path, err := s.saveOverflow(content); err == nil {
			note += fmt.Sprintf("\nAlso saved to %s (%s): Files.read(%q, start, lines) or Files.grep(%q, {match: …}) page it.",
				path, format, path, path)
		} else if s.overflowDir != "" {
			fmt.Fprintf(os.Stderr, "mcpshell mcp: saving overflow output failed: %v\n", err)
		}
		out = head + note
	}
	return toolResult(out+warnings, false)
}

// varsJSON returns the vars argument as JSON object text. The schema declares
// a string (Gemini accepts only an OpenAPI subset, so no free-form object);
// a JSON object is accepted too, for callers that send one. A string must hold
// a JSON object.
func varsJSON(raw json.RawMessage) (string, error) {
	var str string
	if len(raw) == 0 || json.Unmarshal(raw, &str) != nil {
		return string(raw), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(str), &obj); err != nil || obj == nil {
		return "", errors.New(`vars: a string must hold a JSON object, e.g. "{\"path\":\"a.txt\"}"`)
	}
	return str, nil
}

// parseLimitsArg reads the eval tool's limits argument. A timeout is
// milliseconds, or a duration string ("60s").
func parseLimitsArg(raw json.RawMessage) (runtime.LimitSpec, error) {
	var spec runtime.LimitSpec
	var a struct {
		Steps       *json.Number    `json:"steps"`
		Timeout     json.RawMessage `json:"timeout"`
		CallDepth   *json.Number    `json:"callDepth"`
		OutputBytes *json.Number    `json:"outputBytes"`
		Memory      json.RawMessage `json:"memory"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&a); err != nil {
		return spec, fmt.Errorf("limits: %v (use {steps?, timeout?, callDepth?, outputBytes?, memory?} with integers)", err)
	}
	var err error
	if spec.Steps, err = limitInt("steps", a.Steps); err != nil {
		return spec, err
	}
	if spec.CallDepth, err = limitInt("callDepth", a.CallDepth); err != nil {
		return spec, err
	}
	if spec.OutputBytes, err = limitInt("outputBytes", a.OutputBytes); err != nil {
		return spec, err
	}
	if len(a.Memory) > 0 && string(a.Memory) != "null" {
		var str string
		var num json.Number
		var n int64
		if json.Unmarshal(a.Memory, &str) == nil {
			v, err := runtime.ParseBytes(str)
			if err != nil {
				return spec, fmt.Errorf("limits: %v", err)
			}
			n = v
		} else if json.Unmarshal(a.Memory, &num) == nil {
			r, ok := new(big.Rat).SetString(num.String())
			if !ok || !r.IsInt() || r.Sign() <= 0 {
				return spec, errors.New("limits: memory must be a positive integer (bytes) or a size string like \"2GB\"")
			}
			n = math.MaxInt64
			if r.Num().IsInt64() {
				n = r.Num().Int64()
			}
		} else {
			return spec, errors.New("limits: memory must be a positive integer (bytes) or a size string like \"2GB\"")
		}
		spec.MemoryBytes = &n
	}
	if len(a.Timeout) > 0 && string(a.Timeout) != "null" {
		var str string
		var ms int64
		var num json.Number
		if json.Unmarshal(a.Timeout, &str) == nil {
			n, err := runtime.ParseDurationMs(str)
			if err != nil {
				return spec, fmt.Errorf("limits: %v", err)
			}
			ms = n
		} else if json.Unmarshal(a.Timeout, &num) == nil {
			n, err := limitInt("timeout", &num)
			if err != nil {
				return spec, errors.New("limits: timeout must be an integer (ms) or a duration string like \"60s\"")
			}
			ms = int64(*n)
		} else {
			return spec, errors.New("limits: timeout must be an integer (ms) or a duration string like \"60s\"")
		}
		spec.TimeoutMs = &ms
	}
	return spec, nil
}

// limitInt converts a JSON number to an int limit. Clients send integers as
// floats (1e12, 5000000.0), so an integral value is accepted. A value above
// the int range becomes MaxInt, so the operator-maximum check reports it.
func limitInt(name string, n *json.Number) (*int, error) {
	if n == nil {
		return nil, nil
	}
	r, ok := new(big.Rat).SetString(n.String())
	if !ok || !r.IsInt() || r.Sign() < 0 {
		return nil, fmt.Errorf("limits: %s must be a non-negative integer, got %s", name, n.String())
	}
	v := math.MaxInt
	if r.Num().IsInt64() && r.Num().Int64() <= math.MaxInt {
		v = int(r.Num().Int64())
	}
	return &v, nil
}

// overflowContent renders an over-limit value for its overflow file so that
// line-based Files.read()/Files.grep() can page it: an array as JSON Lines (one element
// per line), an object as JSON with one key per line, a string as itself.
func overflowContent(v runtime.Value) (content, format string) {
	switch x := v.(type) {
	case *runtime.ArrayVal:
		var b strings.Builder
		for _, e := range x.Elements {
			b.WriteString(toolkit.ToJSON(e) + "\n")
		}
		return b.String(), "JSON Lines, one element per line"
	case *runtime.ObjectVal:
		var b strings.Builder
		b.WriteString("{\n")
		for i, k := range x.Keys() {
			e, _ := x.Get(k)
			b.WriteString("  " + toolkit.ToJSON(runtime.Str(k)) + ": " + toolkit.ToJSON(e))
			if i < x.Len()-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("}\n")
		return b.String(), "JSON, one key per line"
	case *runtime.StringVal:
		return x.V, "text"
	default:
		return v.Display(), "text"
	}
}

// saveOverflow writes out to a new uniquely named file in the overflow dir and
// returns its path.
func (s *Server) saveOverflow(out string) (string, error) {
	if s.overflowDir == "" {
		return "", errors.New("no overflow dir")
	}
	if err := os.MkdirAll(s.overflowDir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.overflowDir, "eval-*.txt")
	if err != nil {
		return "", err
	}
	_, werr := f.WriteString(out)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return f.Name(), werr
}

// internalErrorLog receives logInternalError's reports; tests replace it.
var internalErrorLog io.Writer = os.Stderr

// logInternalError writes a Go panic in mcpshell to stderr with the script
// that caused it, so the bug can be reproduced: the agent sees only the
// message. vars are cut at 4 KB.
func logInternalError(se *runtime.ShellError, code string, vars json.RawMessage) {
	w := internalErrorLog
	v := string(vars)
	if len(v) > 4096 {
		v = strings.ToValidUTF8(v[:4096], "") + "… (cut)"
	}
	fmt.Fprintf(w, "mcpshell mcp: internal error (a bug in mcpshell)\n%s\n--- script ---\n%s\n--- vars ---\n%s\n--- stack ---\n%s--- end ---\n",
		se.Message, code, v, se.Stack)
}
