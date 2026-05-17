package runtime

import "strings"

// kv is an ordered key→value pair. Ordered slices are used instead of maps
// wherever the build order is observable (namespace object field order).
type kv struct{ k, v string }

// jsNamespaceAliases maps a JS global namespace to its { jsMethod → mcpshellCommand }.
var jsNamespaceAliases = []struct {
	name    string
	methods []kv
}{
	{"JSON", []kv{{"parse", "parseJson"}, {"stringify", "toJson"}}},
	{"Math", []kv{
		{"floor", "floor"}, {"ceil", "ceil"}, {"round", "round"}, {"abs", "abs"},
		{"min", "min"}, {"max", "max"}, {"pow", "pow"},
	}},
	{"Object", []kv{
		{"keys", "keys"}, {"values", "values"}, {"entries", "entries"}, {"fromEntries", "fromEntries"},
	}},
	{"console", []kv{{"log", "print"}}},
	{"Array", []kv{{"isArray", "isArray"}, {"from", "toArray"}}},
}

// jsConstructorAliases maps JS constructor-style calls to mcpshell command names.
var jsConstructorAliases = []kv{
	{"String", "str"}, {"Number", "num"}, {"Boolean", "bool"},
	{"parseInt", "num"}, {"parseFloat", "num"},
}

// mutatingArrayMethods are array methods that mutate-and-write-back via lvalue tracking.
var mutatingArrayMethods = map[string]bool{
	"push": true, "pop": true, "shift": true, "unshift": true, "splice": true,
}

// jsMethodAliases maps JS method names to mcpshell command names, keyed by receiver type.
var jsMethodAliases = map[string]map[string]string{
	"array": {
		"includes": "contains",
	},
	"string": {
		"toUpperCase": "upper",
		"toLowerCase": "lower",
		"includes":    "contains",
		"replaceAll":  "replace",
		"matchAll":    "match",
		"search":      "test",
		"slice":       "substring",
		"trimLeft":    "trimStart",
		"trimRight":   "trimEnd",
	},
}

// jsMethodHints holds JS methods with no mcpshell equivalent → error hint text.
var jsMethodHints = map[string]map[string]string{
	"array": {},
}

// unescapeString resolves backslash escapes in a regular (non-raw) string body.
func unescapeString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var sb strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) {
			switch rs[i+1] {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '\'':
				sb.WriteRune('\'')
			case '`':
				sb.WriteRune('`')
			case '$':
				sb.WriteRune('$')
			default:
				sb.WriteRune('\\')
				sb.WriteRune(rs[i+1])
			}
			i++
		} else {
			sb.WriteRune(rs[i])
		}
	}
	return sb.String()
}
