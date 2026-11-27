//go:build unix

package mcp

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts the program in its own process group, and makes a
// cancel kill the whole group, so children it started die with it.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
