//go:build !unix

package mcp

import "os/exec"

// setProcessGroup: without process groups a cancel kills the program only
// (exec.CommandContext's default).
func setProcessGroup(cmd *exec.Cmd) {}
