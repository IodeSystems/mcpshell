package mcp_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/mcp"
)

// Gemini accepts only an OpenAPI 3 subset of JSON Schema: no type arrays, no
// additionalProperties, no combinators, $ref, const or patternProperties.
func TestEvalSchemaGeminiSubset(t *testing.T) {
	var out bytes.Buffer
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	if err := mcp.NewServer(coreShell(), 16000).RunStdio(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Result struct {
			Tools []struct {
				Name        string
				InputSchema any
			}
		}
	}
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Result.Tools) == 0 {
		t.Fatal("no tools")
	}
	banned := []string{"additionalProperties", "oneOf", "anyOf", "allOf", "$ref", "const", "patternProperties"}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			if ty, ok := x["type"]; ok {
				if _, isStr := ty.(string); !isStr {
					t.Errorf("%s: type is not a plain string: %v", path, ty)
				}
			}
			if x["type"] == "object" {
				if p, ok := x["properties"].(map[string]any); ok && len(p) == 0 {
					t.Errorf("%s: object with empty properties", path)
				} else if !ok {
					t.Errorf("%s: object without properties", path)
				}
			}
			for k, c := range x {
				for _, b := range banned {
					if k == b {
						t.Errorf("%s: banned keyword %s", path, k)
					}
				}
				walk(path+"."+k, c)
			}
		case []any:
			for i, c := range x {
				walk(path+"[]"+string(rune('0'+i)), c)
			}
		}
	}
	for _, tool := range m.Result.Tools {
		walk(tool.Name, tool.InputSchema)
	}
}

func TestEvalVarsForms(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(),
		codeArgs("x + 1", `"vars":{"x":41}`),
		codeArgs("x + 1", `"vars":"{\"x\":41}"`),
		codeArgs("p", `"vars":"{\"p\":\"a\\\\b\"}"`),
		codeArgs("1", `"vars":"not json"`),
		codeArgs("1", `"vars":"[1,2]"`),
		codeArgs("1", `"vars":"\"str\""`),
	)
	if errs[0] || texts[0] != "42" {
		t.Errorf("object vars: %q", texts[0])
	}
	if errs[1] || texts[1] != "42" {
		t.Errorf("string vars: %q", texts[1])
	}
	if errs[2] || texts[2] != `a\b` {
		t.Errorf("string vars with backslash: %q", texts[2])
	}
	for i := 3; i < 6; i++ {
		if !errs[i] || !strings.Contains(texts[i], "vars") {
			t.Errorf("bad vars %d: %q", i, texts[i])
		}
	}
}

func TestEvalMemoryForms(t *testing.T) {
	texts, errs := evalCalls(t, coreShell(),
		codeArgs("limits().maxMemory", `"limits":{"memory":134217728}`),
		codeArgs("limits().maxMemory", `"limits":{"memory":"64MB"}`),
		codeArgs("limits().maxMemory", `"limits":{"memory":"134217728"}`),
		codeArgs("1", `"limits":{"bogus":1}`),
	)
	want := []string{"134217728", "67108864", "134217728"}
	for i, w := range want {
		if errs[i] || texts[i] != w {
			t.Errorf("memory form %d: %q (err %v), want %s", i, texts[i], errs[i], w)
		}
	}
	if !errs[3] || !strings.Contains(texts[3], "limits") {
		t.Errorf("unknown limit key must still error: %q", texts[3])
	}
}
