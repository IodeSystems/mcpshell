package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// ServerInfo identifies an upstream MCP server (from its initialize response).
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolInfo describes one tool exposed by an upstream server.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Client is a minimal synchronous MCP client speaking JSON-RPC over a child
// process's stdio.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	enc     *json.Encoder
	scanner *bufio.Scanner
	nextID  int
	mu      sync.Mutex
}

// Dial spawns command as a subprocess, performs the MCP initialize handshake,
// and returns the connected client together with the server's identity.
func Dial(command []string, env map[string]string) (*Client, ServerInfo, error) {
	if len(command) == 0 {
		return nil, ServerInfo{}, fmt.Errorf("empty command")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, ServerInfo{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, ServerInfo{}, err
	}
	if err := cmd.Start(); err != nil {
		return nil, ServerInfo{}, err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	c := &Client{cmd: cmd, stdin: stdin, enc: json.NewEncoder(stdin), scanner: scanner}

	res, err := c.call("initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mcpshell-mcp", "version": "0.1.0"},
	})
	if err != nil {
		_ = c.Close()
		return nil, ServerInfo{}, fmt.Errorf("initialize failed: %w", err)
	}
	var initResult struct {
		ServerInfo ServerInfo `json:"serverInfo"`
	}
	_ = json.Unmarshal(res, &initResult)
	if err := c.notify("notifications/initialized", nil); err != nil {
		_ = c.Close()
		return nil, ServerInfo{}, err
	}
	return c, initResult.ServerInfo, nil
}

// ListTools returns the tools advertised by the server.
func (c *Client) ListTools() ([]ToolInfo, error) {
	res, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var r struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	return r.Tools, nil
}

// CallTool invokes a tool and returns its joined text content plus the error flag.
func (c *Client) CallTool(name string, args map[string]any) (text string, isError bool, err error) {
	res, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", true, err
	}
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return "", true, err
	}
	var parts []string
	for _, c := range r.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n"), r.IsError, nil
}

// Close shuts down the subprocess.
func (c *Client) Close() error {
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}

// call sends a JSON-RPC request and blocks for the matching response.
func (c *Client) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextID++
	id := c.nextID
	if err := c.enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		return nil, err
	}

	for c.scanner.Scan() {
		line := bytes.TrimSpace(c.scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var resp struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if json.Unmarshal(line, &resp) != nil || len(resp.ID) == 0 {
			continue // malformed line or a server-initiated notification
		}
		var gotID int
		if json.Unmarshal(resp.ID, &gotID) != nil || gotID != id {
			continue // response for a different request
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("%s", resp.Error.Message)
		}
		return resp.Result, nil
	}
	if err := c.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("connection closed before response")
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
