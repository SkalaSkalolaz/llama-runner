package doctor

import "os/exec"

// fakeRunner satisfies runner.CommandRunner for doctor tests: it never runs a
// real tool (every command is "sh -c exit 0") and reports no PATH hit, so
// doctor can be exercised without touching git, cmake or claude.
type fakeRunner struct {
	scripts        map[string]string
	lookPathResult string
	lookPathErr    error
}

func (f fakeRunner) Command(name string, args ...string) *exec.Cmd {
	key := name
	switch {
	case len(args) >= 3 && args[0] == "-C":
		// git -C <path> <subcmd> ...
		key += " " + args[2]
	case len(args) > 0:
		key += " " + args[0]
	}
	script, ok := f.scripts[key]
	if !ok {
		script = "exit 0"
	}
	return exec.Command("sh", "-c", script)
}

func (f fakeRunner) LookPath(file string) (string, error) {
	return f.lookPathResult, f.lookPathErr
}
