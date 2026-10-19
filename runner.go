package main

import "os/exec"

// CommandRunner позволяет подменить exec.Command/exec.LookPath в тестах.
type CommandRunner interface {
    Command(name string, args ...string) *exec.Cmd
    LookPath(file string) (string, error)
}

type realRunner struct{}

func (realRunner) Command(name string, args ...string) *exec.Cmd {
    return exec.Command(name, args...)
}

func (realRunner) LookPath(file string) (string, error) {
    return exec.LookPath(file)
}