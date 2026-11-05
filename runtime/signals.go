package runtime

// break/continue are implemented with panic/recover, caught by the nearest
// enclosing loop; they must never escape the package. (return is not here — it
// uses a flag on the Visitor, since it fires on every call and panic is costly.)

// BreakSignal unwinds to the nearest enclosing loop, or with a Label to the
// loop of that label.
type BreakSignal struct{ Label string }

// ContinueSignal skips to the next iteration of the nearest enclosing loop, or
// with a Label of the loop of that label.
type ContinueSignal struct{ Label string }

// strayLoopSignal turns a break or continue that reached a function or eval
// boundary into an error; JS rejects both at parse time.
func strayLoopSignal(r any) *ShellError {
	var kw, label string
	switch s := r.(type) {
	case BreakSignal:
		kw, label = "break", s.Label
	case ContinueSignal:
		kw, label = "continue", s.Label
	default:
		return nil
	}
	if label != "" {
		return Runtime("'" + kw + " " + label + "' has no enclosing loop labeled '" + label + "'")
	}
	return Runtime("'" + kw + "' outside a loop")
}
