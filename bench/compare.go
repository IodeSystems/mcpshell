package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunRecord is the machine-readable per-teaser result written alongside the
// markdown, so a run can be compared against another without scraping tables.
type RunRecord struct {
	Name      string `json:"name"`
	Success   bool   `json:"success"`
	Turns     int    `json:"turns"`
	ToolCalls int    `json:"tool_calls"`
	PromptTok int    `json:"prompt_tokens"`
	CachedTok int    `json:"cached_tokens"`
	CompTok   int    `json:"completion_tokens"`
	ToolMs    int64  `json:"tool_ms"`
	ModelMs   int64  `json:"model_ms"`
	TotalMs   int64  `json:"total_ms"`
	Error     string `json:"error,omitempty"`
	ToolOnly  bool   `json:"tool_only,omitempty"`
}

// Processed is the compute-cost tokens: non-cached prompt + completion. A large
// system prompt re-sent each turn is cached, so it barely counts here.
func (r RunRecord) Processed() int { return r.PromptTok - r.CachedTok + r.CompTok }

// recordsFileName is the machine-readable results file written per run.
const recordsFileName = "results.json"

func writeRecords(results []Result, outputDir string) error {
	recs := make([]RunRecord, len(results))
	for i, r := range results {
		recs[i] = RunRecord{
			Name:      r.Teaser.Name,
			Success:   r.Success,
			Turns:     r.Stats.Turns,
			ToolCalls: len(r.Attempts),
			PromptTok: r.Stats.Tokens.Prompt,
			CachedTok: r.Stats.Tokens.Cached,
			CompTok:   r.Stats.Tokens.Completion,
			ToolMs:    r.ToolMs,
			ModelMs:   r.ModelMs(),
			TotalMs:   r.DurationMs,
			Error:     r.Error,
			ToolOnly:  r.Teaser.ToolOnly,
		}
	}
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, recordsFileName), append(data, '\n'), 0o644)
}

// LoadRecords reads the results.json written by a run in dir.
func LoadRecords(dir string) ([]RunRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, recordsFileName))
	if err != nil {
		return nil, err
	}
	var recs []RunRecord
	if err := json.Unmarshal(data, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// Compare writes a with/without comparison markdown from two runs' results.json.
// Teasers present in both runs form the head-to-head table; teasers only in the
// "with" run (e.g. tool-only ones skipped by the no-tool baseline) are listed
// separately since they have no fair baseline.
func Compare(withDir, withoutDir, outPath, title, label string) error {
	with, err := LoadRecords(withDir)
	if err != nil {
		return fmt.Errorf("with run: %w", err)
	}
	without, err := LoadRecords(withoutDir)
	if err != nil {
		return fmt.Errorf("without run: %w", err)
	}
	withByName := map[string]RunRecord{}
	for _, r := range with {
		withByName[r.Name] = r
	}
	withoutByName := map[string]RunRecord{}
	for _, r := range without {
		withoutByName[r.Name] = r
	}

	names := make([]string, 0, len(withByName))
	for n := range withByName {
		names = append(names, n)
	}
	sort.Strings(names)

	mark := func(ok bool) string {
		if ok {
			return "✅"
		}
		return "❌"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "**Model:** %s &nbsp;·&nbsp; **With** = mcpshell eval tool available &nbsp;·&nbsp; **Without** = reasoning only, no tool.\n\n", label)

	b.WriteString("## Head-to-head (self-contained problems)\n\n")
	b.WriteString("Proc(cached) = processed (non-cached prompt + generated) with cached prompt tokens in parens; the re-sent system prompt is cached, so it barely counts as cost.\n\n")
	b.WriteString("| Problem | With | Without | Turns (w/wo) | Proc(cached) w/wo | Time w/wo |\n")
	b.WriteString("|---------|:----:|:-------:|:------------:|:-----------------:|:---------:|\n")
	wp, np, tot := 0, 0, 0
	var wTurns, nTurns, wTok, nTok, wCached, nCached int
	var wMs, nMs int64
	for _, n := range names {
		nr, ok := withoutByName[n]
		if !ok {
			continue // tool-only; handled below
		}
		wr := withByName[n]
		tot++
		if wr.Success {
			wp++
		}
		if nr.Success {
			np++
		}
		wTurns += wr.Turns
		nTurns += nr.Turns
		wTok += wr.Processed()
		nTok += nr.Processed()
		wCached += wr.CachedTok
		nCached += nr.CachedTok
		wMs += wr.TotalMs
		nMs += nr.TotalMs
		fmt.Fprintf(&b, "| %s | %s | %s | %d/%d | %d(%d)/%d(%d) | %.1f/%.1fs |\n",
			n, mark(wr.Success), mark(nr.Success),
			wr.Turns, nr.Turns, wr.Processed(), wr.CachedTok, nr.Processed(), nr.CachedTok,
			float64(wr.TotalMs)/1000, float64(nr.TotalMs)/1000)
	}

	var toolOnly []string
	for _, n := range names {
		if _, ok := withoutByName[n]; !ok {
			toolOnly = append(toolOnly, n)
		}
	}

	b.WriteString("\n## Headline\n\n")
	b.WriteString("| Metric (self-contained) | With mcpshell | Without |\n")
	b.WriteString("|-------------------------|:-------------:|:-------:|\n")
	fmt.Fprintf(&b, "| **Solved** | **%d/%d** | %d/%d |\n", wp, tot, np, tot)
	fmt.Fprintf(&b, "| Total turns | %d | %d |\n", wTurns, nTurns)
	fmt.Fprintf(&b, "| Processed tokens | %d | %d |\n", wTok, nTok)
	fmt.Fprintf(&b, "| Cached tokens (~free) | %d | %d |\n", wCached, nCached)
	fmt.Fprintf(&b, "| Total time | %.0fs | %.0fs |\n", float64(wMs)/1000, float64(nMs)/1000)
	if len(toolOnly) > 0 {
		fmt.Fprintf(&b, "\nPlus %d tool-only problem(s) the tool-equipped agent solves that are impossible without it (below).\n",
			len(toolOnly))
	}

	if len(toolOnly) > 0 {
		b.WriteString("\n## Tool-only (no baseline — needs data/state the model can't have)\n\n")
		b.WriteString("| Problem | With | Tool calls | Tool ms | Model ms |\n")
		b.WriteString("|---------|:----:|:----------:|--------:|---------:|\n")
		for _, n := range toolOnly {
			wr := withByName[n]
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d |\n", n, mark(wr.Success), wr.ToolCalls, wr.ToolMs, wr.ModelMs)
		}
	}

	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}
