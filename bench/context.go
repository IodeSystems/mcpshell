package bench

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/iodesystems/mcpshell/runtime"
)

var toolNameSanitize = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// DiscreteToolSchemas builds one OpenAI function-tool schema per registered
// command — the "expose every capability as its own MCP tool" world that a
// single `eval` replaces. Each carries the command's signature and description,
// as a real MCP author would write it.
func DiscreteToolSchemas(sh *runtime.Shell) []json.RawMessage {
	reg := sh.Commands()
	var tools []json.RawMessage
	for _, name := range reg.Names() {
		c := reg.Get(name)
		if c == nil {
			continue
		}
		desc := c.Description
		if c.Signature != "" {
			desc = c.Signature + " — " + c.Description
		}
		def := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        toolNameSanitize.ReplaceAllString(c.DisplayName(), "_"),
				"description": desc,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"args": map[string]any{"type": "array", "description": "positional arguments"},
					},
					"required": []string{"args"},
				},
			},
		}
		b, _ := json.Marshal(def)
		tools = append(tools, b)
	}
	return tools
}

// helpToolSchema is the tiny discovery tool used in the deferred strategy: the
// model calls it to learn capabilities at runtime instead of loading the whole
// reference upfront.
var helpToolSchema = json.RawMessage(`{"type":"function","function":{` +
	`"name":"help",` +
	`"description":"List or search mcpshell commands. Call with a search term to discover capabilities.",` +
	`"parameters":{"type":"object","properties":{"search":{"type":"string"}}}}}`)

// ContextRow is one tool-exposure strategy's measured per-request context cost.
type ContextRow struct {
	Strategy string
	Tools    int
	Tokens   int // prompt tokens above the empty baseline
}

// MeasureContext measures the per-request prompt-token cost of exposing the
// shell's capabilities as N discrete tools vs. a single `eval` tool (with the
// full reference, the compact reference, or a deferred help() discovery), using
// the model's own tokenizer via the API.
func MeasureContext(ctx context.Context, llm *LLM, model string, sh *runtime.Shell) ([]ContextRow, error) {
	base, err := llm.PromptTokens(ctx, model, "", nil)
	if err != nil {
		return nil, err
	}
	discrete := DiscreteToolSchemas(sh)
	evalTool := []json.RawMessage{EvalToolSchema}

	measure := func(system string, tools []json.RawMessage) (int, error) {
		n, err := llm.PromptTokens(ctx, model, system, tools)
		if err != nil {
			return 0, err
		}
		return n - base, nil
	}

	rows := []ContextRow{}
	add := func(name string, nTools int, system string, tools []json.RawMessage) error {
		tok, err := measure(system, tools)
		if err != nil {
			return err
		}
		rows = append(rows, ContextRow{Strategy: name, Tools: nTools, Tokens: tok})
		return nil
	}

	if err := add("N discrete MCP tools", len(discrete), "", discrete); err != nil {
		return nil, err
	}
	if err := add("mcpshell eval + full reference", 1, sh.ToPrompt(false), evalTool); err != nil {
		return nil, err
	}
	if err := add("mcpshell eval + compact reference", 1, sh.ToPrompt(true), evalTool); err != nil {
		return nil, err
	}
	if err := add("mcpshell eval + deferred help()", 2, "", []json.RawMessage{EvalToolSchema, helpToolSchema}); err != nil {
		return nil, err
	}
	return rows, nil
}
