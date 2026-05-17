package bench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iodesystems/mcpshell/runtime"
)

const toolMaxOutput = 16_000

// Options configures a benchmark run.
type Options struct {
	SystemPrompt string
	OutputDir    string
	TimeoutSec   int // default per-teaser timeout
	MaxIters     int
	FailFast     bool
	Only         string // if set, run only teasers whose name contains this
}

// Result is the outcome of one teaser.
type Result struct {
	Teaser      Teaser
	Success     bool
	Attempts    []Attempt
	DurationMs  int64
	FinalAnswer string
	Error       string
}

// Run executes the benchmark suite against the LLM and writes markdown results.
func Run(llm *LLM, model string, shellFactory func() *runtime.Shell, opts Options) error {
	if opts.MaxIters <= 0 {
		opts.MaxIters = 50
	}
	if opts.TimeoutSec <= 0 {
		opts.TimeoutSec = 30
	}

	suite := Suite
	if opts.Only != "" {
		var filtered []Teaser
		for _, teaser := range Suite {
			if strings.Contains(teaser.Name, opts.Only) {
				filtered = append(filtered, teaser)
			}
		}
		suite = filtered
		if len(suite) == 0 {
			return fmt.Errorf("no teaser name contains %q", opts.Only)
		}
	}

	fmt.Printf("Running %d benchmarks against %s (model: %s)\n", len(suite), llm.BaseURL, model)
	fmt.Println(strings.Repeat("-", 60))

	var results []Result
	for i, teaser := range suite {
		fmt.Printf("[%d/%d] %s ... ", i+1, len(suite), teaser.Name)

		timeout := opts.TimeoutSec
		if teaser.TimeoutSec > 0 {
			timeout = teaser.TimeoutSec
		}

		sh := shellFactory()
		runTool := func(code string) string {
			v, err := sh.EvalExported(code, nil)
			if err != nil {
				return "ERROR: " + err.Error()
			}
			out := v.Display()
			if len(out) > toolMaxOutput {
				out = out[:toolMaxOutput] + "\n\n... OUTPUT TRUNCATED"
			}
			return out
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		start := time.Now()
		answer, attempts, err := llm.RunAgent(ctx, model, opts.SystemPrompt,
			teaser.Prompt+"\n"+teaser.FormatHint, runTool, opts.MaxIters)
		cancel()

		res := Result{
			Teaser:      teaser,
			Attempts:    attempts,
			DurationMs:  time.Since(start).Milliseconds(),
			FinalAnswer: answer,
		}
		switch {
		case err != nil && errors.Is(err, context.DeadlineExceeded):
			res.Error = fmt.Sprintf("TIMEOUT (%ds)", timeout)
		case err != nil:
			res.Error = err.Error()
		default:
			res.Success = teaser.Validate(answer)
		}
		results = append(results, res)

		switch {
		case res.Success:
			fmt.Printf("PASS (%d tool calls, %dms)\n", len(attempts), res.DurationMs)
		case res.Error != "":
			fmt.Printf("ERROR — %s (%d tool calls, %dms)\n", res.Error, len(attempts), res.DurationMs)
		default:
			fmt.Printf("FAIL (%d tool calls, %dms)\n", len(attempts), res.DurationMs)
		}

		if opts.FailFast && !res.Success {
			fmt.Println("\nFAIL-FAST: stopping after first failure")
			break
		}
	}

	fmt.Println(strings.Repeat("-", 60))
	if err := writeResults(results, model, opts.OutputDir); err != nil {
		return err
	}
	passed := 0
	for _, r := range results {
		if r.Success {
			passed++
		}
	}
	fmt.Printf("Score: %d/%d passed\n", passed, len(results))
	return nil
}

func writeResults(results []Result, model, outputDir string) error {
	modelDir := filepath.Join(outputDir, sanitizeModel(model))
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return err
	}
	for _, r := range results {
		path := filepath.Join(modelDir, r.Teaser.Name+".md")
		if err := os.WriteFile(path, []byte(renderResult(r)), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(outputDir, "README.md"),
		[]byte(renderIndex(results, model)), 0o644); err != nil {
		return err
	}
	fmt.Printf("Results written to %s\n", modelDir)
	return nil
}

func sanitizeModel(model string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, model)
}

