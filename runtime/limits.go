package runtime

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	// Caps are the operator maximums: extendLimit cannot raise a limit past
	// them. Set with SetCaps; a cap is never below its default.
	CapSteps       int
	CapCallDepth   int
	CapTimeoutMs   int64
	CapOutputBytes int

	MaxSteps       int
	MaxCallDepth   int
	TimeoutMs      int64
	MaxOutputBytes int
	ResetOnEval    bool

	warnMu   sync.Mutex
	warnings []string

	// rejections are the async calls of this eval that failed, shared with
	// forked branch limits; an unhandled one fails the eval at its end.
	rejections *rejectionList

	stepCount   atomic.Int64
	callDepth   atomic.Int64
	startTimeMs atomic.Int64
	outputBytes atomic.Int64
	cancelled   atomic.Bool

	// Opt-in cost tracing: when profiling, Step attributes each step to its
	// source line. Off by default (one atomic load per step, negligible).
	profiling  atomic.Bool
	profMu     sync.Mutex
	lineSteps  map[int]int64
	profStart0 int64
}

// LineCost is a source line's accumulated step count during profiling.
type LineCost struct {
	Line  int
	Steps int64
}

const timeoutCheckInterval = 1000

// Default operator maximums for extendLimit: 20x steps, 10x timeout, 8x call
// depth, 16x output bytes of the standard limits (see docs/design.md).
const (
	DefaultCapSteps       = 20_000_000
	DefaultCapCallDepth   = 2048
	DefaultCapTimeoutMs   = 300_000
	DefaultCapOutputBytes = 1_000_000
)

// SetCaps sets the operator maximums. A value <= 0 keeps the current cap. A
// cap below the current default is raised to the default.
func (l *ExecutionLimits) SetCaps(steps, callDepth int, timeoutMs int64, outputBytes int) {
	if steps > 0 {
		l.CapSteps = max(steps, l.defaultMaxSteps)
	}
	if callDepth > 0 {
		l.CapCallDepth = max(callDepth, l.defaultMaxCallDepth)
	}
	if timeoutMs > 0 {
		l.CapTimeoutMs = max(timeoutMs, l.defaultTimeoutMs)
	}
	if outputBytes > 0 {
		l.CapOutputBytes = max(outputBytes, l.defaultMaxOutputBytes)
	}
}

// extendHint is the last line of a limit-hit message: a suggested value
// (want, clamped to the cap), or a statement that the cap is reached.
func extendHint(name string, cur, want, cap int64, what string) string {
	if cur >= cap {
		return fmt.Sprintf("    - %s is at the operator maximum (%d); extendLimit cannot raise it. Reduce the work", name, cap)
	}
	return fmt.Sprintf("    - If %s, use extendLimit({%s: %d}) (operator maximum: %d)", what, name, min(want, cap), cap)
}

// NewExecutionLimits builds limits with the standard defaults.
func NewExecutionLimits() *ExecutionLimits {
	l := &ExecutionLimits{
		defaultMaxSteps:       1_000_000,
		defaultMaxCallDepth:   256,
		defaultTimeoutMs:      30_000,
		defaultMaxOutputBytes: 64_000,
		CapSteps:              DefaultCapSteps,
		CapCallDepth:          DefaultCapCallDepth,
		CapTimeoutMs:          DefaultCapTimeoutMs,
		CapOutputBytes:        DefaultCapOutputBytes,
		ResetOnEval:           true,
		rejections:            &rejectionList{},
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
	if l.profiling.Load() {
		l.profMu.Lock()
		l.lineSteps[line]++
		l.profMu.Unlock()
	}
	if count > int64(l.MaxSteps) {
		panic(Runtime(fmt.Sprintf(
			"Execution step limit exceeded (%d steps) at line %d%s\n\n"+
				"  Common fixes:\n"+
				"    - Recursive algorithms (e.g. fib(n-1)+fib(n-2)) are O(2^n) — rewrite with a loop\n"+
				"    - Check while/for conditions for infinite loops\n"+
				"    - Filter or limit() data earlier to reduce iterations\n"+
				"    - profile(() => ...) to see which lines burn the steps\n%s",
			l.MaxSteps, line, l.hotLinesHint(),
			extendHint("steps", int64(l.MaxSteps), int64(l.MaxSteps)*5, int64(l.CapSteps),
				"your algorithm is correct but data is large"))))
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
				"    - Restructure to avoid redundant computation\n%s",
			l.TimeoutMs, float64(elapsed)/1000.0, line,
			extendHint("timeout", l.TimeoutMs, l.TimeoutMs*2, l.CapTimeoutMs,
				"your algorithm is correct but data is large"))))
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
				"    - Use reduce() instead of manual recursion\n%s",
			l.MaxCallDepth, line,
			extendHint("callDepth", int64(l.MaxCallDepth), int64(l.MaxCallDepth)*2, int64(l.CapCallDepth),
				"recursion depth is expected"))))
	}
}

// PopCall leaves a call frame.
func (l *ExecutionLimits) PopCall() { l.callDepth.Add(-1) }

