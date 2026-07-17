package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// TestReferenceSolutions runs each canonical mcpshell reference solution and
// checks it against its teaser's validator — proving the deterministic teasers
// are solvable in mcpshell and that the baked-in expected answers are correct,
// no LLM required. Heavy solutions run only when MCPSHELL_BENCH_HEAVY is set;
// Ceiling solutions are documented but not executed.
func TestReferenceSolutions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fixture.sqlite")
	if err := SeedSQLite(dbPath); err != nil {
		t.Fatal(err)
	}
	newShell := func() *runtime.Shell {
		sh := runtime.NewShell()
		toolkit.InstallCore(sh)
		toolkit.InstallMath(sh)
		if _, err := toolkit.InstallSQL(sh, "shop", dbPath, true); err != nil {
			t.Fatal(err)
		}
		return sh
	}

	validators := map[string]func(string) bool{}
	for _, ts := range Suite {
		validators[ts.Name] = ts.Validate
	}
	runHeavy := os.Getenv("MCPSHELL_BENCH_HEAVY") != ""

	for _, r := range References {
		t.Run(r.Name, func(t *testing.T) {
			validate := validators[r.Name]
			if validate == nil {
				t.Fatalf("reference %q has no matching teaser in Suite", r.Name)
			}
			if r.Ceiling {
				t.Skipf("ceiling reference — documented, exceeds practical interpreter runtime")
			}
			if r.Heavy && !runHeavy {
				t.Skipf("heavy reference — set MCPSHELL_BENCH_HEAVY=1 to run")
			}
			v, err := newShell().Eval(r.Code)
			if err != nil {
				t.Fatalf("eval error: %v", err)
			}
			if got := v.Display(); !validate(got) {
				t.Fatalf("answer %q fails the teaser validator", got)
			}
		})
	}
}

// TestReferenceCoverage keeps References in lockstep with the deterministic
// teasers: every euler_*/compose_* teaser must have a reference solution.
func TestReferenceCoverage(t *testing.T) {
	haveRef := map[string]bool{}
	for _, r := range References {
		haveRef[r.Name] = true
	}
	for _, ts := range Suite {
		if strings.HasPrefix(ts.Name, "euler_") || strings.HasPrefix(ts.Name, "compose_") {
			if !haveRef[ts.Name] {
				t.Errorf("teaser %q has no reference solution in References", ts.Name)
			}
		}
	}
}
