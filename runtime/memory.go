package runtime

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// Memory limit. The unit is bytes of Go heap objects (live objects plus
// garbage the sweeper has not freed). Over the limit, one forced GC runs and
// the heap is read again; only a heap that is still over the limit fails the
// eval. See docs/design.md "Memory".

const (
	DefaultMaxMemory = 1 << 30
	DefaultCapMemory = 2 << 30

	// memCheckMask: Step reads the heap every 4096 steps.
	memCheckMask = 4095
	// memForceGap is the least time between two forced GCs from Step.
	memForceGap = 100 * time.Millisecond
)

const heapObjectsMetric = "/memory/classes/heap/objects:bytes"

// HeapBytes is the bytes of heap objects now: live objects plus garbage not
// yet freed.
func HeapBytes() int64 {
	s := [1]metrics.Sample{{Name: heapObjectsMetric}}
	metrics.Read(s[:])
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return int64(s[0].Value.Uint64())
}

// liveHeap collects garbage and reads the heap again.
func liveHeap() int64 {
	runtime.GC()
	return HeapBytes()
}

// MemReporter builds the holders part of a memory report for a heap of used
// bytes and a limit; the shell sets it.
type MemReporter func(used, limit int64) string

// checkMemory is the periodic check from Step.
func (l *ExecutionLimits) checkMemory(line int) {
	max := l.MaxMemory
	if max <= 0 {
		return
	}
	if HeapBytes() <= max {
		return
	}
	// Garbage can push the heap over: collect, but not more often than
	// memForceGap, so a live heap near the limit does not cost a GC per check.
	now := time.Now().UnixNano()
	if last := l.lastForceNs.Load(); now-last < int64(memForceGap) {
		return
	}
	l.lastForceNs.Store(now)
	if used := liveHeap(); used > max {
		l.memoryError(used, 0, "", line)
	}
}

// AllocBytes is count*per, saturating at the largest int64, for estimates of
// counts that came from a script.
func AllocBytes(count, per int64) int64 {
	if count <= 0 || per <= 0 {
		return 0
	}
	if count > math.MaxInt64/per {
		return math.MaxInt64
	}
	return count * per
}

// BigResult is the size from which a result of a native command is checked
// against the memory limit: this many array elements or string bytes.
const BigResult = 1 << 16

// CheckResult checks the heap after a command returned a big array or
// string. Commands whose output grows with their input (concat, flat, map)
// are not estimated up front: a doubling loop would pass the limit in a few
// steps, before Step reads the heap. The check is after the allocation, so
// the heap can pass the limit by one result.
func (l *ExecutionLimits) CheckResult(v Value) {
	switch x := v.(type) {
	case *ArrayVal:
		if len(x.Elements) < BigResult {
			return
		}
	case *StringVal:
		if len(x.V) < BigResult*16 {
			return
		}
	default:
		return
	}
	if max := l.MaxMemory; max > 0 && HeapBytes() > max {
		if used := liveHeap(); used > max {
			l.memoryError(used, 0, "", 0)
		}
	}
}

// CheckAlloc refuses an allocation of about est bytes that would take the
// heap over the limit, before the memory is allocated. what names it for the
// error, such as "range(0, 10000000000)".
func (l *ExecutionLimits) CheckAlloc(est int64, what string) {
	max := l.MaxMemory
	if max <= 0 || est <= 0 {
		return
	}
	used := HeapBytes()
	if est <= max-used {
		return
	}
	used = liveHeap()
	if est > max-used {
		l.memoryError(used, est, what, 0)
	}
}

// memoryError raises the memory error with the holders report.
func (l *ExecutionLimits) memoryError(used, want int64, what string, line int) {
	l.memHit.Store(true)
	var b strings.Builder
	if want > 0 {
		fmt.Fprintf(&b, "memory limit exceeded: %s needs ~%s; %s of %s live heap is in use",
			what, FormatBytes(want), FormatBytes(used), FormatBytes(l.MaxMemory))
	} else {
		fmt.Fprintf(&b, "memory limit exceeded: %s of %s live heap", FormatBytes(used), FormatBytes(l.MaxMemory))
		if line > 0 {
			fmt.Fprintf(&b, " at line %d", line)
		}
	}
	if l.memReport != nil {
		b.WriteString("\n")
		b.WriteString(l.memReport(used, l.MaxMemory))
	}
	b.WriteString(l.memoryHint(used + want))
	panic(Runtime(b.String()))
}

