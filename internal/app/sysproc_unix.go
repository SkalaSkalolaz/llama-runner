//go:build !windows

package app

import (
	"os/exec"
	"syscall"
)

// setProcAttr puts the child in its own process group, so any helper
// processes it spawns die with it. syscall.SysProcAttr.Setpgid exists only
// on Unix; the Windows counterpart is a no-op (see sysproc_windows.go).
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalChild delivers sig to cmd: to its process group when the child was
// started with setProcAttr, to the process itself otherwise. Nil commands and
// not-yet-started children are silently ignored — a signal may race with a
// failing Start.
func signalChild(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
		return
	}
	_ = cmd.Process.Signal(sig)
}
