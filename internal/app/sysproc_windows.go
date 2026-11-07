//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// setProcAttr is a no-op on Windows: process groups (Setpgid) are a Unix
// concept and do not exist here. Helper processes are not cascaded on
// shutdown; the launcher terminates each child individually.
func setProcAttr(cmd *exec.Cmd) {}

// signalChild delivers sig to cmd directly. Windows has no process groups,
// and os.Process.Signal on Windows implements only os.Kill (== syscall.SIGKILL)
// as a hard TerminateProcess; SIGINT and SIGTERM are ignored by the runtime.
// The launcher's escalation ladder still ends in SIGKILL, so shutdown
// completes — just after the configured timeout rather than immediately.
func signalChild(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(sig)
}
