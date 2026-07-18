package runtime_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
)

// BenchmarkHotLoop exercises the interpreter's arithmetic/loop hot path.
func BenchmarkHotLoop(b *testing.B) {
	src := `let s = 0; for (let i = 0; i < 20000; i = i + 1) { s = s + i * 2 - 1 }; s`
	for i := 0; i < b.N; i++ {
		sh := runtime.NewShell()
		if _, err := sh.Eval(src); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSmallIntLoop stresses small-integer results (interning target).
func BenchmarkSmallIntLoop(b *testing.B) {
	src := `let c = 0; for (let i = 0; i < 20000; i = i + 1) { c = i % 100 }; c`
	for i := 0; i < b.N; i++ {
		sh := runtime.NewShell()
		if _, err := sh.Eval(src); err != nil {
			b.Fatal(err)
		}
	}
}
