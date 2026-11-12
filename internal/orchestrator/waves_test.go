package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

func stepNoDeps(id, alias string) planner.Step {
	return planner.Step{ID: id, Role: "coder", Goal: "goal " + id, Alias: alias}
}

func TestModelParallel(t *testing.T) {
	tests := []struct {
		name     string
		profiles map[string]*config.Config
		alias    string
		want     int
	}{
		{"nil profiles", nil, "x", 1},
		{"missing alias", map[string]*config.Config{}, "x", 1},
		{"nil profile entry", map[string]*config.Config{"x": nil}, "x", 1},
		{"empty value", map[string]*config.Config{"x": {Parallel: ""}}, "x", 1},
		{"garbage", map[string]*config.Config{"x": {Parallel: "two"}}, "x", 1},
		{"zero", map[string]*config.Config{"x": {Parallel: "0"}}, "x", 1},
		{"negative", map[string]*config.Config{"x": {Parallel: "-3"}}, "x", 1},
		{"three", map[string]*config.Config{"x": {Parallel: "3"}}, "x", 3},
		{"spaces", map[string]*config.Config{"x": {Parallel: " 2 "}}, "x", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelParallel(tt.profiles, tt.alias); got != tt.want {
				t.Errorf("modelParallel = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRun_StepsOnOneModelRunConcurrently(t *testing.T) {
	opts := baseOpts(t, "sleep 0.3; echo ok")
	opts.Profiles["worker"].Parallel = "3"
	opts.Swapper = &fakeSwapper{current: "worker",
		urls: map[string]string{"worker": "http://u:2"}}
	plan := planner.Plan{Steps: []planner.Step{
		stepNoDeps("1", "worker"),
		stepNoDeps("2", "worker"),
		stepNoDeps("3", "worker"),
	}}
	start := time.Now()
	res := Run(context.Background(), plan, opts)
	elapsed := time.Since(start)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(res.Steps))
	}
	// Sequential execution would take >= 0.9s of sleeps alone; a single
	// wave on 3 slots finishes in roughly one sleep.
	if elapsed >= 800*time.Millisecond {
		t.Errorf("elapsed = %v — the steps did not run concurrently", elapsed)
	}
}

func TestRun_ParallelOneRunsSequentially(t *testing.T) {
	opts := baseOpts(t, "sleep 0.25; echo ok")
	opts.Profiles["worker"].Parallel = "1"
	opts.Swapper = &fakeSwapper{current: "worker",
		urls: map[string]string{"worker": "http://u:2"}}
	plan := planner.Plan{Steps: []planner.Step{
		stepNoDeps("1", "worker"),
		stepNoDeps("2", "worker"),
	}}
	start := time.Now()
	res := Run(context.Background(), plan, opts)
	elapsed := time.Since(start)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	// Two waves of one slot each: the sleeps cannot overlap.
	if elapsed < 500*time.Millisecond {
		t.Errorf("elapsed = %v — a parallel=1 model must run steps one by one", elapsed)
	}
}

func TestRun_PrefersLoadedModelBeforeSwapping(t *testing.T) {
	opts := baseOpts(t, "true")
	sw := &fakeSwapper{current: "auto",
		urls: map[string]string{"auto": "http://u:1", "worker": "http://u:2"}}
	opts.Swapper = sw
	plan := planner.Plan{Steps: []planner.Step{
		stepNoDeps("1", "auto"),
		stepNoDeps("2", "worker"),
		stepNoDeps("3", "auto"),
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	// Both auto steps drain before the only swap of the whole run.
	if len(sw.calls) != 1 || sw.calls[0] != "worker" {
		t.Errorf("swaps = %v, want exactly [worker]", sw.calls)
	}
	if len(res.Steps) != 3 {
		t.Errorf("steps = %d, want 3", len(res.Steps))
	}
}

func TestRun_DependentChainMinimizesSwaps(t *testing.T) {
	opts := baseOpts(t, "true")
	sw := &fakeSwapper{current: "auto",
		urls: map[string]string{"auto": "http://u:1", "worker": "http://u:2"}}
	opts.Swapper = sw
	plan := planner.Plan{Steps: []planner.Step{
		stepNoDeps("1", "auto"),
		{ID: "2", Role: "coder", Goal: "g", Alias: "worker", DependsOn: []string{"1"}},
		{ID: "3", Role: "coder", Goal: "g", Alias: "worker", DependsOn: []string{"2"}},
		{ID: "4", Role: "tester", Goal: "g", Alias: "auto", DependsOn: []string{"2", "3"}},
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	// Worker steps 2 and 3 run back to back on a single load of worker.
	if len(sw.calls) != 2 || sw.calls[0] != "worker" || sw.calls[1] != "auto" {
		t.Errorf("swaps = %v, want [worker auto]", sw.calls)
	}
	if len(res.Steps) != 4 {
		t.Errorf("steps = %d, want 4", len(res.Steps))
	}
}

func TestRun_FailureLetsTheWaveFinishThenStops(t *testing.T) {
	// The script fails fast when the step prompt carries FAIL, otherwise
	// succeeds after a short sleep: the failing step finishes first while
	// its wave-mate is still running.
	script := `case "$*" in *FAIL*) exit 3 ;; esac; sleep 0.3; echo done`
	opts := baseOpts(t, script)
	opts.Profiles["worker"].Parallel = "2"
	opts.Swapper = &fakeSwapper{current: "worker",
		urls: map[string]string{"worker": "http://u:2"}}
	slow := stepNoDeps("2", "worker")
	slow.Goal = "slow ok"
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "FAIL fast", Alias: "worker"},
		slow,
		stepNoDeps("3", "worker"), // never scheduled
	}}
	res := Run(context.Background(), plan, opts)
	if !res.Failed {
		t.Fatal("a failing step must fail the run")
	}
	if len(res.Steps) != 2 {
		t.Fatalf("steps = %d, want 2: the wave finishes, step 3 is never scheduled",
			len(res.Steps))
	}
	for _, r := range res.Steps {
		switch r.Step.ID {
		case "1":
			if r.OK {
				t.Error("step 1 must fail")
			}
		case "2":
			if !r.OK {
				t.Errorf("step 2 must be allowed to finish: %+v", r)
			}
		}
	}
}
