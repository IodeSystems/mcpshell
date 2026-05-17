package runtime

// Non-local control flow is implemented with panic/recover — an
// exception-driven interpreter structure. The top-level Eval recovers these;
// they must never escape the package.

// ReturnSignal carries a function's return value out of nested statements.
type ReturnSignal struct{ Value Value }

// BreakSignal unwinds to the nearest enclosing loop.
type BreakSignal struct{}

// ContinueSignal skips to the next iteration of the nearest enclosing loop.
type ContinueSignal struct{}
