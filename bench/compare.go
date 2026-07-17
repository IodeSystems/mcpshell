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
	ToolCalls int    `json:"tool_calls"`
	ToolMs    int64  `json:"tool_ms"`
	ModelMs   int64  `json:"model_ms"`
	TotalMs   int64  `json:"total_ms"`
	Error     string `json:"error,omitempty"`
	ToolOnly  bool   `json:"tool_only,omitempty"`
}

// recordsFileName is the machine-readable results file written per run.
const recordsFileName = "results.json"

func writeRecords(results []Result, outputDir string) error {
	recs := make([]RunRecord, len(results))
	for i, r := range results {
		recs[i] = RunRecord{
			Name:      r.Teaser.Name,
			Success:   r.Success,
			ToolCalls: len(r.Attempts),
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
	b.WriteString("| Problem | With | Without | Tool calls | Tool ms | Model ms |\n")
	b.WriteString("|---------|:----:|:-------:|:----------:|--------:|---------:|\n")
	wp, np, tot := 0, 0, 0
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
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d |\n",
			n, mark(wr.Success), mark(nr.Success), wr.ToolCalls, wr.ToolMs, wr.ModelMs)
	}
	fmt.Fprintf(&b, "\n**Self-contained totals:** with **%d/%d**, without **%d/%d**.\n", wp, tot, np, tot)

	var toolOnly []string
	for _, n := range names {
		if _, ok := withoutByName[n]; !ok {
			toolOnly = append(toolOnly, n)
		}
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
