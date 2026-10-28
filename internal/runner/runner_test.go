package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RealRunner must stay assignable to the interface the launcher programs against.
var _ CommandRunner = RealRunner{}

func TestCommandCarriesFileAndArguments(t *testing.T) {
	cmd := RealRunner{}.Command("/opt/llama/bin/llama-server", "-m", "model.gguf", "--port", "8080")
	if cmd == nil {
		t.Fatal("Command returned nil")
	}
	if len(cmd.Args) != 5 {
		t.Errorf("Args = %v, want program plus 4 arguments", cmd.Args)
	}
	if cmd.Args[0] != "/opt/llama/bin/llama-server" {
		t.Errorf("Args[0] = %q, want the program path", cmd.Args[0])
	}
	if cmd.Args[1] != "-m" || cmd.Args[4] != "8080" {
		t.Errorf("Args = %v, want arguments preserved in order", cmd.Args)
	}
}

func TestLookPathFindsExecutableByAbsolutePath(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mcp-search-server")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := RealRunner{}.LookPath(bin)
	if err != nil {
		t.Fatalf("LookPath(%s): %v", bin, err)
	}
	if resolved != bin {
		t.Errorf("resolved = %q, want %q", resolved, bin)
	}
}

func TestLookPathFindsBareNameOnPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "llama-fake-on-path")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	resolved, err := RealRunner{}.LookPath("llama-fake-on-path")
	if err != nil {
		t.Fatalf("LookPath on PATH entry: %v", err)
	}
	if !strings.HasSuffix(resolved, "llama-fake-on-path") {
		t.Errorf("resolved = %q", resolved)
	}
}

func TestLookPathMissingNameErrors(t *testing.T) {
	r := RealRunner{}
	if _, err := r.LookPath("llama-runner-definitely-not-installed"); err == nil {
		t.Error("LookPath of a missing name should error")
	}
}
