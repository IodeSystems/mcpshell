package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/iodesystems/mcpshell/runtime"
)

// schemaTypes returns the JSON types each property of a tool's input schema
// declares. A property without a plain "type" (anyOf, oneOf, $ref, …) is left
// out, so its argument is not checked.
func schemaTypes(schema json.RawMessage) map[string][]string {
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return nil
	}
	out := make(map[string][]string, len(s.Properties))
	for name, raw := range s.Properties {
		var p struct {
			Type json.RawMessage `json:"type"`
		}
		if json.Unmarshal(raw, &p) != nil || len(p.Type) == 0 {
			continue
		}
		var one string
		var many []string
		if json.Unmarshal(p.Type, &one) == nil {
			out[name] = []string{one}
		} else if json.Unmarshal(p.Type, &many) == nil && len(many) > 0 {
			out[name] = many
		}
	}
	return out
}

// coerceArgs converts each argument to its declared type when that loses
// nothing: a number or boolean to a string, a numeric string to a number, a
// "true"/"false" string to a boolean, a JSON string to an array or object,
// and a single value to a one-element array. An argument that cannot be
// converted fails the call before it goes upstream. null is passed as is.
func coerceArgs(callName string, args map[string]any, types map[string][]string) {
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		want, ok := types[name]
		v := args[name]
		if !ok || v == nil {
			continue
		}
		if c, ok := coerce(v, want); ok {
			args[name] = c
			continue
		}
		shown, _ := json.Marshal(v)
		if len(shown) > 80 {
			shown = append(shown[:80], "…"...)
		}
		panic(runtime.Runtime(fmt.Sprintf("%s: argument '%s' must be %s, got %s %s",
			callName, name, strings.Join(want, " or "), jsonKind(v), shown)))
	}
}

func coerce(v any, want []string) (any, bool) {
	for _, t := range want {
		if isType(v, t) {
			return v, true
		}
	}
	for _, t := range want {
		if c, ok := convert(v, t); ok {
			return c, true
		}
	}
	return nil, false
}

// isType reports whether v already has JSON type t. An unknown type name
// matches anything.
func isType(v any, t string) bool {
	switch t {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(json.Number)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		return ok && isWhole(n)
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "null":
		return v == nil
	default:
		return true
	}
}

// convert converts v to JSON type t when no information is lost.
func convert(v any, t string) (any, bool) {
	s, isStr := v.(string)
	switch t {
	case "string":
		switch x := v.(type) {
		case json.Number:
			return string(x), true
		case bool:
			return strconv.FormatBool(x), true
		}
	case "number", "integer":
		if isStr {
			if n, ok := decodeJSON(s).(json.Number); ok && (t == "number" || isWhole(n)) {
				return n, true
			}
		}
	case "boolean":
		if isStr {
			switch strings.ToLower(strings.TrimSpace(s)) {
			case "true":
				return true, true
			case "false":
				return false, true
			}
		}
	case "array":
		if isStr {
			if a, ok := decodeJSON(s).([]any); ok {
				return a, true
			}
		}
		return []any{v}, true
	case "object":
		if isStr {
			if o, ok := decodeJSON(s).(map[string]any); ok {
				return o, true
			}
		}
	}
	return nil, false
}

// decodeJSON parses s as one JSON value with exact numbers, or returns nil.
func decodeJSON(s string) any {
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(s)))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil // trailing data
	}
	return v
}

func isWhole(n json.Number) bool {
	r, ok := new(big.Rat).SetString(string(n))
	return ok && r.IsInt()
}

func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "value"
	}
}
