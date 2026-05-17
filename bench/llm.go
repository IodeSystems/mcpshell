package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Attempt records one mcpshell tool invocation made during an agent run.
type Attempt struct {
	Code    string
	Result  string
	IsError bool
}

// LLM is a minimal client for an OpenAI-compatible chat-completions API.
type LLM struct {
	BaseURL string
	APIKey  string
	client  *http.Client
}

// NewLLM builds a client for the given base URL. Per-request deadlines are
// supplied via context, so the http client itself has no timeout.
func NewLLM(baseURL, apiKey string) *LLM {
	return &LLM{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		client:  &http.Client{},
	}
}

func (l *LLM) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, l.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if l.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+l.APIKey)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, truncate(string(data), 300))
	}
	return data, nil
}

// ListModels returns the model ids advertised at /v1/models.
func (l *LLM) ListModels(ctx context.Context) ([]string, error) {
	data, err := l.do(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	ids := make([]string, len(r.Data))
	for i, m := range r.Data {
		ids[i] = m.ID
	}
	return ids, nil
}

// ResolveModel resolves want against the server's model list: an exact id wins,
// otherwise the first id containing want (case-insensitive) is used.
func (l *LLM) ResolveModel(ctx context.Context, want string) (string, error) {
	ids, err := l.ListModels(ctx)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no models available at %s", l.BaseURL)
	}
	for _, id := range ids {
		if id == want {
			return id, nil
		}
	}
	lower := strings.ToLower(want)
	for _, id := range ids {
		if strings.Contains(strings.ToLower(id), lower) {
			return id, nil
		}
	}
	return "", fmt.Errorf("model %q not found; available: %s", want, strings.Join(ids, ", "))
}

// --- chat completions --------------------------------------------------------

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// mcpshellToolDef is the OpenAI function schema for the single `mcpshell` tool.
var mcpshellToolDef = json.RawMessage(`{"type":"function","function":{` +
	`"name":"mcpshell",` +
	`"description":"Execute mcpshell code (a sandboxed JS-subset scripting language) and return the result.",` +
	`"parameters":{"type":"object","properties":{` +
	`"code":{"type":"string","description":"mcpshell source code"}},"required":["code"]}}}`)

func (l *LLM) chat(ctx context.Context, model string, messages []chatMessage) (chatMessage, error) {
	data, err := l.do(ctx, http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":       model,
		"messages":    messages,
		"tools":       []json.RawMessage{mcpshellToolDef},
		"tool_choice": "auto",
		"temperature": 0,
	})
	if err != nil {
		return chatMessage{}, err
	}
	var r struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return chatMessage{}, err
	}
	if len(r.Choices) == 0 {
		return chatMessage{}, fmt.Errorf("chat completion returned no choices")
	}
	return r.Choices[0].Message, nil
}

// RunAgent drives a tool-calling agent loop: it sends the prompt, executes any
// mcpshell tool calls via runTool, feeds results back, and returns the model's
// final text answer along with every tool attempt made.
func (l *LLM) RunAgent(
	ctx context.Context,
	model, systemPrompt, userPrompt string,
	runTool func(code string) string,
	maxIters int,
) (answer string, attempts []Attempt, err error) {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	for range maxIters {
		msg, err := l.chat(ctx, model, messages)
		if err != nil {
			return "", attempts, err
		}
		messages = append(messages, msg)
		if len(msg.ToolCalls) == 0 {
			return strings.TrimSpace(msg.Content), attempts, nil
		}
		for _, tc := range msg.ToolCalls {
			code := extractCodeArg(tc.Function.Arguments)
			res := runTool(code)
			attempts = append(attempts, Attempt{
				Code:    code,
				Result:  res,
				IsError: strings.Contains(res, "ERROR:"),
			})
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    res,
			})
		}
	}
	return "", attempts, fmt.Errorf("reached max iterations (%d) without a final answer", maxIters)
}

// extractCodeArg pulls the `code` field out of a tool call's JSON arguments.
func extractCodeArg(argsJSON string) string {
	var a struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(argsJSON), &a) == nil {
		return a.Code
	}
	return argsJSON
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