// Profiling reports whether cost tracing is currently active.
func (l *ExecutionLimits) Profiling() bool { return l.profiling.Load() }

// StartProfile begins per-line step attribution, resetting any prior trace.
func (l *ExecutionLimits) StartProfile() {
	l.profMu.Lock()
	l.lineSteps = make(map[int]int64)
	l.profMu.Unlock()
	l.profStart0 = l.stepCount.Load()
	l.profiling.Store(true)
}

// StopProfile ends attribution and returns per-line costs (highest first) plus
// the total steps executed while profiling.
func (l *ExecutionLimits) StopProfile() (lines []LineCost, total int64) {
	l.profiling.Store(false)
	l.profMu.Lock()
	defer l.profMu.Unlock()
	for ln, c := range l.lineSteps {
		lines = append(lines, LineCost{Line: ln, Steps: c})
	}
	l.lineSteps = nil
	sort.Slice(lines, func(i, j int) bool { return lines[i].Steps > lines[j].Steps })
	return lines, l.stepCount.Load() - l.profStart0
}

// hotLinesHint returns a short " — hot lines: L3 ×842000, ..." fragment when a
// profile is active (so a limit tripped inside profile(...) is actionable).
func (l *ExecutionLimits) hotLinesHint() string {
	if !l.profiling.Load() {
		return ""
	}
	l.profMu.Lock()
	lines := make([]LineCost, 0, len(l.lineSteps))
	for ln, c := range l.lineSteps {
		lines = append(lines, LineCost{Line: ln, Steps: c})
	}
	l.profMu.Unlock()
	if len(lines) == 0 {
		return ""
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Steps > lines[j].Steps })
	var parts []string
	for i, lc := range lines {
		if i == 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("L%d ×%d", lc.Line, lc.Steps))
	}
	return "\n  Hot lines: " + strings.Join(parts, ", ")
}

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
				"    - Filter or map to extract only the fields you need\n%s",
			total, l.MaxOutputBytes, source,
			extendHint("outputBytes", int64(l.MaxOutputBytes), int64(l.MaxOutputBytes)*4, int64(l.CapOutputBytes),
				"you need all of it"))))
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
	l.CapSteps = max(l.CapSteps, maxSteps)
	l.CapCallDepth = max(l.CapCallDepth, maxCallDepth)
	l.CapTimeoutMs = max(l.CapTimeoutMs, timeoutMs)
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
		CapSteps:              l.CapSteps,
		CapCallDepth:          l.CapCallDepth,
		CapTimeoutMs:          l.CapTimeoutMs,
		CapOutputBytes:        l.CapOutputBytes,
		ResetOnEval:           false,
		rejections:            l.rejections,
	}
	child.MaxSteps = l.MaxSteps
	child.MaxCallDepth = l.MaxCallDepth
	child.TimeoutMs = l.TimeoutMs
	child.MaxOutputBytes = l.MaxOutputBytes
	child.startTimeMs.Store(l.startTimeMs.Load())
	return child
}

// warn records a warning for the current eval; a repeated one is kept once.
func (l *ExecutionLimits) warn(msg string) {
	l.warnMu.Lock()
	defer l.warnMu.Unlock()
	if !slices.Contains(l.warnings, msg) {
		l.warnings = append(l.warnings, msg)
	}
}

// Warnings returns the warnings recorded since ClearWarnings.
func (l *ExecutionLimits) Warnings() []string {
	l.warnMu.Lock()
	defer l.warnMu.Unlock()
	return slices.Clone(l.warnings)
}

// ClearWarnings starts a new eval's warnings.
func (l *ExecutionLimits) ClearWarnings() {
	l.warnMu.Lock()
	defer l.warnMu.Unlock()
	l.warnings = nil
	l.rejections = &rejectionList{}
}

// ParseCaps applies a "steps=N,timeout=60s,callDepth=N,outputBytes=N" spec (the
// --max-limits flag) to the operator maximums. Unlisted keys keep their cap.
// timeout takes a Go duration ("60s", "5m") or plain milliseconds.
func (l *ExecutionLimits) ParseCaps(spec string) error {
	var steps, depth, out int
	var timeoutMs int64
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			return fmt.Errorf("max-limits: %q is not key=value", part)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if key == "timeout" {
			d, err := time.ParseDuration(val)
			if err != nil {
				n, nerr := strconv.ParseInt(val, 10, 64)
				if nerr != nil {
					return fmt.Errorf("max-limits: timeout %q is not a duration (60s) or milliseconds", val)
				}
				d = time.Duration(n) * time.Millisecond
			}
			if d <= 0 {
				return fmt.Errorf("max-limits: timeout must be positive")
			}
			timeoutMs = d.Milliseconds()
			continue
		}
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return fmt.Errorf("max-limits: %s %q is not a positive integer", key, val)
		}
		switch key {
		case "steps":
			steps = n
		case "callDepth":
			depth = n
		case "outputBytes":
			out = n
		default:
			return fmt.Errorf("max-limits: unknown key %q (use steps, timeout, callDepth, outputBytes)", key)
		}
	}
	l.SetCaps(steps, depth, timeoutMs, out)
	return nil
}
