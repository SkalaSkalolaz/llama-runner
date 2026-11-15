package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// gitPassthroughRunner extends the script-runner harness with a real-git
// passthrough: the guard spawns the actual git binary, while claude and the
// verify shell keep behaving exactly as in scriptRunner.
type gitPassthroughRunner struct{ scriptRunner }

func (r gitPassthroughRunner) Command(name string, args ...string) *exec.Cmd {
	if name == "git" {
		return exec.Command("git", args...)
	}
	return r.scriptRunner.Command(name, args...)
}

// gitRepo creates a temporary git repository with one empty base commit,
// the same setup as the vcs package tests. Tests skip when git is absent.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	// A fresh HOME isolates git from the user's global hooks (core.hooksPath)
	// and ignores; the guard's commits run with --no-verify anyway.
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	cmd.Env = append(os.Environ(), "HOME="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir,
		"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "base")
	cmd.Env = append(os.Environ(), "HOME="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git base commit: %v\n%s", err, out)
	}
	return dir
}

// guardOpts builds run options whose step working directory is the given
// repository and whose runner passes git through to the real binary.
func guardOpts(t *testing.T, claudeScript, dir string) Options {
	t.Helper()
	opts := baseOpts(t, claudeScript)
	opts.Cfg.ClaudeDir = dir
	opts.Runner = gitPassthroughRunner{scriptRunner{claude: claudeScript}}
	return opts
}

// runGit runs a git command in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// commitCount counts the commits reachable from HEAD.
func commitCount(t *testing.T, dir string) int {
	t.Helper()
	out := strings.TrimSpace(runGit(t, dir, "rev-list", "--count", "HEAD"))
	n, err := strconv.Atoi(out)
	if err != nil {
		t.Fatalf("rev-list --count = %q: %v", out, err)
	}
	return n
}

func TestRun_GitGuardDisabledByDefault(t *testing.T) {
	// No -git-guard: the run must not create any branch, byte-for-byte
	// the pre-guard behaviour.
	dir := gitRepo(t)
	opts := guardOpts(t, "echo done", dir)
	plan := planner.Plan{Steps: []planner.Step{stepNoDeps("1", "auto")}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	if branches := strings.TrimSpace(runGit(t, dir, "branch", "--list", "llama-runner/*")); branches != "" {
		t.Errorf("no llama-runner branch must exist without the flag, got:\n%s", branches)
	}
}

func TestRun_GitGuardCheckpointPerWave(t *testing.T) {
	// Two independent steps on a parallel=1 model form two waves; each
	// successful wave leaves exactly one checkpoint commit behind.
	dir := gitRepo(t)
	script := `case "$*" in
*AAA*) echo a > a.txt ;;
*BBB*) echo b > b.txt ;;
esac
echo done`
	opts := guardOpts(t, script, dir)
	opts.GitGuard = true
	s1 := stepNoDeps("1", "auto")
	s1.Goal = "AAA write a.txt"
	s2 := stepNoDeps("2", "auto")
	s2.Goal = "BBB write b.txt"
	plan := planner.Plan{Steps: []planner.Step{s1, s2}}

	before := commitCount(t, dir)
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	if got := commitCount(t, dir) - before; got != 2 {
		t.Errorf("new commits = %d, want 2 (one checkpoint per wave)", got)
	}
	if branches := strings.TrimSpace(runGit(t, dir, "branch", "--list", "llama-runner/*")); branches == "" {
		t.Error("the guard must create a llama-runner/* branch")
	}
}

func TestRun_GitGuardRollsBackFailedWave(t *testing.T) {
	// Wave 1 succeeds and is checkpointed; wave 2 modifies the committed
	// file, drops an untracked file and fails. After the run the tracked
	// state is back at the checkpoint and nothing was deleted.
	dir := gitRepo(t)
	script := `case "$*" in
*S1*) echo v1 > file1.txt ;;
*S2*) echo v2 > file1.txt; echo x > file2.txt; exit 3 ;;
esac
echo done`
	opts := guardOpts(t, script, dir)
	opts.GitGuard = true
	s1 := stepNoDeps("1", "auto")
	s1.Goal = "S1 write file1"
	s2 := stepNoDeps("2", "auto")
	s2.Goal = "S2 break"
	plan := planner.Plan{Steps: []planner.Step{s1, s2}}

	before := commitCount(t, dir)
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("a failing step must fail the run")
	}
	// file1 exists with the checkpointed content: step 2's edit is undone.
	data, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatalf("file1.txt must exist after the rollback: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "v1" {
		t.Errorf("file1.txt = %q, want v1 (step 2's change must be rolled back)", got)
	}
	// Only the wave-1 checkpoint was added; the failed wave committed nothing.
	if got := commitCount(t, dir) - before; got != 1 {
		t.Errorf("new commits = %d, want 1 (checkpoint of wave 1 only)", got)
	}
	// The guard never deletes untracked files.
	if _, err := os.Stat(filepath.Join(dir, "file2.txt")); err != nil {
		t.Errorf("untracked file2.txt must be left in place: %v", err)
	}
}

func TestRun_GitGuardNotARepoFails(t *testing.T) {
	// With the guard on, a non-repository working directory stops the run
	// before any step executes.
	opts := baseOpts(t, "echo done")
	opts.GitGuard = true
	plan := planner.Plan{Steps: []planner.Step{stepNoDeps("1", "auto")}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("the guard must refuse a non-repository directory")
	}
	if len(res.Steps) != 0 {
		t.Errorf("steps = %d, want 0 (the run must not start)", len(res.Steps))
	}
}
