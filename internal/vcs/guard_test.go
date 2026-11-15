package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a temporary git repository and returns its path.
// If git is not available, the test is skipped.
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "base")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	return dir
}

func TestNew_NotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := New(nil, dir, fakeRunner{}, false, nil)
	if err == nil {
		t.Fatal("expected error for non-repo")
	}
	if !strings.Contains(err.Error(), "not a git repository") && !strings.Contains(err.Error(), "не является git-репозиторием") {
		t.Fatalf("wrong error: %v", err)
	}
}

// fakeRunner is a minimal CommandRunner for tests.
type fakeRunner struct{}

func (fakeRunner) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (fakeRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func TestNew_DirtyRefused(t *testing.T) {
	dir := initRepo(t)
	// Create an untracked file to make the tree dirty
	path := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := New(nil, dir, fakeRunner{}, false, nil)
	if err == nil {
		t.Fatal("expected error for dirty tree")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") && !strings.Contains(err.Error(), "незакоммиченные изменения") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestNew_DirtyAllowed(t *testing.T) {
	dir := initRepo(t)
	path := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	g, err := New(nil, dir, fakeRunner{}, true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(g.Branch(), "llama-runner/run-") {
		t.Fatalf("wrong branch name: %s", g.Branch())
	}
}

func TestNew_CreatesBranch(t *testing.T) {
	dir := initRepo(t)
	g, err := New(nil, dir, fakeRunner{}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(g.Branch(), "llama-runner/run-") {
		t.Fatalf("wrong branch name: %s", g.Branch())
	}
	if g.Base() == "" {
		t.Fatal("base should not be empty")
	}
}

func TestCheckpoint_Commits(t *testing.T) {
	dir := initRepo(t)
	g, err := New(nil, dir, fakeRunner{}, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create a file and checkpoint
	path := filepath.Join(dir, "step1.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := g.Checkpoint(nil, "step 1")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if hash == g.Base() {
		t.Fatal("checkpoint hash should differ from base")
	}

	// Verify the file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatal("file should exist after checkpoint")
	}
}

func TestCheckpoint_EmptyIsNoop(t *testing.T) {
	dir := initRepo(t)
	g, err := New(nil, dir, fakeRunner{}, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// First checkpoint
	hash1, err := g.Checkpoint(nil, "first")
	if err != nil {
		t.Fatalf("Checkpoint 1: %v", err)
	}

	// Second checkpoint with no changes — should be a no-op
	hash2, err := g.Checkpoint(nil, "second")
	if err != nil {
		t.Fatalf("Checkpoint 2: %v", err)
	}

	if hash2 != hash1 {
		t.Fatalf("no-op checkpoint should return same hash: %s vs %s", hash2, hash1)
	}
}

func TestRollback_RestoresCheckpoint(t *testing.T) {
	dir := initRepo(t)
	g, err := New(nil, dir, fakeRunner{}, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Step 1: create file1
	path1 := filepath.Join(dir, "file1.txt")
	if err := os.WriteFile(path1, []byte("step1"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = g.Checkpoint(nil, "step 1")
	if err != nil {
		t.Fatalf("Checkpoint 1: %v", err)
	}

	// Step 2: modify file1 and create file2
	if err := os.WriteFile(path1, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	path2 := filepath.Join(dir, "file2.txt")
	if err := os.WriteFile(path2, []byte("step2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rollback
	var buf strings.Builder
	g.out = &buf
	if err := g.Rollback(nil); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// file1 should exist with original content
	content, err := os.ReadFile(path1)
	if err != nil {
		t.Fatal("file1 should exist after rollback")
	}
	if string(content) != "step1" {
		t.Fatalf("file1 content wrong: %q", string(content))
	}

	// file2 is untracked (never committed), so it survives reset --hard
	// This is correct behavior: rollback only resets tracked state
	if _, err := os.Stat(path2); os.IsNotExist(err) {
		t.Fatal("file2 (untracked) should survive rollback")
	}
}

func TestRollback_KeepsUntracked(t *testing.T) {
	dir := initRepo(t)
	g, err := New(nil, dir, fakeRunner{}, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create an untracked file
	untracked := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(untracked, []byte("untracked"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rollback
	var buf strings.Builder
	g.out = &buf
	if err := g.Rollback(nil); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Untracked file should still exist
	if _, err := os.Stat(untracked); os.IsNotExist(err) {
		t.Fatal("untracked file should remain after rollback")
	}

	// Output should mention untracked files
	if !strings.Contains(buf.String(), "untracked") {
		t.Fatalf("output should mention untracked files: %q", buf.String())
	}
}

func TestSummary_MentionsChangedFile(t *testing.T) {
	dir := initRepo(t)
	g, err := New(nil, dir, fakeRunner{}, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create and checkpoint a file
	path := filepath.Join(dir, "changed.txt")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = g.Checkpoint(nil, "changed")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	s, err := g.Summary(nil)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !strings.Contains(s, "changed.txt") {
		t.Fatalf("summary should mention changed.txt: %q", s)
	}
}
