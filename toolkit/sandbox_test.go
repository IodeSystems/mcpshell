package toolkit_test

import (
	"strings"
	"testing"

	"github.com/iodesystems/mcpshell/runtime"
	"github.com/iodesystems/mcpshell/toolkit"
)

// fullShell installs every non-privileged toolkit, so the sandbox assertions
// below prove the escapes are blocked by the *language*, not merely by a missing
// toolkit. (SQL/file/browser are opt-in and scoped; not needed to make the point.)
func fullShell() *runtime.Shell {
	sh := runtime.NewShell()
	toolkit.InstallCore(sh)
	toolkit.InstallMath(sh)
	toolkit.InstallWeb(sh)
	toolkit.InstallGraph(sh)
	return sh
}

// TestSandboxNoHostEscape documents mcpshell's safety guarantee for untrusted
// execution: the language has no path to the host — no imports, no eval, no
// process, no ambient filesystem/network/exec — so none of these compile away
// into capability. Each must error rather than reach outside the sandbox.
func TestSandboxNoHostEscape(t *testing.T) {
	escapes := []string{
		`require("fs")`,           // no module system
		`import("fs")`,            // no dynamic import
		`eval("1+1")`,             // no nested/host eval
		`process`,                 // no process object
		`process.exit(0)`,         // no process control
		`globalThis`,              // no global object to walk
		`this`,                    // no receiver / prototype chain
		`this.constructor`,        // no constructor escape
		`readFile("/etc/passwd")`, // no ambient filesystem
		`open("/etc/passwd")`,
		`fetch("http://169.254.169.254/")`, // no ambient network (SSRF)
		`exec("id")`,                       // no process spawn
		`system("id")`,
		`Function("return 1")`, // no function constructor
	}
	sh := fullShell()
	for _, src := range escapes {
		if _, err := sh.Eval(src); err == nil {
			t.Errorf("sandbox escape not blocked: %q evaluated without error", src)
		}
	}
}

// TestSandboxResourceLimits proves untrusted code can't hang or exhaust the
// host: runaway loops and unbounded recursion terminate with a limit error
// instead of running forever.
func TestSandboxResourceLimits(t *testing.T) {
	sh := fullShell()

	if _, err := sh.Eval(`while (true) {}`); err == nil {
		t.Error("infinite loop was not stopped by the step limit")
	} else if !strings.Contains(strings.ToLower(err.Error()), "step") {
		t.Errorf("infinite loop stopped, but not by the step limit: %v", err)
	}

	if _, err := sh.Eval(`function f() { return f() } f()`); err == nil {
		t.Error("unbounded recursion was not stopped by the call-depth limit")
	}
}

// TestCapabilitiesAreOptIn shows the base language grants no I/O at all — the
// web namespace only exists once InstallWeb is called. A core-only shell cannot
// reach the network even by name.
func TestCapabilitiesAreOptIn(t *testing.T) {
	core := toolkit.InstallCore(runtime.NewShell())
	if _, err := core.Eval(`Web.fetch("http://example.com")`); err == nil {
		t.Error("Web.fetch worked on a core-only shell; capabilities must be opt-in")
	}
	// Once granted, it resolves as a command (may still fail on the network,
	// but the point is the capability is now present by explicit choice).
	web := fullShell()
	if _, err := web.Eval(`typeof Web`); err != nil {
		t.Errorf("Web namespace missing after InstallWeb: %v", err)
	}
}