func renderResult(r Result) string {
	var b strings.Builder
	status := "FAIL"
	if r.Success {
		status = "PASS"
	}
	fmt.Fprintf(&b, "# %s\n\n", r.Teaser.Name)
	fmt.Fprintf(&b, "**Status:** %s\n", status)
	fmt.Fprintf(&b, "**Duration:** %dms\n", r.DurationMs)
	fmt.Fprintf(&b, "**Tool calls:** %d\n", len(r.Attempts))
	if r.Error != "" {
		fmt.Fprintf(&b, "**Error:** %s\n", r.Error)
	}
	fmt.Fprintf(&b, "\n## Prompt\n\n> %s\n\n", strings.ReplaceAll(r.Teaser.Prompt, "\n", "\n> "))
	fmt.Fprintf(&b, "## Final Answer\n\n```\n%s\n```\n\n", r.FinalAnswer)

	if len(r.Attempts) > 0 {
		b.WriteString("## Attempts\n\n")
		for i, a := range r.Attempts {
			st := "OK"
			if a.IsError {
				st = "ERROR"
			}
			fmt.Fprintf(&b, "### Attempt %d (%s)\n\n```javascript\n%s\n```\n\n", i+1, st, a.Code)
			res := a.Result
			if len(res) > 500 {
				res = res[:500] + "…"
			}
			fmt.Fprintf(&b, "**Result:**\n```\n%s\n```\n\n", res)
		}
	}
	return b.String()
}

func renderIndex(results []Result, model string) string {
	var b strings.Builder
	passed, totalCalls, totalErrors, firstTry, recovered := 0, 0, 0, 0, 0
	var totalDuration int64
	for _, r := range results {
		if r.Success {
			passed++
		}
		totalCalls += len(r.Attempts)
		totalDuration += r.DurationMs
		errs := 0
		for _, a := range r.Attempts {
			if a.IsError {
				errs++
			}
		}
		totalErrors += errs
		if r.Success && len(r.Attempts) == 1 {
			firstTry++
		}
		if r.Success && errs > 0 {
			recovered++
		}
	}

	fmt.Fprintf(&b, "# Benchmark Results\n\n")
	fmt.Fprintf(&b, "**Model:** %s\n", model)
	fmt.Fprintf(&b, "**Date:** %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "**Score:** %d/%d\n\n", passed, len(results))

	b.WriteString("| Teaser | Status | Tool Calls | Errors | Duration | Details |\n")
	b.WriteString("|--------|--------|-----------|--------|----------|---------|\n")
	for _, r := range results {
		status := "FAIL"
		if r.Success {
			status = "PASS"
		}
		errs := 0
		for _, a := range r.Attempts {
			if a.IsError {
				errs++
			}
		}
		note := ""
		if r.Error != "" {
			note = " (" + r.Error + ")"
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %dms | [detail](%s/%s.md)%s |\n",
			r.Teaser.Name, status, len(r.Attempts), errs, r.DurationMs,
			sanitizeModel(model), r.Teaser.Name, note)
	}

	b.WriteString("\n## Summary\n\n")
	var failed []string
	for _, r := range results {
		if !r.Success {
			failed = append(failed, r.Teaser.Name)
		}
	}
	if len(failed) == 0 {
		b.WriteString("All benchmarks passed.\n")
	} else {
		fmt.Fprintf(&b, "Failed: %s\n", strings.Join(failed, ", "))
	}

	b.WriteString("\n## Aggregate Stats\n\n")
	n := len(results)
	passRate, avgCalls, avgDuration := 0, 0.0, int64(0)
	if n > 0 {
		passRate = passed * 100 / n
		avgCalls = float64(totalCalls) / float64(n)
		avgDuration = totalDuration / int64(n)
	}
	b.WriteString("| Metric | Value |\n|--------|-------|\n")
	fmt.Fprintf(&b, "| Pass rate | %d%% (%d/%d) |\n", passRate, passed, n)
	fmt.Fprintf(&b, "| First-try success | %d/%d |\n", firstTry, n)
	fmt.Fprintf(&b, "| Total tool calls | %d |\n", totalCalls)
	fmt.Fprintf(&b, "| Tool errors | %d |\n", totalErrors)
	fmt.Fprintf(&b, "| Avg tool calls/teaser | %.1f |\n", avgCalls)
	fmt.Fprintf(&b, "| Total time | %ds |\n", totalDuration/1000)
	fmt.Fprintf(&b, "| Avg time/teaser | %ds |\n", avgDuration/1000)
	if totalErrors > 0 {
		fmt.Fprintf(&b, "| Error recovery | %d teaser(s) passed despite a tool error |\n", recovered)
	} else {
		b.WriteString("| Error recovery | N/A (no errors) |\n")
	}
	return b.String()
}
