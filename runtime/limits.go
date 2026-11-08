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

	// Hint is how limit-hit errors say to raise a limit. The default names
	// extendLimit; the MCP server and the CLI set the form they accept.
	Hint HintMode

	warnMu   sync.Mutex
	warnings []string
	notes    []string

	// rejections are the async calls of this eval that failed, shared with
	// forked branch limits; an unhandled one fails the eval at its end.
	rejections *rejectionList

	stepCount   atomic.Int64
	callDepth   atomic.Int64
	startTimeMs atomic.Int64
	outputBytes atomic.Int64
	cancelled   atomic.Bool
	peakDepth   atomic.Int64

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

// HintMode selects how a limit-hit error tells the agent to raise the limit.
type HintMode int

const (
	HintExtend  HintMode = iota // extendLimit({steps: N}): REPL, embedding hosts
	HintEvalArg                 // the eval tool argument limits: {steps: N} (MCP server)
	HintFlag                    // --limits steps=N (CLI one-shot)
)

// RaiseHint renders the way to set limit name to n in the current HintMode.
func (l *ExecutionLimits) RaiseHint(name string, n int64) string {
	switch l.Hint {
	case HintEvalArg:
		return fmt.Sprintf("re-run with the eval argument limits: {%s: %d}", name, n)
	case HintFlag:
		return fmt.Sprintf("re-run with --limits %s=%d", name, n)
	}
	return fmt.Sprintf("use extendLimit({%s: %d})", name, n)
}

// profileHint is the error-text line that points at per-line costs.
func (l *ExecutionLimits) profileHint() string {
	switch l.Hint {
	case HintEvalArg:
		return "run with the eval argument profile: true to see which lines burn the steps"
	case HintFlag:
		return "run with --profile to see which lines burn the steps"
	}
	return "profile(() => ...) to see which lines burn the steps"
}

// extendHint is the last line of a limit-hit message: a suggested value
// (want, clamped to the cap), or a statement that the cap is reached.
func (l *ExecutionLimits) extendHint(name string, cur, want, cap int64, what string) string {
	if cur >= cap {
		return fmt.Sprintf("    - %s is at the operator maximum (%d); it cannot be raised. Reduce the work", name, cap)
	}
	return fmt.Sprintf("    - If %s, %s (operator maximum: %d)", what, l.RaiseHint(name, min(want, cap)), cap)
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

// Cancelled reports whether Cancel was called.
func (l *ExecutionLimits) Cancelled() bool { return l.cancelled.Load() }

// Remaining is the wall-clock time left before the eval timeout. It is zero
// or negative when the time is used up.
func (l *ExecutionLimits) Remaining() time.Duration {
	return time.Duration(l.TimeoutMs-(nowMs()-l.startTimeMs.Load())) * time.Millisecond
}

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
				"    - %s\n%s",
			l.MaxSteps, line, l.hotLinesHint(), l.profileHint(),
			l.extendHint("steps", int64(l.MaxSteps), int64(l.MaxSteps)*5, int64(l.CapSteps),
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
			l.extendHint("timeout", l.TimeoutMs, l.TimeoutMs*2, l.CapTimeoutMs,
				"your algorithm is correct but data is large"))))
	}
}

