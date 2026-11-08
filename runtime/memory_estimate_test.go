package runtime_test

import (
	"runtime"
	"testing"

	mr "github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// EstimateSize stays near the measured heap, so the memory report names the
// holder that really holds the memory. A 2M-row array was reported at a
// third of its size (the walk budget cut it) and the rest showed as "this
// eval", which points the agent at the wrong thing.
func TestEstimateSizeNearMeasured(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~250 MB")
	}
	for _, code := range []string{
		`range(0, 200000).map(i => ({i, s: "x" + i}))`,
		`range(0, 200000).map(i => ({a: null, b: null, c: null, d: null, e: null, f: null, g: null, h: null, j: null}))`,
		`range(0, 200000).map(i => "row " + i)`,
		`range(0, 200000).map(i => [i, i])`,
	} {
		sh := mr.NewShell()
		toolkit.InstallCore(sh)
		sh.Limits().MaxSteps = 1e8
		before := liveHeap()
		v, err := sh.Eval(code)
		if err != nil {
			t.Fatal(err)
		}
		real := float64(liveHeap() - before)
		est, cut := mr.EstimateSize(v, 2_000_000)
		runtime.KeepAlive(v)
		if r := real / float64(est); cut || r < 0.6 || r > 1.6 {
			t.Errorf("%s: measured %.1f MB, estimate %.1f MB (ratio %.2f, cut %v)", code, real/1e6, float64(est)/1e6, r, cut)
		}
	}
}
