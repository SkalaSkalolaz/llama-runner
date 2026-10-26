package main

import "os/exec"

// CommandRunner abstracts the two process-spawning primitives the runner
// depends on — exec.Command and exec.LookPath — so that tests can inject a
// fake implementation and observe or replace what would be launched without
// starting real processes.
type CommandRunner interface {
	// Command builds (but does not start) the command named name with the
	// given arguments, mirroring exec.Command semantics.
	Command(name string, args ...string) *exec.Cmd
	// LookPath resolves file to an absolute executable path, searching PATH
	// for bare names, mirroring exec.LookPath semantics.
	LookPath(file string) (string, error)
}

// realRunner is the production CommandRunner: it delegates directly to the
// os/exec package and carries no state.
type realRunner struct{}

// Command returns the named command via exec.Command.
func (realRunner) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// LookPath resolves an executable via exec.LookPath.
func (realRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
