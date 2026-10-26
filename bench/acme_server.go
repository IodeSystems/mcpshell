package bench

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

// ServeAcme serves the Acme tools as a streamable-HTTP MCP server on a
// loopback port, for mcpshell to host as an upstream. Close it when done.
func ServeAcme(d *AcmeData) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.ID) == 0 { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var res any
		switch req.Method {
		case "initialize":
			res = map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "acme", "version": "1"},
			}
		case "tools/list":
			var tools []map[string]any
			for _, t := range d.Tools() {
				tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema})
			}
			res = map[string]any{"tools": tools}
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			text, isError := d.Call(p.Name, p.Arguments)
			res = map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "method not found: " + req.Method}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
	}))
}
