package runtime

import (
	"fmt"
	"sync/atomic"
	"time"
)

// ExecutionLimits bounds a single eval: step count, call depth, wall-clock
// time, and output size. Counters are atomic so parallel branches (the scatter
// pipe, all()/race()) share one budget safely.
type ExecutionLimits struct {
	defaultMaxSteps       int
	defaultMaxCallDepth   int
	defaultTimeoutMs      int64
	defaultMaxOutputBytes int

	MaxSteps       int
	MaxCallDepth   int
	TimeoutMs      int64
	MaxOutputBytes int
	ResetOnEval    bool

	stepCount   atomic.Int64
	callDepth   atomic.Int64
	startTimeMs atomic.Int64
	outputBytes atomic.Int64
	cancelled   atomic.Bool
}

const timeoutCheckInterval = 1000

// NewExecutionLimits builds limits with the standard defaults.
func NewExecutionLimits() *ExecutionLimits {
	l := &ExecutionLimits{
		defaultMaxSteps:       1_000_000,
		defaultMaxCallDepth:   256,
		defaultTimeoutMs:      30_000,
		defaultMaxOutputBytes: 64_000,
		ResetOnEval:           true,
	}
	l.MaxSteps = l.defaultMaxSteps
	l.MaxCallDepth = l.defaultMaxCallDepth
	l.TimeoutMs = l.defaultTimeoutMs
	l.MaxOutputBytes = l.defaultMaxOutputBytes
	l.startTimeMs.Store(nowMs())
	return l
}

func nowMs() int64 { return time.Now().UnixMilli() }

// Cancel requests cooperative cancellation; the next step() will raise.
func (l *ExecutionLimits) Cancel() { l.cancelled.Store(true) }

// Step counts one execution step, checking cancellation, the step limit, and
// (periodically) the wall-clock timeout. Panics *ShellError when a limit trips.
func (l *ExecutionLimits) Step(line int) {
	if l.cancelled.Load() {
		panic(Runtime("Execution cancelled"))
	}
	count := l.stepCount.Add(1)
	if count > int64(l.MaxSteps) {
		panic(Runtime(fmt.Sprintf(
			"Execution step limit exceeded (%d steps) at line %d\n\n"+
				"  Common fixes:\n"+
				"    - Recursive algorithms (e.g. fib(n-1)+fib(n-2)) are O(2^n) — rewrite with a loop\n"+
				"    - Check while/for conditions for infinite loops\n"+
				"    - Filter or limit() data earlier to reduce iterations\n"+
				"    - If your algorithm is correct but data is large, use extendLimit({steps: %d})",
			l.MaxSteps, line, l.MaxSteps*5)))
	}
	if count%timeoutCheckInterval == 0 {
		l.checkTimeout(line)
	}
}

func (l *ExecutionLimits) checkTimeout(line int) {
	elapsed := nowMs() - l.startTimeMs.Load()
	if elapsed > l.TimeoutMs {
		panic(Runtime(fmt.Sprintf(
			"Execution timeout exceeded (%dms / %.1fs elapsed) at line %d\n\n"+
				"  Common fixes:\n"+
				"    - Recursive algorithms (e.g. fib(n-1)+fib(n-2)) are O(2^n) — rewrite with a loop\n"+
				"    - Process less data: use limit() or filter early\n"+
				"    - Restructure to avoid redundant computation\n"+
				"    - If your algorithm is correct but data is large, use extendLimit({timeout: %d})",
			l.TimeoutMs, float64(elapsed)/1000.0, line, l.TimeoutMs*2)))
	}
}

// PushCall enters a call frame, checking the call-depth limit.
func (l *ExecutionLimits) PushCall(line int) {
	if l.callDepth.Add(1) > int64(l.MaxCallDepth) {
		panic(Runtime(fmt.Sprintf(
			"Call stack depth exceeded (%d) at line %d\n\n"+
				"  Common fixes:\n"+
				"    - Is the recursion missing a base case?\n"+
				"    - Convert to an iterative approach using while/for\n"+
				"    - Use reduce() instead of manual recursion\n"+
				"    - If recursion depth is expected, use extendLimit({callDepth: %d})",
			l.MaxCallDepth, line, l.MaxCallDepth*2)))
	}
}

// PopCall leaves a call frame.
func (l *ExecutionLimits) PopCall() { l.callDepth.Add(-1) }

// TrackOutput accumulates output bytes, raising when the limit is exceeded.
func (l *ExecutionLimits) TrackOutput(bytes int, source string) {
	total := l.outputBytes.Add(int64(bytes))
	if total > int64(l.MaxOutputBytes) {
		panic(Runtime(fmt.Sprintf(
			"Output limit exceeded (%d bytes > %d byte limit) in %s\n\n"+
				"  Your program is producing too much output.\n\n"+
				"  Common fixes:\n"+
				"    - Use limit() to reduce results before returning\n"+
				"    - Use read(path, startLine, lineCount) for partial file reads\n"+
				"    - Filter or map to extract only the fields you need",
			total, l.MaxOutputBytes, source)))
	}
}

// SetDefaults overrides the standard limit defaults. Embedding hosts that need
// tighter or looser bounds call this before evaluating; the new values take
// effect immediately and on every subsequent Reset.
func (l *ExecutionLimits) SetDefaults(maxSteps, maxCallDepth int, timeoutMs int64) {
	l.defaultMaxSteps = maxSteps
	l.defaultMaxCallDepth = maxCallDepth
	l.defaultTimeoutMs = timeoutMs
	l.MaxSteps = maxSteps
	l.MaxCallDepth = maxCallDepth
	l.TimeoutMs = timeoutMs
}

// Reset restores defaults and zeroes the per-eval counters.
func (l *ExecutionLimits) Reset() {
	l.MaxSteps = l.defaultMaxSteps
	l.MaxCallDepth = l.defaultMaxCallDepth
	l.TimeoutMs = l.defaultTimeoutMs
	l.MaxOutputBytes = l.defaultMaxOutputBytes
	l.stepCount.Store(0)
	l.callDepth.Store(0)
	l.startTimeMs.Store(nowMs())
	l.outputBytes.Store(0)
	l.cancelled.Store(false)
}

// Fork creates child limits for a parallel branch, sharing the start time but
// carrying their own step budget and cancellation token.
func (l *ExecutionLimits) Fork() *ExecutionLimits {
	child := &ExecutionLimits{
		defaultMaxSteps:       l.MaxSteps,
		defaultMaxCallDepth:   l.MaxCallDepth,
		defaultTimeoutMs:      l.TimeoutMs,
		defaultMaxOutputBytes: l.MaxOutputBytes,
		ResetOnEval:           false,
	}
	child.MaxSteps = l.MaxSteps
	child.MaxCallDepth = l.MaxCallDepth
	child.TimeoutMs = l.TimeoutMs
	child.MaxOutputBytes = l.MaxOutputBytes
	child.startTimeMs.Store(l.startTimeMs.Load())
	return child
}
