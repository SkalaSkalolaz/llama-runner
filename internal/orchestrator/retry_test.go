package orchestrator

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// baseRetryOpts is like baseOpts but with MaxRetries on the config.
func baseRetryOpts(t *testing.T, claudeScript string) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Cfg: &config.Config{
			Alias:      "auto",
			ClaudeBin:  "claude",
			ClaudeDir:  dir,
			Host:       "127.0.0.1",
			Port:       "8080",
			CtxSize:    "32768",
			Parallel:   "1",
			MaxRetries: 0, // default: no retries
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

// TestRun_NoRetriesByDefault verifies that without MaxRetries the step
// runs exactly once and fails.
func TestRun_NoRetriesByDefault(t *testing.T) {
	opts := baseRetryOpts(t, "exit 3")
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("expected a failed run")
	}
	if len(res.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(res.Steps))
	}
	if res.Steps[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", res.Steps[0].Attempts)
	}
}

// TestRun_RetrySucceedsOnSecondAttempt verifies that a step retried
// succeeds on the second attempt when MaxRetries >= 2.
func TestRun_RetrySucceedsOnSecondAttempt(t *testing.T) {
	opts := baseRetryOpts(t, `
case "$*" in
	*"Previous attempt"*) echo fixed; touch file.txt ;;
	*) exit 3 ;;
esac
`)
	opts.Cfg.MaxRetries = 2
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto", Artifact: "file.txt"},
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("expected success: %+v", res.Steps)
	}
	if res.Steps[0].Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", res.Steps[0].Attempts)
	}
}

// TestRun_RetriesExhausted verifies that when MaxRetries is reached,
// the step fails with Attempts == max+1.
func TestRun_RetriesExhausted(t *testing.T) {
	opts := baseRetryOpts(t, "exit 3")
	opts.Cfg.MaxRetries = 1
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("expected a failed run")
	}
	if res.Steps[0].Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", res.Steps[0].Attempts)
	}
}

// TestRun_VerifyFailureIsRetried verifies that a failing verify command
// triggers a retry.
func TestRun_VerifyFailureIsRetried(t *testing.T) {
	opts := baseRetryOpts(t, "true")
	opts.Runner = scriptRunner{claude: "true", shell: "exit 7"}
	opts.Cfg.MaxRetries = 2
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "tester", Goal: "g", Alias: "auto", Verify: "some check"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatalf("expected a failed run: %+v", res.Steps)
	}
	if res.Steps[0].Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", res.Steps[0].Attempts)
	}
}

// TestRun_MissingArtifactIsRetried verifies that a missing artifact
// triggers a retry.
func TestRun_MissingArtifactIsRetried(t *testing.T) {
	opts := baseRetryOpts(t, "true")
	opts.Cfg.MaxRetries = 1
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto", Artifact: "absent.txt"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("expected a failed run")
	}
	if res.Steps[0].Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", res.Steps[0].Attempts)
	}
	if !strings.Contains(res.Steps[0].Err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", res.Steps[0].Err)
	}
}

// TestRun_TimeoutIsNotRetried verifies that a step timeout does NOT trigger
// a retry.
func TestRun_TimeoutIsNotRetried(t *testing.T) {
	opts := baseRetryOpts(t, "sleep 5")
	opts.StepTimeout = 300 * time.Millisecond
	opts.Cfg.MaxRetries = 3
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("expected a failed run")
	}
	if res.Steps[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want 1 (timeout is not retried)", res.Steps[0].Attempts)
	}
}

// TestRun_CancelledContextDoesNotRetry verifies that a cancelled context
// does not trigger a retry.
func TestRun_CancelledContextDoesNotRetry(t *testing.T) {
	opts := baseRetryOpts(t, "exit 3")
	opts.Cfg.MaxRetries = 3
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(ctx, plan, opts)
	if !res.Failed {
		t.Fatal("expected a failed run")
	}
	if len(res.Steps) != 0 {
		t.Errorf("steps = %d, want 0 (cancelled context)", len(res.Steps))
	}
}

