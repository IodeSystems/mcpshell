package runtime

import (
	"strings"
	"testing"
)

func TestParseBytes(t *testing.T) {
	for in, want := range map[string]int64{
		"1048576": 1 << 20, "512MB": 512 << 20, "2GB": 2 << 30, "1.5gb": 3 << 29,
		"64 KiB": 64 << 10, "10b": 10, "1tb": 1 << 40,
	} {
		got, err := ParseBytes(in)
		if err != nil || got != want {
			t.Errorf("ParseBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "GB", "-1MB", "0", "abc", "1e30GB", "12 parsecs"} {
		if _, err := ParseBytes(bad); err == nil {
			t.Errorf("ParseBytes(%q): want an error", bad)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	for in, want := range map[int64]string{
		10: "10 B", 1 << 20: "1 MB", 412 << 20: "412 MB", 1 << 30: "1 GB",
		1100 << 20: "1.07 GB", 3 << 29: "1.5 GB", 15 << 20: "15 MB",
	} {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", in, got, want)
		}
	}
	if got := FormatBytesExact(2 << 30); got != "2GB" {
		t.Errorf("FormatBytesExact = %q", got)
	}
}

func TestMemoryCapsAndSpec(t *testing.T) {
	l := NewExecutionLimits()
	if l.MaxMemory != 1<<30 || l.CapMemory != 2<<30 {
		t.Fatalf("defaults: %d / %d", l.MaxMemory, l.CapMemory)
	}
	if err := l.ParseCaps("memory=4GB,steps=30000000"); err != nil {
		t.Fatal(err)
	}
	if l.CapMemory != 4<<30 || l.CapSteps != 30_000_000 {
		t.Errorf("caps: %d %d", l.CapMemory, l.CapSteps)
	}
	if err := l.ParseCaps("memory=100MB"); err != nil {
		t.Fatal(err)
	}
	if l.CapMemory != 1<<30 {
		t.Errorf("a cap below the default must rise to it, got %d", l.CapMemory)
	}
	if err := l.ParseCaps("memory=lots"); err == nil || !strings.Contains(err.Error(), "max-limits") {
		t.Errorf("bad size: %v", err)
	}
	s, err := ParseLimitSpec("limits", "memory=512MB,steps=5")
	if err != nil || s.MemoryBytes == nil || *s.MemoryBytes != 512<<20 {
		t.Fatalf("spec: %+v %v", s, err)
	}
	if err := l.Set("limits", s); err != nil || l.MaxMemory != 512<<20 {
		t.Errorf("Set: %v, MaxMemory %d", err, l.MaxMemory)
	}
	over := int64(8 << 30)
	err = l.Set("limits", LimitSpec{MemoryBytes: &over})
	if err == nil || !strings.Contains(err.Error(), "exceeds the operator maximum (1 GB)") || !strings.Contains(err.Error(), "--max-limits memory") {
		t.Errorf("over the cap: %v", err)
	}
	l.Reset()
	if l.MaxMemory != 1<<30 {
		t.Errorf("Reset must restore the default, got %d", l.MaxMemory)
	}
}

func TestEstimateSize(t *testing.T) {
	shared := &StringVal{V: strings.Repeat("x", 1000)}
	arr := &ArrayVal{Elements: []Value{shared, shared, shared}}
	n, cut := EstimateSize(arr, 1000)
	if cut || n < 1000 || n > 1500 {
		t.Errorf("a shared string counts once: %d (cut %v)", n, cut)
	}
	big := &ArrayVal{}
	for i := 0; i < 1000; i++ {
		big.Elements = append(big.Elements, Num(float64(i)))
	}
	n, cut = EstimateSize(big, 100)
	if !cut || n <= 0 {
		t.Errorf("a capped walk must say so: %d cut %v", n, cut)
	}
	self := &ArrayVal{}
	self.Elements = append(self.Elements, self)
	if _, cut := EstimateSize(self, 1000); cut {
		t.Error("a cycle must end by itself")
	}
}

// A tight loop is the worst case for the check: it runs on every 4096th step.
// Run with -bench MemoryCheck to compare the check on (default) and off.
func benchLoop(b *testing.B, mem int64) {
	sh := NewShell()
	sh.Limits().SetDefaults(1<<60, 256, 1<<40)
	sh.Limits().MaxMemory = mem
	sh.Limits().defaultMaxMemory = mem
	for b.Loop() {
		if _, err := sh.EvalExported("let s = 0; for (let i = 0; i < 200000; i++) { s += i }; s", nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemoryCheckOn(b *testing.B)  { benchLoop(b, DefaultMaxMemory) }
func BenchmarkMemoryCheckOff(b *testing.B) { benchLoop(b, 0) }

func BenchmarkHeapBytes(b *testing.B) {
	for b.Loop() {
		HeapBytes()
	}
}
