//go:build unix

package mcp_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iodesystems/mcpshell/mcp"
)

// waitGone waits until pid no longer runs (a zombie waiting for its reaper
// counts as gone once it is reaped).
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("process %d still runs: the process group was not killed", pid)
}

// pidFrom reads the child's pid from the helper's pid file.
func pidFrom(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no pid file: %v", err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// The command's timeoutMs kills the program and the child it started (same
// process group), and the call is an error.
func TestCommandTimeoutKillsGroup(t *testing.T) {
	pf := filepath.Join(t.TempDir(), "pid")
	decl := strings.Replace(helperDecl(`{"sleep": {"args": ["sleep"], "timeoutMs": 300}}`, `, "PIDFILE=`+pf+`"`), `"env"`, `"timeoutMs": 60000, "env"`, 1)
	srv := commandServer(t, decl, mcp.CommandOptions{})
	start := time.Now()
	got, isErr := srv.EvalText(`t.sleep()`)
	if !isErr || !strings.Contains(got, "t.sleep: still running at its timeout (300ms, set by the operator); the process was killed") {
		t.Errorf("eval = %q", got)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s: the child kept the output pipe open", d)
	}
	waitGone(t, pidFrom(t, pf))
}

// A cancelled eval kills the running program.
func TestCommandCancelKills(t *testing.T) {
	pf := filepath.Join(t.TempDir(), "pid")
	srv, sh := commandServerShell(t, helperDecl(`{"sleep": {"args": ["sleep"]}}`, `, "PIDFILE=`+pf+`"`))
	go func() {
		for i := 0; i < 250; i++ {
			if _, err := os.Stat(pf); err == nil {
				sh.Limits().Cancel()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	got, isErr := srv.EvalText(`t.sleep()`)
	if !isErr || !strings.Contains(got, "Execution cancelled") {
		t.Errorf("eval = %q", got)
	}
	waitGone(t, pidFrom(t, pf))
}
