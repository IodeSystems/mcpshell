// Command bench runs the mcpshell LLM benchmark suite against an
// OpenAI-compatible chat-completions API. It is a side tool, separate from the
// mcpshell CLI itself.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iodesystems/mcpshell/bench"
	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

const usage = `bench — run the mcpshell LLM benchmark suite

Usage: bench [flags]

Connection settings are read from env.local (gitignored) or the environment:
MCPSHELL_LLM_URL, MCPSHELL_LLM_MODEL, MCPSHELL_LLM_API_KEY.`

func main() {
	_ = bench.LoadEnvFile("env.local")

	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		fmt.Fprintln(os.Stderr, "\nFlags:")
		fs.PrintDefaults()
	}
	url := fs.String("url", os.Getenv("MCPSHELL_LLM_URL"), "LLM API base URL (or MCPSHELL_LLM_URL)")
	model := fs.String("model", os.Getenv("MCPSHELL_LLM_MODEL"), "model id or substring (or MCPSHELL_LLM_MODEL)")
	compact := fs.Bool("compact", false, "use the compact system prompt")
	failFast := fs.Bool("fail-fast", false, "stop on the first failure")
	timeout := fs.Int("timeout", 30, "per-teaser timeout in seconds")
	maxIters := fs.Int("max-iters", 50, "max agent iterations per teaser")
	out := fs.String("out", "", "output directory (default benchmarks/results[-compact])")
	only := fs.String("only", "", "run only teasers whose name contains this substring")
	_ = fs.Parse(os.Args[1:])

	if *url == "" {
		fail("no LLM URL — set MCPSHELL_LLM_URL in env.local or pass --url")
	}
	if *model == "" {
		fail("no model — set MCPSHELL_LLM_MODEL in env.local or pass --model")
	}
	outDir := *out
	if outDir == "" {
		outDir = "benchmarks/results"
		if *compact {
			outDir += "-compact"
		}
	}

	llm := bench.NewLLM(*url, os.Getenv("MCPSHELL_LLM_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	resolved, err := llm.ResolveModel(ctx, *model)
	cancel()
	if err != nil {
		fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "bench: model %q at %s\n", resolved, *url)

	if err := bench.Run(llm, resolved, benchShell, bench.Options{
		SystemPrompt: benchSystemPrompt(*compact),
		OutputDir:    outDir,
		TimeoutSec:   *timeout,
		MaxIters:     *maxIters,
		FailFast:     *failFast,
		Only:         *only,
	}); err != nil {
		fail("benchmark: %v", err)
	}
}

// benchShell builds the shell used for each benchmark teaser (core + math).
func benchShell() *runtime.Shell {
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	return sh
}

// benchSystemPrompt builds the benchmark system prompt: the mcpshell reference
// plus instructions to always compute via the tool and return the raw value.
func benchSystemPrompt(compact bool) string {
	var b strings.Builder
	b.WriteString("You are a helpful assistant with access to mcpshell via the mcpshell tool.\n")
	b.WriteString("IMPORTANT: mcpshell is NOT JavaScript. Read the reference below carefully before writing code.\n")
	b.WriteString("Use the mcpshell tool to execute mcpshell code when asked to compute, transform, or query data.\n\n")
	b.WriteString(benchShell().ToPrompt(compact))
	b.WriteString("\n\nIMPORTANT: You MUST call the mcpshell tool for every question — never answer from memory. ")
	b.WriteString("Even if you know the answer, compute it with mcpshell to verify. ")
	b.WriteString("Return ONLY the raw result from the tool — no explanation, no markdown, no wrapping. Just the value.")
	return b.String()
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "bench: "+format+"\n", a...)
	os.Exit(1)
}
