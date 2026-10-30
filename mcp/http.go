package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// authRefreshMargin is how long before a JWT's exp claim the cached token is
// treated as expired and auth_cmd is run again.
const authRefreshMargin = 60 * time.Second

// httpTimeout caps one HTTP exchange, body included. eval's wall-clock limit
// cannot interrupt a blocked native call, so this is what stops a stalled
// server from hanging eval. A var so tests can shorten it.
var httpTimeout = 120 * time.Second

// authCmdTimeout caps one run of auth_cmd.
const authCmdTimeout = 30 * time.Second

// errSessionExpired is a 404 on a request that carried a session id: the
// spec says the client must start a new session.
var errSessionExpired = errors.New("MCP session expired")

// HTTPClient is a minimal synchronous MCP client speaking JSON-RPC over the
// streamable HTTP transport: each message is a POST, answered with either
// application/json or a text/event-stream of `data:` frames.
type HTTPClient struct {
	url     string
	headers map[string]string
	authCmd string
	http    *http.Client

	mu        sync.Mutex
	nextID    int
	sessionID string
	protocol  string // negotiated version, sent as MCP-Protocol-Version after initialize
	token     string
	tokenExp  time.Time // zero: no known expiry, refreshed only on 401
}

// DialHTTP performs the MCP initialize handshake against url and returns the
// connected client together with the server's identity. If authCmd is set, it
// is run via `sh -c` and its trimmed stdout is sent as a bearer token.
func DialHTTP(url string, headers map[string]string, authCmd string) (*HTTPClient, ServerInfo, error) {
	c := &HTTPClient{url: url, headers: headers, authCmd: authCmd, http: &http.Client{Timeout: httpTimeout}}
	info, err := c.handshake()
	if err != nil {
		return nil, ServerInfo{}, err
	}
	return c, info, nil
}

// handshake runs initialize + notifications/initialized, starting a fresh
// session. Callers hold c.mu (or own c exclusively).
func (c *HTTPClient) handshake() (ServerInfo, error) {
	c.sessionID, c.protocol = "", ""
	res, err := c.request("initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mcpshell-mcp", "version": Version},
	})
	if err != nil {
		return ServerInfo{}, fmt.Errorf("initialize failed: %w", err)
	}
	var initResult struct {
		ProtocolVersion string     `json:"protocolVersion"`
		ServerInfo      ServerInfo `json:"serverInfo"`
	}
	_ = json.Unmarshal(res, &initResult)
	c.protocol = initResult.ProtocolVersion
	if c.protocol == "" {
		c.protocol = protocolVersion
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		return ServerInfo{}, err
	}
	return initResult.ServerInfo, nil
}

// ListTools returns the tools advertised by the server.
func (c *HTTPClient) ListTools() ([]ToolInfo, error) {
	res, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseToolsList(res)
}

// CallTool invokes a tool and returns its joined text content plus the error flag.
func (c *HTTPClient) CallTool(name string, args map[string]any) (text string, isError bool, err error) {
	res, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", true, err
	}
	return parseToolResult(res)
}

// Close ends the session (DELETE with the session id) when the server issued one.
func (c *HTTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessionID == "" {
		return nil
	}
	req, err := http.NewRequest(http.MethodDelete, c.url, nil)
	if err != nil {
		return err
	}
	if err := c.setHeaders(req, false); err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// call sends a JSON-RPC request and blocks for the matching response. If the
// server reports the session expired, it re-initializes and retries once.
func (c *HTTPClient) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	res, err := c.request(method, params)
	if errors.Is(err, errSessionExpired) {
		if _, err := c.handshake(); err != nil {
			return nil, err
		}
		res, err = c.request(method, params)
	}
	return res, err
}

// request sends one JSON-RPC request. Callers hold c.mu.
func (c *HTTPClient) request(method string, params any) (json.RawMessage, error) {
	c.nextID++
	id := c.nextID
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	resp, err := c.post(body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var msgs [][]byte
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		msgs, err = readSSE(resp.Body, id)
	} else {
		var data []byte
		data, err = io.ReadAll(resp.Body)
		msgs = [][]byte{data}
	}
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		if res, ok, err := matchResponse(m, id); ok {
			return res, err
		}
	}
	return nil, fmt.Errorf("no response for request %d", id)
}

// notify sends a JSON-RPC notification (no response expected). Callers hold c.mu.
func (c *HTTPClient) notify(method string, params any) error {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	resp, err := c.post(body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// post sends one JSON-RPC message, re-minting the auth token and retrying
// once on 401. Callers hold c.mu and must close the returned body.
func (c *HTTPClient) post(body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if err := c.setHeaders(req, attempt > 0); err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && c.authCmd != "" && attempt == 0 {
			resp.Body.Close()
			continue
		}
		if resp.StatusCode == http.StatusNotFound && c.sessionID != "" {
			resp.Body.Close()
			return nil, errSessionExpired
		}
		if resp.StatusCode/100 != 2 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(msg)))
		}
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			c.sessionID = sid
		}
		return resp, nil
	}
}

// setHeaders applies static headers, the session id, and the bearer token
// (minted by authCmd; forceAuth discards the cached one and tells authCmd so).
func (c *HTTPClient) setHeaders(req *http.Request, forceAuth bool) error {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.protocol != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocol)
	}
	if c.authCmd == "" {
		return nil
	}
	if forceAuth || c.token == "" || (!c.tokenExp.IsZero() && time.Now().After(c.tokenExp.Add(-authRefreshMargin))) {
		ctx, cancel := context.WithTimeout(context.Background(), authCmdTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", c.authCmd)
		if forceAuth {
			// The 401 retry: a minter that caches tokens (an opaque one, or
			// one revoked before its exp) must mint a new one, not return
			// the rejected token again.
			cmd.Env = append(os.Environ(), "MCPSHELL_AUTH_RETRY=1", "MCPSHELL_AUTH_REJECTED="+c.token)
		}
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("auth_cmd failed: %w", err)
		}
		c.token = strings.TrimSpace(string(out))
		if c.token == "" {
			return fmt.Errorf("auth_cmd printed no token")
		}
		c.tokenExp = jwtExpiry(c.token)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return nil
}

// readSSE collects `data:` payloads from an event stream, stopping once the
// response with the given id has arrived (the server may keep the stream open).
func readSSE(r io.Reader, id int) ([][]byte, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var msgs [][]byte
	var data []byte
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")...)
			data = append(data, '\n')
		case line == "" && len(data) > 0:
			msgs = append(msgs, data)
			if _, ok, _ := matchResponse(data, id); ok {
				return msgs, nil
			}
			data = nil
		}
	}
	if len(data) > 0 {
		msgs = append(msgs, data)
	}
	return msgs, sc.Err()
}

// matchResponse reports whether msg is the JSON-RPC response for id, returning
// its result or error.
func matchResponse(msg []byte, id int) (json.RawMessage, bool, error) {
	var resp struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(msg), &resp) != nil || len(resp.ID) == 0 {
		return nil, false, nil
	}
	var gotID int
	if json.Unmarshal(resp.ID, &gotID) != nil || gotID != id {
		return nil, false, nil
	}
	if resp.Error != nil {
		return nil, true, fmt.Errorf("%s", resp.Error.Message)
	}
	return resp.Result, true, nil
}

// jwtExpiry returns the exp claim of a JWT, or the zero time if token is not
// a JWT or has no exp. The signature is not checked — this only schedules refresh.
func jwtExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}