// memoryHint is the end of a memory error: how to raise the limit, or that it
// cannot be raised. need is the heap the work needs in all, or 0 if unknown.
func (l *ExecutionLimits) memoryHint(need int64) string {
	head := "\n\n  Common fixes:\n"
	switch {
	case need > l.CapMemory:
		return head + fmt.Sprintf("    - The work needs ~%s; the operator maximum is %s. Reduce the work (limit, chunk, a smaller range)", FormatBytes(need), FormatBytes(l.CapMemory))
	case l.MaxMemory >= l.CapMemory:
		return head + fmt.Sprintf("    - memory is at the operator maximum (%s); it cannot be raised. Free the holders above or reduce the work", FormatBytes(l.CapMemory))
	}
	want := min((max(l.MaxMemory*2, need)+(1<<20)-1)>>20<<20, l.CapMemory)
	return head + fmt.Sprintf("    - Free the holders above, or if the data is needed, %s (operator maximum: %s)",
		l.raiseMemoryHint(want), FormatBytes(l.CapMemory))
}

func (l *ExecutionLimits) raiseMemoryHint(n int64) string {
	v := FormatBytesExact(n)
	switch l.Hint {
	case HintEvalArg:
		return fmt.Sprintf("re-run with the eval argument limits: {memory: %q}", v)
	case HintFlag:
		return "re-run with --limits memory=" + v
	}
	return fmt.Sprintf("use extendLimit({memory: %q})", v)
}

// MemoryNote returns the report for the end of an eval when the heap holds
// at least 80% of the limit after a GC, or "". An eval that failed with the
// memory error has its own report.
func (l *ExecutionLimits) MemoryNote() string {
	max := l.MaxMemory
	if max <= 0 || l.memHit.Load() {
		return ""
	}
	if float64(HeapBytes()) < nearLimit*float64(max) {
		return ""
	}
	used := liveHeap()
	if float64(used) < nearLimit*float64(max) {
		return ""
	}
	s := fmt.Sprintf("memory %s of %s live heap (%d%%)", FormatBytes(used), FormatBytes(max), used*100/max)
	if l.memReport != nil {
		s += "\n" + l.memReport(used, max)
	}
	return s
}

// applyGCTarget sets the Go runtime's soft memory limit to 90% of the
// eval's memory limit, so the collector works harder as the heap nears the
// limit instead of letting garbage pile up. GCTarget is off for embedding
// hosts (the setting is process-wide); a set GOMEMLIMIT is the operator's and
// stays.
func (l *ExecutionLimits) applyGCTarget() {
	if !l.GCTarget || l.MaxMemory <= 0 || os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	debug.SetMemoryLimit(l.MaxMemory)
}

// --- sizes ---------------------------------------------------------------------

// FormatBytes renders a size for people: "412 MB", "1.02 GB". Units are 1024.
func FormatBytes(n int64) string {
	const k = 1024
	f := float64(n)
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return trimUnit(f/k, "KB")
	case n < k*k*k:
		return trimUnit(f/(k*k), "MB")
	case n < k*k*k*k:
		return trimUnit(f/(k*k*k), "GB")
	}
	return trimUnit(f/(k*k*k*k), "TB")
}

func trimUnit(v float64, unit string) string {
	switch {
	case v >= 100:
		return fmt.Sprintf("%.0f %s", v, unit)
	case v >= 10:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") + " " + unit
	}
	s := fmt.Sprintf("%.2f", v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s + " " + unit
}

// FormatBytesExact renders n in the largest unit that divides it, such as
// "2GB" or "1536MB", the form ParseBytes reads back.
func FormatBytesExact(n int64) string {
	for _, u := range []struct {
		s string
		n int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}} {
		if n%u.n == 0 {
			return strconv.FormatInt(n/u.n, 10) + u.s
		}
	}
	return strconv.FormatInt(n, 10)
}

