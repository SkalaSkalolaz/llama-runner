package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// argLog is the shared sink recordingRunner writes through. It exists because
// CommandRunner is satisfied by value: appending to a field of a value copy
// would be invisible to the caller, so the list lives behind a pointer.
type argLog struct {
	commands [][]string
}

// recordingRunner captures the argument vector of every command Run builds and
// substitutes a command that exits immediately, so the -server path returns at
// once while the exact llama-server command line stays observable. Unlike
// fakeRunner it records rather than scripts, which is what the alias assertions
// need.
type recordingRunner struct {
	log *argLog
}

func (r recordingRunner) Command(name string, args ...string) *exec.Cmd {
	line := append([]string{name}, args...)
	r.log.commands = append(r.log.commands, line)
	return exec.Command("sh", "-c", "exit 0")
}

func (r recordingRunner) LookPath(file string) (string, error) {
	return file, nil
}

// serverRun runs Run in -server mode inside a throwaway home that contains a
// usable llama-server binary plus both the built-in-default and the gpt-oss
// model files, so the caller may choose either model on the command line. It
// returns the recorded llama-server argument vector and stderr for diagnostics.
func serverRun(t *testing.T, args []string) (server []string, errText string) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	bin := filepath.Join(dir, ".local", "bin", "llama-server")
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(dir, "GGUF")
	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Qwen3.8-35B-A3B-Q4_K_M.gguf", "gpt-oss-20b-Q5_K_M.gguf"} {
		if err := os.WriteFile(filepath.Join(dir, "GGUF", name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var out, errb bytes.Buffer
	log := &argLog{}
	code := Run(args, &out, &errb, recordingRunner{log: log}, "1.5.3-test")
	if code != 0 {
		t.Fatalf("Run = %d, want 0 (stderr=%s)", code, errb.String())
	}
	if len(log.commands) == 0 {
		t.Fatalf("no llama-server command recorded (stderr=%s)", errb.String())
	}
	return log.commands[0], errb.String()
}

func TestRun_ServerMode_UsesAliasDerivedFromCLIModel(t *testing.T) {
	// Server mode is no longer exempt from naming: the WebUI card must carry
	// the same name the Claude Code mode would have advertised, and a CLI
	// -model must not be shadowed by a stale alias from the config file.
	args, errText := serverRun(t, []string{"-config", "", "-server",
		"-model", "~/GGUF/gpt-oss-20b-Q5_K_M.gguf"})
	if !strings.Contains(strings.Join(args, " "), "--alias gpt-oss-20b-Q5_K_M") {
		t.Errorf("--alias gpt-oss-20b-Q5_K_M missing from: %s (stderr=%s)",
			strings.Join(args, " "), errText)
	}
}

func TestRun_ServerMode_ProfileAliasPreserved(t *testing.T) {
	// -p coder without a CLI -model: the profile is self-contained, so its
	// own alias survives into --alias.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")
	content := `{"profiles": {"coder": {"alias": "qwen3.8-coder"}}}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	args, errText := serverRun(t, []string{"-config", cfgPath, "-server", "-p", "coder"})
	if !strings.Contains(strings.Join(args, " "), "--alias qwen3.8-coder") {
		t.Errorf("--alias qwen3.8-coder missing from: %s (stderr=%s)",
			strings.Join(args, " "), errText)
	}
}