// TestModelRetries verifies the modelRetries function with various configs.
func TestModelRetries(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		profiles map[string]*config.Config
		alias    string
		want     int
	}{
		{
			name:  "nil profiles uses root",
			cfg:   &config.Config{MaxRetries: 2},
			alias: "auto",
			want:  2,
		},
		{
			name: "profile with value",
			cfg:  &config.Config{MaxRetries: 0},
			profiles: map[string]*config.Config{
				"auto": {MaxRetries: 2},
			},
			alias: "auto",
			want:  2,
		},
		{
			name: "profile nil uses root",
			cfg:  &config.Config{MaxRetries: 3},
			profiles: map[string]*config.Config{
				"auto": {MaxRetries: -1},
			},
			alias: "auto",
			want:  3,
		},
		{
			name: "profile clamps to 5",
			cfg:  &config.Config{MaxRetries: 0},
			profiles: map[string]*config.Config{
				"auto": {MaxRetries: 99},
			},
			alias: "auto",
			want:  5,
		},
		{
			name:  "root clamps negative to 0",
			cfg:   &config.Config{MaxRetries: -1},
			alias: "auto",
			want:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				Cfg:      tt.cfg,
				Profiles: tt.profiles,
			}
			got := modelRetries(opts, tt.alias)
			if got != tt.want {
				t.Errorf("modelRetries(%q) = %d, want %d", tt.alias, got, tt.want)
			}
		})
	}
}

// TestRetryBlock verifies the retry block content.
func TestRetryBlock(t *testing.T) {
	prev := StepResult{
		Err:    errors.New("some error message"),
		Output: "some output",
	}
	block := retryBlock(2, 3, prev)
	if !strings.Contains(block, "Previous attempt of this step FAILED") {
		t.Errorf("missing marker:\n%s", block)
	}
	if !strings.Contains(block, "retry 2 of 3") {
		t.Errorf("missing attempt info:\n%s", block)
	}
	if !strings.Contains(block, "some error message") {
		t.Errorf("missing error text:\n%s", block)
	}
	if !strings.Contains(block, "some output") {
		t.Errorf("missing summary:\n%s", block)
	}
	if !strings.Contains(block, "Fix that specific problem") {
		t.Errorf("missing fix instruction:\n%s", block)
	}
}

// TestRetryBlock_LongErrorTruncated verifies that a long error is truncated.
func TestRetryBlock_LongErrorTruncated(t *testing.T) {
	longErr := strings.Repeat("x", 2000)
	prev := StepResult{Err: errors.New(longErr)}
	block := retryBlock(1, 3, prev)
	if strings.Contains(block, longErr) {
		t.Error("long error should be truncated")
	}
	if !strings.Contains(block, "…") {
		t.Error("truncation ellipsis missing")
	}
}

// TestRetryBlock_NoOutput verifies that empty output is not included.
func TestRetryBlock_NoOutput(t *testing.T) {
	prev := StepResult{Err: errors.New("boom")}
	block := retryBlock(1, 3, prev)
	if strings.Contains(block, "Your previous summary:") {
		t.Error("empty output should not produce a summary line")
	}
}

// TestCapRunes verifies the capRunes function.
func TestCapRunes(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{
			name: "short string",
			s:    "hello",
			n:    10,
			want: "hello",
		},
		{
			name: "long string truncated",
			s:    strings.Repeat("x", 100),
			n:    10,
			want: strings.Repeat("x", 10) + "…",
		},
		{
			name: "exact length",
			s:    "12345",
			n:    5,
			want: "12345",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capRunes(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("capRunes(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

// TestRetryable verifies the retryable function.
func TestRetryable(t *testing.T) {
	tests := []struct {
		name string
		kind failureKind
		want bool
	}{
		{"failSetup", failSetup, false},
		{"failTimeout", failTimeout, false},
		{"failExec", failExec, true},
		{"failArtifact", failArtifact, true},
		{"failVerify", failVerify, true},
		{"failNone", failNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retryable(tt.kind)
			if got != tt.want {
				t.Errorf("retryable(%d) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

// TestRun_RetryStdoutOutput verifies that retry messages appear on stdout.
func TestRun_RetryStdoutOutput(t *testing.T) {
	var stdout strings.Builder
	opts := baseRetryOpts(t, "exit 3")
	opts.Cfg.MaxRetries = 1
	opts.Stdout = &stdout
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	Run(context.Background(), plan, opts)
	if !strings.Contains(stdout.String(), "[retry") {
		t.Errorf("stdout should contain retry message:\n%s", stdout.String())
	}
}