// ParseBytes reads a size: plain bytes ("1048576") or a number with a unit
// B, KB, MB, GB, TB (also KiB, MiB, …; case does not matter). Units are 1024.
func ParseBytes(val string) (int64, error) {
	s := strings.ToLower(strings.TrimSpace(val))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"tib", 1 << 40}, {"gib", 1 << 30}, {"mib", 1 << 20}, {"kib", 1 << 10},
		{"tb", 1 << 40}, {"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10}, {"b", 1}} {
		if t, ok := strings.CutSuffix(s, u.suffix); ok {
			s, mult = strings.TrimSpace(t), u.mult
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 || math.IsInf(f, 0) || f*float64(mult) >= math.MaxInt64 {
		return 0, fmt.Errorf("memory %q is not a positive size (1073741824, 512MB, 2GB)", val)
	}
	return int64(f * float64(mult)), nil
}

// Bytes per value, for estimates. A number is a 64-byte struct, a string a
// 16-byte header plus its bytes; an array slot is a 16-byte interface.
const (
	sizeIface  = 16
	sizeNumber = int64(unsafe.Sizeof(NumberVal{}))
	sizeString = int64(unsafe.Sizeof(StringVal{}))
	sizeOther  = 128
	// NumberElem is the estimate for one number in an array.
	NumberElem = sizeIface + 64
	// SlotElem is the estimate for one array slot that holds a shared value.
	SlotElem = sizeIface
)

// arraySample is how many elements of a large array the estimate walks.
const arraySample = 1024

// objectSize is the bytes of an object with n keys, without the keys' text
// and the values. Measured with go1.27 on amd64 (100k objects, heap after
// GC): {} 107 B; 1, 2, 4, 8 keys 416, 432, 472, 552 B; 9 keys 1016 B. A map
// allocates a group of 8 slots (~300 B) for its first key, and doubles its
// groups past 8.
func objectSize(n int) int64 {
	size := int64(107)
	if n > 0 {
		groups := int64(1)
		for groups*8 < int64(n) {
			groups *= 2
		}
		size += 300*groups + 20*int64(n)
	}
	return size
}

// sizeWalk is the state of one size estimate: shared values count once, and
// the walk stops after max values.
type sizeWalk struct {
	seen  map[Value]struct{}
	nodes int
	max   int
	cut   bool
}

// EstimateSize estimates the heap bytes a value holds, walking at most
// maxNodes values. cut is true when the walk stopped early: the size is then
// a lower bound. Shared arrays, objects and long strings count once. It is an
// estimate: Go allocator rounding and map load are guessed.
func EstimateSize(v Value, maxNodes int) (bytes int64, cut bool) {
	sizes, cut := EstimateSizes([]Value{v}, maxNodes)
	return sizes[0], cut
}

// EstimateSizes is EstimateSize for several values that may share parts: a
// shared part counts for the first value that reaches it.
func EstimateSizes(vs []Value, maxNodes int) (sizes []int64, cut bool) {
	w := &sizeWalk{seen: map[Value]struct{}{}, max: maxNodes}
	for _, v := range vs {
		sizes = append(sizes, w.size(v))
	}
	return sizes, w.cut
}

func (w *sizeWalk) size(v Value) int64 {
	if w.nodes >= w.max {
		w.cut = true
		return 0
	}
	w.nodes++
	switch x := v.(type) {
	case *NumberVal:
		n := sizeNumber
		if x.bigI != nil {
			n += int64(x.bigI.BitLen())/8 + 32
		}
		if x.rat != nil {
			n += int64(x.rat.Num().BitLen()+x.rat.Denom().BitLen())/8 + 64
		}
		return n
	case *StringVal:
		if len(x.V) > 64 {
			if _, ok := w.seen[v]; ok {
				return 0
			}
			w.seen[v] = struct{}{}
		}
		return sizeString + int64(len(x.V))
	case *BoolVal, *NullVal, nil:
		return 0
	case *ArrayVal:
		if _, ok := w.seen[v]; ok {
			return 0
		}
		w.seen[v] = struct{}{}
		n := int64(unsafe.Sizeof(ArrayVal{})) + int64(cap(x.Elements))*sizeIface
		if len(x.Elements) > arraySample*2 {
			// A large array is sampled, so one array does not use the walk
			// budget and the other holders still get an estimate.
			step := len(x.Elements) / arraySample
			var sum int64
			k := 0
			for i := 0; i < len(x.Elements) && !w.cut; i += step {
				sum += w.size(x.Elements[i])
				k++
			}
			if k > 0 {
				n += sum * int64(len(x.Elements)) / int64(k)
			}
			return n
		}
		for _, e := range x.Elements {
			n += w.size(e)
			if w.cut {
				break
			}
		}
		return n
	case *ObjectVal:
		if _, ok := w.seen[v]; ok {
			return 0
		}
		w.seen[v] = struct{}{}
		n := objectSize(len(x.m))
		for k, e := range x.m { // hidden (class methods) and accessors too
			n += int64(len(k)) + w.size(e)
			if w.cut {
				return n
			}
		}
		for _, fields := range x.priv {
			n += objectSize(len(fields))
			for _, e := range fields {
				n += w.size(e)
			}
		}
		if x.proto != nil {
			n += w.size(x.proto) // shared by instances: counted once
		}
		return n
	case *MapVal:
		if _, ok := w.seen[v]; ok {
			return 0
		}
		w.seen[v] = struct{}{}
		// a Go map entry per key plus the two slices.
		n := objectSize(x.live) + int64(cap(x.keys)+cap(x.vals))*sizeIface
		for i, k := range x.keys {
			if k == nil {
				continue
			}
			n += w.size(k)
			if !x.isSet() {
				n += w.size(x.vals[i])
			}
			if w.cut {
				break
			}
		}
		return n
	case *FuncVal:
		if x.obj == nil {
			return sizeOther
		}
		if _, ok := w.seen[v]; ok {
			return 0
		}
		w.seen[v] = struct{}{}
		return sizeOther + w.size(x.obj)
	case *accessorVal:
		return sizeOther
	}
	return sizeOther
}

// --- holders -------------------------------------------------------------------

// MemCoster is implemented by the closer of a loadable that holds memory
// across evals. MemCost reports it, so the memory report can name the
// toolkit and the action that frees it.
type MemCoster interface {
	MemCost() MemCost
}

// MemCost is what a toolkit holds. OffHeap is true when the memory is not in
// the Go heap (SQLite pages, an external process): the limit does not count
// it, but the report names it.
type MemCost struct {
	Bytes   int64
	Detail  string // such as `3 tables; largest "events" 380 MB`
	Free    string // such as `Db.drop("events") or unload("db")`
	OffHeap bool
}

type memHolder struct {
	name    string
	bytes   int64
	approx  bool // an estimate from a walk of values
	cut     bool // the walk stopped early: a lower bound
	offHeap bool
	detail  string
	free    string
}

const (
	memWalkNodes  = 2_000_000
	memShownLines = 8
	memSmall      = 1 << 20 // holders under this are summed, not listed
)

// memoryReport lists what holds memory across evals, largest first, then the
// rest as this eval's. It is the body of every memory report.
func (t *Shell) memoryReport(used, limit int64) string {
	var hs []memHolder

	r := t.commands
	r.mu.RLock()
	var coster []struct {
		name string
		c    MemCoster
	}
	for name, l := range r.loadables {
		if l.loaded == nil {
			continue
		}
		if c, ok := l.loaded.closer.(MemCoster); ok {
			coster = append(coster, struct {
				name string
				c    MemCoster
			}{name, c})
		}
	}
	r.mu.RUnlock()
	for _, c := range coster {
		mc := c.c.MemCost()
		hs = append(hs, memHolder{name: c.name, bytes: mc.Bytes, offHeap: mc.OffHeap, detail: mc.Detail, free: mc.Free})
	}

	kept := map[string]bool{}
	for _, k := range t.kept {
		kept[k] = true
	}
	binds := t.globals.UserBindings()
	names := make([]string, 0, len(binds))
	for name := range binds {
		names = append(names, name)
	}
	sort.Strings(names)
	vals := make([]Value, len(names))
	for i, n := range names {
		vals[i] = binds[n]
	}
	sizes, cut := EstimateSizes(vals, memWalkNodes)
	for i, n := range names {
		h := memHolder{bytes: sizes[i], approx: true, cut: cut, detail: describeValue(vals[i])}
		if kept[n] {
			h.name = n
			h.detail = "kept result; " + h.detail
			h.free = fmt.Sprintf("forget(%q)", n)
		} else {
			h.name = n
			h.free = fmt.Sprintf("forget(%q)", n)
		}
		hs = append(hs, h)
	}

	sort.SliceStable(hs, func(i, j int) bool { return hs[i].bytes > hs[j].bytes })
	var b strings.Builder
	var onHeap int64
	hidden, hiddenBytes := 0, int64(0)
	for i, h := range hs {
		if !h.offHeap {
			onHeap += h.bytes
		}
		if i >= memShownLines || (h.bytes < memSmall && i > 0) {
			hidden++
			hiddenBytes += h.bytes
			continue
		}
		pre := ""
		if h.approx {
			pre = "~"
			if h.cut {
				pre = "≥~"
			}
		}
		fmt.Fprintf(&b, "  %s: %s%s", h.name, pre, FormatBytes(h.bytes))
		var extra []string
		if h.offHeap {
			extra = append(extra, "off-heap, not in the limit")
		}
		if h.detail != "" {
			extra = append(extra, h.detail)
		}
		if len(extra) > 0 {
			b.WriteString(" (" + strings.Join(extra, "; ") + ")")
		}
		if h.free != "" {
			b.WriteString(" — " + h.free)
		}
		b.WriteString("\n")
	}
	if hidden > 0 {
		fmt.Fprintf(&b, "  %d smaller holders: ~%s together\n", hidden, FormatBytes(hiddenBytes))
	}
	if rest := used - onHeap; rest > 0 {
		fmt.Fprintf(&b, "  this eval: the rest (~%s) — values the code holds; process in parts (limit, chunk) or drop references", FormatBytes(rest))
	} else {
		b.WriteString("  this eval: nothing beyond the holders above")
	}
	if t.limits.Hint == HintEvalArg {
		b.WriteString("\n  all of it: eval with clear: true (unloads toolkits, drops globals and kept results)")
	}
	return b.String()
}

// describeValue is a short description of a value: type and size.
func describeValue(v Value) string {
	switch x := v.(type) {
	case *ArrayVal:
		return fmt.Sprintf("Array, %s elements", Count(len(x.Elements)))
	case *ObjectVal:
		if x.file != nil {
			return fileInspect(x)
		}
		return fmt.Sprintf("Object, %d keys", x.Len())
	case *StringVal:
		return fmt.Sprintf("String, %s characters", Count(len(x.V)))
	}
	return v.TypeName()
}

// Count renders a count: 950, 1.2k, 3.4M.
func Count(n int) string {
	switch {
	case n >= 10_000_000:
		return fmt.Sprintf("%dM", n/1_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}

// ForgetResult drops a kept result $rN and returns the bytes it held, by
// estimate. An unknown name is an error that names the results still kept.
func (t *Shell) ForgetResult(name string) (int64, error) {
	idx := -1
	for i, k := range t.kept {
		if k == name {
			idx = i
		}
	}
	if idx < 0 {
		if len(t.kept) == 0 {
			return 0, fmt.Errorf("forget: %q is not a kept result; none are kept", name)
		}
		return 0, fmt.Errorf("forget: %q is not a kept result; kept: %s", name, strings.Join(t.kept, ", "))
	}
	n := int64(0)
	if v := t.globals.Get(name); v != nil {
		n, _ = EstimateSize(v, memWalkNodes)
	}
	t.globals.Remove(map[string]struct{}{name: {}})
	t.kept = append(t.kept[:idx:idx], t.kept[idx+1:]...)
	return n, nil
}
