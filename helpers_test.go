package main

import "os/exec"

// fakeRunner подменяет exec.Command для тестов.
// Ключ для scripts:
//   - "git -C <path> <subcmd> ..." → "git <subcmd>"
//   - "cmake <first-arg> ..."      → "cmake <first-arg>"
//
// Если ключа нет в map — команда завершается с "exit 0".
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
