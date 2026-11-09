package runtime

import "strings"

// Ancestors is the path of containers from the root of a walk to the
// current value. A value already on the path is a cycle (a.self = a). A value
// reached twice by different paths (shared, not cyclic) is not on the path
// the second time, so it is walked again, as JSON.stringify does.
type Ancestors []Value

// Enter adds v to the path, or reports false when v is already on it.
func (a *Ancestors) Enter(v Value) bool {
	for _, x := range *a {
		if x == v {
			return false
		}
	}
	*a = append(*a, v)
	return true
}

// Leave removes the last value Enter added.
func (a *Ancestors) Leave() { *a = (*a)[:len(*a)-1] }

// CircularError is the error of a walk that cannot represent a cycle, as JS
// raises for JSON.stringify of an object that contains itself.
func CircularError(what string) *ShellError {
	return Runtime(what + ": the value contains itself (a circular reference, such as o.self = o), which " +
		what + " cannot represent. Remove the back-reference first, or keep an id instead of the object")
}

// inspectIn renders v for display, with [Circular] for a value that
// contains itself, as Node prints it.
func inspectIn(v Value, anc *Ancestors) string {
	switch x := v.(type) {
	case *ArrayVal:
		if !anc.Enter(x) {
			return "[Circular]"
		}
		defer anc.Leave()
		parts := make([]string, len(x.Elements))
		for i, e := range x.Elements {
			parts[i] = inspectIn(e, anc)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *MapVal:
		if !anc.Enter(x) {
			return "[Circular]"
		}
		defer anc.Leave()
		return x.inspect(anc)
	case *ObjectVal:
		if x.file != nil {
			return fileInspect(x)
		}
		if !anc.Enter(x) {
			return "[Circular]"
		}
		defer anc.Leave()
		if x.isErrorObject() && x.proto != nil {
			return errorInspect(x, anc)
		}
		var sb strings.Builder
		if c := x.className(); c != "" {
			sb.WriteString(c)
			sb.WriteByte(' ')
		}
		sb.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(inspectIn(x.m[k], anc))
		}
		sb.WriteByte('}')
		return sb.String()
	}
	return v.Inspect()
}
