package orchestrator

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// fakeSwapper records swaps and answers with canned URLs.
type fakeSwapper struct {
	current string
	urls    map[string]string
	calls   []string
	err     error
}

func (f *fakeSwapper) SwapTo(_ context.Context, alias string) (string, error) {
	f.calls = append(f.calls, alias)
	if f.err != nil {
		return "", f.err
	}
	f.current = alias
	return f.urls[alias], nil
}

func (f *fakeSwapper) CurrentAlias() string { return f.current }

// scriptRunner stands in for runner.CommandRunner: claude becomes the
// "claude" script, verify shells ("sh") become the "shell" script. Claude's
// real arguments are forwarded as positional parameters ($1 onward), so a
// script can branch on the prompt content — e.g. fail steps whose goal
// carries a marker.
type scriptRunner struct {
	claude string
	shell  string
}

func (r scriptRunner) Command(name string, args ...string) *exec.Cmd {
	if name == "sh" {
		return exec.Command("sh", "-c", r.shell)
	}
	// sh -c 'script' sh arg1 arg2...
	// $0="sh", $1=arg1, $2=arg2. The script can use "$@" or "$*" if needed.
	// We do NOT append "$@" to the script text, otherwise in multi-command
	// scripts (separated by ";") the arguments would be passed to the last
	// command (e.g. `touch` would receive `-p` and fail).
	cmdArgs := append([]string{"-c", r.claude, "sh"}, args...)
	return exec.Command("sh", cmdArgs...)
}

func (r scriptRunner) LookPath(file string) (string, error) { return "/fake/claude", nil }

func baseOpts(t *testing.T, claudeScript string) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Cfg: &config.Config{
			Alias: "auto", ClaudeBin: "claude", ClaudeDir: dir,
			Host: "127.0.0.1", Port: "8080", CtxSize: "32768", Parallel: "1",
		},
		Profiles: map[string]*config.Config{
			"auto":   {Alias: "auto", CtxSize: "32768", Parallel: "1"},
			"worker": {Alias: "worker", CtxSize: "131072", Parallel: "1"},
		},
		Home:        dir,
		Runner:      scriptRunner{claude: claudeScript},
		Task:        "do it",
		BaseURL:     "http://127.0.0.1:8080",
		StepTimeout: 30 * time.Second,
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	}
}

func TestRun_ExecutesInOrderSwapsAndVerifies(t *testing.T) {
	opts := baseOpts(t, "echo step-done; touch file.txt")
	sw := &fakeSwapper{current: "auto", urls: map[string]string{
		"auto": "http://u:1", "worker": "http://u:2",
	}}
	opts.Swapper = sw
	plan := planner.Plan{Steps: []planner.Step{
		// Deliberately NOT in execution order: 2 depends on 1.
		{ID: "2", Role: "tester", Goal: "check", Alias: "auto",
			DependsOn: []string{"1"}, Verify: "test -f file.txt"},
		{ID: "1", Role: "coder", Goal: "write", Alias: "worker", Artifact: "file.txt"},
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	if len(res.Steps) != 2 || res.Steps[0].Step.ID != "1" || res.Steps[1].Step.ID != "2" {
		t.Errorf("steps = %+v, want topological order [1 2]", res.Steps)
	}
	// Start on auto: swap to worker for step 1, back to auto for step 2.
	if len(sw.calls) != 2 || sw.calls[0] != "worker" || sw.calls[1] != "auto" {
		t.Errorf("swaps = %v, want [worker auto]", sw.calls)
	}
	if res.Steps[0].Output != "step-done" {
		t.Errorf("summary = %q, want the last output line", res.Steps[0].Output)
	}
}

func TestRun_FailedStepStopsThePlan(t *testing.T) {
	opts := baseOpts(t, "exit 3")
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
		{ID: "2", Role: "tester", Goal: "g", Alias: "auto", DependsOn: []string{"1"}},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("a failing step must fail the run")
	}
	if len(res.Steps) != 1 {
		t.Errorf("steps = %d, want 1 (the plan stops at the first failure)", len(res.Steps))
	}
}

func TestRun_MissingArtifactFails(t *testing.T) {
	opts := baseOpts(t, "true") // produces nothing
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto", Artifact: "absent.txt"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed || res.Steps[0].Err == nil {
		t.Fatal("a missing artifact must fail the step")
	}
	if !strings.Contains(res.Steps[0].Err.Error(), "not found") {
		t.Errorf("err = %v, want an artifact-not-found message", res.Steps[0].Err)
	}
}

func TestRun_FailedVerifyFails(t *testing.T) {
	opts := baseOpts(t, "true")
	opts.Runner = scriptRunner{claude: "true", shell: "exit 7"}
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "tester", Goal: "g", Alias: "auto", Verify: "some check"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed || res.Steps[0].Err == nil {
		t.Fatal("a failing verify command must fail the step")
	}
	if !strings.Contains(res.Steps[0].Err.Error(), "verification") {
		t.Errorf("err = %v, want a verification message", res.Steps[0].Err)
	}
}

func TestRun_SwapFailureStopsThePlan(t *testing.T) {
	opts := baseOpts(t, "true")
	opts.Swapper = &fakeSwapper{current: "auto", err: errors.New("boom")}
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "worker"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed || len(res.Steps) != 1 {
		t.Fatalf("res = %+v, want a single failed step", res)
	}
	if !strings.Contains(res.Steps[0].Err.Error(), "boom") {
		t.Errorf("err = %v", res.Steps[0].Err)
	}
}

func TestRun_StepTimeoutKillsTheStep(t *testing.T) {
	opts := baseOpts(t, "sleep 5")
	opts.StepTimeout = 300 * time.Millisecond
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed || res.Steps[0].Err == nil {
		t.Fatal("an overlong step must fail")
	}
	if !strings.Contains(res.Steps[0].Err.Error(), "did not finish") {
		t.Errorf("err = %v, want a timeout message", res.Steps[0].Err)
	}
}

func TestRun_NilSwapperRunsEveryStepOnTheStartupModel(t *testing.T) {
	opts := baseOpts(t, "true")
	opts.Swapper = nil
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "worker"},
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("res = %+v", res.Steps)
	}
	if res.Steps[0].Alias != "worker" {
		t.Errorf("alias = %q", res.Steps[0].Alias)
	}
}

func TestRun_CancelledContextStopsImmediately(t *testing.T) {
	opts := baseOpts(t, "true")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(ctx, plan, opts)
	if !res.Failed || len(res.Steps) != 0 {
		t.Errorf("res = %+v, want a failed run with no executed steps", res)
	}
}
