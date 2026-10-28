package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestSetWorkDir_ExplicitDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{WorkDir: dir}
	cmd := exec.Command("true")
	if code := SetWorkDir(cmd, cfg, dir, newStderr()); code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if cmd.Dir != dir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, dir)
	}
}

func TestSetWorkDir_MissingDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{WorkDir: filepath.Join(dir, "nope")}
	cmd := exec.Command("true")
	if code := SetWorkDir(cmd, cfg, dir, newStderr()); code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if cmd.Dir != "" {
		t.Errorf("cmd.Dir = %q, want untouched", cmd.Dir)
	}
}

func TestSetWorkDir_DefaultsToLauncherCwd(t *testing.T) {
	cfg := &config.Config{}
	cmd := exec.Command("true")
	if code := SetWorkDir(cmd, cfg, "/unused", newStderr()); code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	wanted, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatal(wdErr)
	}
	if cmd.Dir != wanted {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, wanted)
	}
}

func TestApplyLdLibraryPath_PrependsSiblingLib(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	libDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := filepath.Join(binDir, "llama-server")
	if err := os.WriteFile(serverBin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LD_LIBRARY_PATH", "/usr/local/cuda/lib64")

	cmd := exec.Command("true")
	ApplyLdLibraryPath(cmd, serverBin, newStderr(), false)

	if cmd.Env == nil {
		t.Fatal("cmd.Env = nil, want materialised environment")
	}
	var found []string
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "LD_LIBRARY_PATH=") {
			found = append(found, kv)
		}
	}
	if len(found) == 0 {
		t.Fatalf("LD_LIBRARY_PATH not set on cmd.Env: %v", found)
	}
	// The launcher appends its entry; execve takes the last occurrence, so
	// the effective value is the last one.
	value := found[len(found)-1][len("LD_LIBRARY_PATH="):]
	if value != libDir+":/usr/local/cuda/lib64" {
		t.Errorf("effective LD_LIBRARY_PATH = %q, want %q first", value, libDir)
	}
}

func TestApplyLdLibraryPath_DoesNotDuplicateExistingSegment(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	libDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LD_LIBRARY_PATH", libDir+":/else")

	cmd := exec.Command("true")
	cmd.Env = []string{"PATH=/usr/bin", "LD_LIBRARY_PATH=" + libDir + ":/else"}
	ApplyLdLibraryPath(cmd, filepath.Join(binDir, "llama-server"), newStderr(), false)

	entries := 0
	for _, kv := range cmd.Env {
		if !strings.HasPrefix(kv, "LD_LIBRARY_PATH=") {
			continue
		}
		entries++
		if strings.Count(kv, libDir) != 1 {
			t.Errorf("segment duplicated: %q", kv)
		}
	}
	if entries != 1 {
		t.Errorf("LD_LIBRARY_PATH entries = %d, want the pre-existing one untouched", entries)
	}
	if !strings.Contains(strings.Join(cmd.Env, "\n"), "PATH=/usr/bin") {
		t.Errorf("cmd.Env replaced instead of left alone: %v", cmd.Env)
	}
}

func newStderr() *bytes.Buffer { return new(bytes.Buffer) }
