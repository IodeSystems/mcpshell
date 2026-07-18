package toolkit_test

import (
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// Interpreter CPU benchmarks over representative hot paths. Each measures the
// full eval path (parse + run) on a fresh core+math shell, as the eval tool
// does. Run: go test -bench=Interp -benchmem ./toolkit/
func benchEval(b *testing.B, src string) {
	b.Helper()
	for i := 0; i < b.N; i++ {
		sh := toolkit.InstallMath(toolkit.InstallCore(runtime.NewShell()))
		if _, err := sh.Eval(src); err != nil {
			b.Fatalf("eval: %v", err)
		}
	}
}

func BenchmarkInterpArithLoop(b *testing.B) {
	benchEval(b, `let s = 0; for (let i = 0; i < 5000; i = i + 1) { s = s + i * 2 - 1 }; s`)
}
func BenchmarkInterpBigIntFactorial(b *testing.B) {
	benchEval(b, `let f = 1; for (let i = 1; i <= 200; i = i + 1) { f = f * i }; f`)
}
func BenchmarkInterpExactDecimal(b *testing.B) {
	benchEval(b, `let s = 0; for (let i = 0; i < 2000; i = i + 1) { s = s + 0.1 }; s`)
}
func BenchmarkInterpRecursion(b *testing.B) {
	benchEval(b, `function fib(n) { if (n < 2) { return n } return fib(n - 1) + fib(n - 2) } fib(24)`)
}
func BenchmarkInterpString(b *testing.B) {
	benchEval(b, `let r = ""; for (let i = 0; i < 1000; i = i + 1) { r = "the quick brown fox" |> split(" ") |> reverse() |> join("-") }; r`)
}
func BenchmarkInterpPipeline(b *testing.B) {
	benchEval(b, `range(1, 5000) |> map(x => x * x) |> filter(x => x % 2 == 0) |> reduce((a, x) => a + x, 0)`)
}
func BenchmarkInterpObject(b *testing.B) {
	benchEval(b, `let o = {}; for (let i = 0; i < 2000; i = i + 1) { o = {n: i, sq: i * i, ok: i % 2 == 0}; }; o.sq`)
}
func BenchmarkInterpRegex(b *testing.B) {
	benchEval(b, `let c = 0; for (let i = 0; i < 1000; i = i + 1) { if ("user@example.com" |> test(/^[^@]+@[^@]+$/)) { c = c + 1 } }; c`)
}
