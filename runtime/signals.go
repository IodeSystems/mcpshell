package runtime

// break/continue are implemented with panic/recover, caught by the nearest
// enclosing loop; they must never escape the package. (return is not here — it
// uses a flag on the Visitor, since it fires on every call and panic is costly.)

// BreakSignal unwinds to the nearest enclosing loop.
type BreakSignal struct{}

// ContinueSignal skips to the next iteration of the nearest enclosing loop.
type ContinueSignal struct{}