// PushCall enters a call frame, checking the call-depth limit.
func (l *ExecutionLimits) PushCall(line int) {
	d := l.callDepth.Add(1)
	if d > l.peakDepth.Load() {
		l.peakDepth.Store(d)
	}
	if d > int64(l.MaxCallDepth) {
		panic(Runtime(fmt.Sprintf(
			"Call stack depth exceeded (%d) at line %d\n\n"+
				"  Common fixes:\n"+
				"    - Is the recursion missing a base case?\n"+
				"    - Convert to an iterative approach using while/for\n"+
				"    - Use reduce() instead of manual recursion\n%s",
			l.MaxCallDepth, line,
			l.extendHint("callDepth", int64(l.MaxCallDepth), int64(l.MaxCallDepth)*2, int64(l.CapCallDepth),
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
			l.extendHint("outputBytes", int64(l.MaxOutputBytes), int64(l.MaxOutputBytes)*4, int64(l.CapOutputBytes),
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
	l.peakDepth.Store(0)
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

// Note records a toolkit notice for the current eval; a repeated one is kept
// once.
func (l *ExecutionLimits) Note(msg string) {
	l.warnMu.Lock()
	defer l.warnMu.Unlock()
	if !slices.Contains(l.notes, msg) {
		l.notes = append(l.notes, msg)
	}
}

// Notes returns the notes recorded since ClearWarnings.
func (l *ExecutionLimits) Notes() []string {
	l.warnMu.Lock()
	defer l.warnMu.Unlock()
	return slices.Clone(l.notes)
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
	l.notes = nil
	l.rejections = &rejectionList{}
}

// LimitSpec is a set of limit values; nil fields are not set.
type LimitSpec struct {
	Steps       *int
	TimeoutMs   *int64
	CallDepth   *int
	OutputBytes *int
}

// Empty reports whether no field is set.
func (s LimitSpec) Empty() bool {
	return s.Steps == nil && s.TimeoutMs == nil && s.CallDepth == nil && s.OutputBytes == nil
}

// ParseDurationMs reads a Go duration ("60s", "5m") or plain milliseconds.
func ParseDurationMs(val string) (int64, error) {
	d, err := time.ParseDuration(val)
	if err != nil {
		n, nerr := strconv.ParseInt(val, 10, 64)
		if nerr != nil {
			return 0, fmt.Errorf("timeout %q is not a duration (60s) or milliseconds", val)
		}
		d = time.Duration(n) * time.Millisecond
	}
	if d <= 0 {
		return 0, fmt.Errorf("timeout must be positive")
	}
	return d.Milliseconds(), nil
}

// ParseLimitSpec parses "steps=N,timeout=60s,callDepth=N,outputBytes=N". who
// prefixes errors (the flag name).
func ParseLimitSpec(who, spec string) (LimitSpec, error) {
	var out LimitSpec
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			return out, fmt.Errorf("%s: %q is not key=value", who, part)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if key == "timeout" {
			ms, err := ParseDurationMs(val)
			if err != nil {
				return out, fmt.Errorf("%s: %v", who, err)
			}
			out.TimeoutMs = &ms
			continue
		}
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return out, fmt.Errorf("%s: %s %q is not a positive integer", who, key, val)
		}
		switch key {
		case "steps":
			out.Steps = &n
		case "callDepth":
			out.CallDepth = &n
		case "outputBytes":
			out.OutputBytes = &n
		default:
			return out, fmt.Errorf("%s: unknown key %q (use steps, timeout, callDepth, outputBytes)", who, key)
		}
	}
	return out, nil
}

// ParseCaps applies a "steps=N,timeout=60s,callDepth=N,outputBytes=N" spec (the
// --max-limits flag) to the operator maximums. Unlisted keys keep their cap.
// timeout takes a Go duration ("60s", "5m") or plain milliseconds.
func (l *ExecutionLimits) ParseCaps(spec string) error {
	s, err := ParseLimitSpec("max-limits", spec)
	if err != nil {
		return err
	}
	var steps, depth, out int
	var timeoutMs int64
	if s.Steps != nil {
		steps = *s.Steps
	}
	if s.CallDepth != nil {
		depth = *s.CallDepth
	}
	if s.OutputBytes != nil {
		out = *s.OutputBytes
	}
	if s.TimeoutMs != nil {
		timeoutMs = *s.TimeoutMs
	}
	l.SetCaps(steps, depth, timeoutMs, out)
	return nil
}

// CapError is the error for a limit value over its operator maximum.
func CapError(who, name string, n, cap int64) *ShellError {
	return Runtime(fmt.Sprintf("%s: %s (%d) exceeds the operator maximum (%d); the operator sets it with --max-limits %s=N", who, name, n, cap, name))
}

// Set applies spec to this eval's limits, as the eval arguments do: a value
// may be lower or higher than the current one, and over an operator maximum it
// is an error (not clamped). Nothing is applied when any value is over.
func (l *ExecutionLimits) Set(who string, s LimitSpec) error {
	check := func(name string, n, cap int64) error {
		if n <= 0 {
			return Runtime(fmt.Sprintf("%s: %s (%d) must be positive", who, name, n))
		}
		if n > cap {
			return CapError(who, name, n, cap)
		}
		return nil
	}
	var errs []error
	if s.Steps != nil {
		errs = append(errs, check("steps", int64(*s.Steps), int64(l.CapSteps)))
	}
	if s.TimeoutMs != nil {
		errs = append(errs, check("timeout", *s.TimeoutMs, l.CapTimeoutMs))
	}
	if s.CallDepth != nil {
		errs = append(errs, check("callDepth", int64(*s.CallDepth), int64(l.CapCallDepth)))
	}
	if s.OutputBytes != nil {
		errs = append(errs, check("outputBytes", int64(*s.OutputBytes), int64(l.CapOutputBytes)))
	}
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	if s.Steps != nil {
		l.MaxSteps = *s.Steps
	}
	if s.TimeoutMs != nil {
		l.TimeoutMs = *s.TimeoutMs
	}
	if s.CallDepth != nil {
		l.MaxCallDepth = *s.CallDepth
	}
	if s.OutputBytes != nil {
		l.MaxOutputBytes = *s.OutputBytes
	}
	return nil
}

// nearLimit is the share of a limit from which Usage reports it.
const nearLimit = 0.8

// Usage reports the limits that this eval hit or used at least 80% of, as
// "steps 850000/1000000 (85%)" parts, or nil. A hit one reads "(hit)".
func (l *ExecutionLimits) Usage() []string {
	var parts []string
	add := func(name string, used, limit int64, unit string) {
		if limit <= 0 {
			return
		}
		switch {
		case used > limit:
			parts = append(parts, fmt.Sprintf("%s %d%s/%d%s (hit)", name, limit, unit, limit, unit))
		case float64(used) >= nearLimit*float64(limit):
			parts = append(parts, fmt.Sprintf("%s %d%s/%d%s (%d%%)", name, used, unit, limit, unit, used*100/limit))
		}
	}
	add("steps", l.stepCount.Load(), int64(l.MaxSteps), "")
	add("time", nowMs()-l.startTimeMs.Load(), l.TimeoutMs, "ms")
	add("callDepth", l.peakDepth.Load(), int64(l.MaxCallDepth), "")
	add("outputBytes", l.outputBytes.Load(), int64(l.MaxOutputBytes), "")
	return parts
}
