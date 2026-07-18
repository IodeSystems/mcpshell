// Command bench runs the mcpshell LLM benchmark suite against an
// OpenAI-compatible chat-completions API. It is a side tool, separate from the
// mcpshell CLI itself.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	if len(os.Args) > 1 && os.Args[1] == "compare" {
		runCompare(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "context" {
		runContext(os.Args[2:])
		return
	}
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
	noTool := fs.Bool("no-tool", false, "reasoning-only baseline: run without the mcpshell tool")
	failFast := fs.Bool("fail-fast", false, "stop on the first failure")
	timeout := fs.Int("timeout", 30, "per-teaser timeout in seconds")
	maxIters := fs.Int("max-iters", 50, "max agent iterations per teaser")
	out := fs.String("out", "", "output directory (default benchmarks/results[-compact])")
	only := fs.String("only", "", "run only teasers whose name contains this substring")
	label := fs.String("label", "", "name used in output paths/reports (default: resolved model id)")
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
		if *noTool {
			outDir += "-notool"
		}
	}
	// Seed the SQLite fixture used by the SQL composition teasers, then build a
	// shell factory that attaches it read-only under the `shop` namespace.
	dbPath := filepath.Join(os.TempDir(), "mcpshell-bench-fixture.sqlite")
	_ = os.Remove(dbPath)
	if err := bench.SeedSQLite(dbPath); err != nil {
		fail("seed fixture: %v", err)
	}
	shellFactory := newBenchShell(dbPath)

	systemPrompt := benchSystemPrompt(*compact, shellFactory)
	if *noTool {
		systemPrompt = noToolSystemPrompt()
	}

	llm := bench.NewLLM(*url, os.Getenv("MCPSHELL_LLM_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	resolved, err := llm.ResolveModel(ctx, *model)
	cancel()
	if err != nil {
		fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "bench: model %q at %s\n", resolved, *url)

	if err := bench.Run(llm, resolved, shellFactory, bench.Options{
		SystemPrompt: systemPrompt,
		OutputDir:    outDir,
		TimeoutSec:   *timeout,
		MaxIters:     *maxIters,
		FailFast:     *failFast,
		Only:         *only,
		NoTool:       *noTool,
		Label:        *label,
	}); err != nil {
		fail("benchmark: %v", err)
	}
}

// newBenchShell returns a factory building the shell used for each teaser:
// core + math, plus the seeded SQLite fixture attached read-only as `shop`.
func newBenchShell(dbPath string) func() *runtime.Shell {
	return func() *runtime.Shell {
		sh := runtime.NewShell()
		toolkit.InstallCore(sh)
		toolkit.InstallMath(sh)
		if _, err := toolkit.InstallSQL(sh, "shop", dbPath, true); err != nil {
			fail("attach fixture: %v", err)
		}
		return sh
	}
}

// benchSystemPrompt builds the benchmark system prompt: the mcpshell reference
// plus instructions to always compute via the tool and return the raw value.
func benchSystemPrompt(compact bool, shellFactory func() *runtime.Shell) string {
	var b strings.Builder
	b.WriteString("You are a helpful assistant with access to mcpshell via the mcpshell tool.\n")
	b.WriteString("IMPORTANT: mcpshell is NOT JavaScript. Read the reference below carefully before writing code.\n")
	b.WriteString("Use the mcpshell tool to execute mcpshell code when asked to compute, transform, or query data.\n\n")
	b.WriteString(shellFactory().ToPrompt(compact))
	b.WriteString("\n\nIMPORTANT: You MUST call the mcpshell tool for every question — never answer from memory. ")
	b.WriteString("Even if you know the answer, compute it with mcpshell to verify. ")
	b.WriteString("Return ONLY the raw result from the tool — no explanation, no markdown, no wrapping. Just the value.")
	return b.String()
}

// runContext measures the per-request context cost of exposing capabilities as
// N discrete MCP tools vs. one mcpshell eval tool, using the model's tokenizer.
//
//	bench context
func runContext(args []string) {
	_ = bench.LoadEnvFile("env.local")
	fs := flag.NewFlagSet("bench context", flag.ExitOnError)
	url := fs.String("url", os.Getenv("MCPSHELL_LLM_URL"), "LLM API base URL")
	model := fs.String("model", os.Getenv("MCPSHELL_LLM_MODEL"), "model id or substring")
	_ = fs.Parse(args)

	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	toolkit.InstallWeb(sh)
	toolkit.InstallGraph(sh)

	llm := bench.NewLLM(*url, os.Getenv("MCPSHELL_LLM_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	resolved, err := llm.ResolveModel(ctx, *model)
	if err != nil {
		fail("%v", err)
	}
	rows, err := bench.MeasureContext(ctx, llm, resolved, sh)
	if err != nil {
		fail("measure context: %v", err)
	}
	fmt.Printf("Per-request context cost (prompt tokens above baseline), model %q:\n\n", resolved)
	fmt.Printf("%-38s %6s  %s\n", "Strategy", "Tools", "Tokens")
	fmt.Println(strings.Repeat("-", 58))
	for _, r := range rows {
		fmt.Printf("%-38s %6d  %d\n", r.Strategy, r.Tools, r.Tokens)
	}
}

// runCompare builds a with/without comparison doc from two runs' results.json.
//
//	bench compare <with_dir> <without_dir> [flags]
func runCompare(args []string) {
	fs := flag.NewFlagSet("bench compare", flag.ExitOnError)
	out := fs.String("out", "", "output path (default <with_dir>/../comparison.md)")
	title := fs.String("title", "With vs. without mcpshell", "comparison title")
	label := fs.String("label", "model", "model name shown in the comparison")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "bench compare <with_dir> <without_dir> [flags]\n\n"+
			"Both dirs must contain a results.json (written by a benchmark run).")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 2 {
		fs.Usage()
		os.Exit(2)
	}
	withDir, withoutDir := fs.Arg(0), fs.Arg(1)
	outPath := *out
	if outPath == "" {
		outPath = filepath.Join(filepath.Dir(withDir), "comparison.md")
	}
	if err := bench.Compare(withDir, withoutDir, outPath, *title, *label); err != nil {
		fail("compare: %v", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)
}

// noToolSystemPrompt is the reasoning-only baseline: no tool is offered, so the
// model must compute the answer itself and return the raw value.
func noToolSystemPrompt() string {
	return "You are a helpful assistant. Answer the question directly. " +
		"Work out the exact answer yourself. " +
		"Return ONLY the raw final value — no explanation, no markdown, no wrapping. Just the value."
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "bench: "+format+"\n", a...)
	os.Exit(1)
}
