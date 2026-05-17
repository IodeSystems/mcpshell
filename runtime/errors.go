package runtime

import (
	"sort"
	"strings"
)

// ShellError is the runtime error type. It implements `error` and is raised
// via panic; the top-level Eval recovers it into a result.
type ShellError struct{ Message string }

func (e *ShellError) Error() string { return e.Message }

// Runtime builds a plain runtime error.
func Runtime(message string) *ShellError { return &ShellError{Message: message} }

// Throw panics with a runtime error — shorthand for `panic(Runtime(msg))`.
func Throw(message string) { panic(Runtime(message)) }

// UnknownCommand builds the "Unknown command" error, with a JS-global hint or
// a fuzzy "did you mean?" list plus the full available-command listing.
func UnknownCommand(name string, available []string) *ShellError {
	if hint := jsGlobalHint(name); hint != "" {
		return &ShellError{Message: hint}
	}

	type scored struct {
		name string
		dist int
	}
	var near []scored
	for _, c := range available {
		if d := levenshtein(name, c); d <= 3 {
			near = append(near, scored{c, d})
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].dist < near[j].dist })
	if len(near) > 3 {
		near = near[:3]
	}

	suggestion := ""
	if len(near) > 0 {
		var b strings.Builder
		b.WriteString("\n\n  Did you mean?\n")
		for _, s := range near {
			b.WriteString("    " + s.name + "\n")
		}
		suggestion = strings.TrimRight(b.String(), "\n")
	}

	availableList := ""
	if len(available) > 0 {
		sorted := append([]string(nil), available...)
		sort.Strings(sorted)
		var b strings.Builder
		b.WriteString("\n\n  Available commands:\n")
		for _, c := range sorted {
			b.WriteString("    " + c + "\n")
		}
		availableList = strings.TrimRight(b.String(), "\n")
	}

	return &ShellError{Message: "Unknown command '" + name + "'" + suggestion + availableList}
}

// jsGlobalHint returns targeted guidance for JS keywords/globals that mcpshell
// deliberately omits, or "" if the name is not one of them.
func jsGlobalHint(name string) string {
	switch name {
	case "class":
		return "mcpshell does not support classes — use objects and functions\n\n  Example: let obj = {name: \"Alice\", greet: () => \"hi\"}"
	case "new":
		return "mcpshell does not support 'new' — there are no constructors or classes"
	case "import", "require":
		return "mcpshell does not support imports — all commands are built-in or registered via toolkits"
	case "async", "await":
		return "mcpshell does not support async/await — use all() for parallel execution\n\n  Example: all(() => fetchA(), () => fetchB())"
	case "void":
		return "mcpshell does not support 'void' — use null instead"
	case "instanceof":
		return "mcpshell does not support 'instanceof' — use typeof to check types\n\n  Example: typeof x == \"array\""
	case "this":
		return "mcpshell does not support 'this' — there are no classes or methods"
	case "super":
		return "mcpshell does not support 'super' — there is no inheritance"
	case "yield":
		return "mcpshell does not support generators — use arrays and pipes for data transformation"
	case "with":
		return "mcpshell does not support 'with'"
	case "enum":
		return "mcpshell does not support enums — use objects as constants\n\n  Example: let Status = {OK: 0, ERR: 1}"
	case "setTimeout", "setInterval", "clearTimeout", "clearInterval":
		return "mcpshell does not support timers"
	case "RegExp":
		return "mcpshell has regex literals instead\n\n  Example: /[0-9]+/g\n  Example: \"abc123\" |> match(/[0-9]+/)"
	case "Map", "Set", "WeakMap", "WeakSet":
		return "mcpshell does not have " + name + " — use objects and arrays\n\n  Example: unique([1, 2, 2, 3])  // Set-like dedup"
	case "Symbol", "Proxy", "Reflect":
		return "mcpshell does not support " + name
	case "Error", "TypeError", "RangeError":
		return "mcpshell does not have error types — use fail(message)\n\n  Example: fail(\"invalid input\")"
	case "NaN", "Infinity":
		return "mcpshell does not have " + name + " — use numeric checks instead"
	case "isNaN", "isFinite":
		return "mcpshell does not have '" + name + "'"
	case "encodeURIComponent", "decodeURIComponent", "encodeURI", "decodeURI":
		return "mcpshell does not have URI encoding functions"
	case "atob", "btoa":
		return "mcpshell does not have base64 functions"
	default:
		return ""
	}
}

// WrongArguments builds the "Wrong arguments" error.
func WrongArguments(name, expectedSignature string, got []Value, example string) *ShellError {
	parts := make([]string, len(got))
	for i, g := range got {
		parts[i] = g.TypeName() + ": " + g.Inspect()
	}
	exampleStr := ""
	if example != "" {
		exampleStr = "\n\n  Example:\n    " + example
	}
	return &ShellError{Message: "Wrong arguments for '" + name + "'\n\n" +
		"  Expected: " + name + "(" + expectedSignature + ")\n" +
		"  Got:      " + name + "(" + strings.Join(parts, ", ") + ")" + exampleStr}
}

// TypeMismatch builds the "Type mismatch in <operation>" error.
func TypeMismatch(operation, expected string, got Value, hint string) *ShellError {
	hintStr := ""
	if hint != "" {
		hintStr = "\n\n  Hint: " + hint
	}
	return &ShellError{Message: "Type mismatch in " + operation + "\n\n" +
		"  Expected: " + expected + "\n" +
		"  Got:      " + got.TypeName() + " (" + got.Inspect() + ")" + hintStr}
}

// PipeMismatch builds the "Type mismatch in pipe" error.
func PipeMismatch(fromCommand, toCommand string, value Value, expectedType, hint string) *ShellError {
	hintStr := ""
	if hint != "" {
		hintStr = "\n\n  Hint: " + hint
	}
	return &ShellError{Message: "Type mismatch in pipe\n\n" +
		"  '" + toCommand + "' expects " + expectedType + "\n" +
		"  but received " + value.TypeName() + " from '" + fromCommand + "'" + hintStr}
}

// levenshtein is the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	dp := make([][]int, len(ra)+1)
	for i := range dp {
		dp[i] = make([]int, len(rb)+1)
		dp[i][0] = i
	}
	for j := 0; j <= len(rb); j++ {
		dp[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			dp[i][j] = min(dp[i-1][j]+1, dp[i][j-1]+1, dp[i-1][j-1]+cost)
		}
	}
	return dp[len(ra)][len(rb)]
}
