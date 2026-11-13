package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// priorChain builds n steps 1..n where each depends on the previous one,
// plus the matching byID map of successful results.
func priorChain(n int) ([]planner.Step, map[string]StepResult) {
	steps := make([]planner.Step, 0, n)
	byID := make(map[string]StepResult, n)
	for i := 1; i <= n; i++ {
		id := strconv.Itoa(i)
		s := planner.Step{ID: id, Role: "coder", Goal: "goal " + id, Alias: "auto"}
		if i > 1 {
			s.DependsOn = []string{strconv.Itoa(i - 1)}
		}
		steps = append(steps, s)
		byID[id] = StepResult{Step: s, Alias: "auto", OK: true, Output: "sum " + id}
	}
	return steps, byID
}

func priorIDs(rs []StepResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Step.ID
	}
	return out
}

func TestPriorContext_TransitiveDeps(t *testing.T) {
	steps, byID := priorChain(3)
	target := planner.Step{ID: "4", DependsOn: []string{"3"}}
	got := priorIDs(priorContext(target, steps, byID))
	if strings.Join(got, ",") != "1,2,3" {
		t.Errorf("prior = %v, want [1 2 3] oldest first", got)
	}
}

func TestPriorContext_NoDeps(t *testing.T) {
	steps, byID := priorChain(3)
	if got := priorContext(planner.Step{ID: "9"}, steps, byID); len(got) != 0 {
		t.Errorf("prior = %v, want none", priorIDs(got))
	}
}

func TestPriorContext_MissingDepSkipped(t *testing.T) {
	steps, byID := priorChain(2)
	delete(byID, "1")
	target := planner.Step{ID: "3", DependsOn: []string{"2", "9"}}
	got := priorIDs(priorContext(target, steps, byID))
	if strings.Join(got, ",") != "2" {
		t.Errorf("prior = %v, want [2] (missing deps skipped)", got)
	}
}

func TestPriorContext_DedupSharedDep(t *testing.T) {
	s1 := planner.Step{ID: "1", Role: "coder", Goal: "g", Alias: "auto"}
	s2 := planner.Step{ID: "2", Role: "coder", Goal: "g", Alias: "auto", DependsOn: []string{"1"}}
	s3 := planner.Step{ID: "3", Role: "coder", Goal: "g", Alias: "auto", DependsOn: []string{"1"}}
	steps := []planner.Step{s1, s2, s3}
	byID := map[string]StepResult{
		"1": {Step: s1, OK: true}, "2": {Step: s2, OK: true}, "3": {Step: s3, OK: true},
	}
	target := planner.Step{ID: "4", DependsOn: []string{"2", "3"}}
	got := priorIDs(priorContext(target, steps, byID))
	if strings.Join(got, ",") != "1,2,3" {
		t.Errorf("prior = %v, want shared dep once: [1 2 3]", got)
	}
}

func TestPriorContext_CapDropsOldest(t *testing.T) {
	n := priorContextCap + 2
	steps, byID := priorChain(n)
	target := planner.Step{ID: "x", DependsOn: []string{strconv.Itoa(n)}}
	got := priorContext(target, steps, byID)
	if len(got) != priorContextCap {
		t.Fatalf("len = %d, want %d", len(got), priorContextCap)
	}
	if got[0].Step.ID != "3" {
		t.Errorf("oldest kept = %s, want 3 (1 and 2 dropped)", got[0].Step.ID)
	}
}

func TestStepPrompt_IncludesPrior(t *testing.T) {
	steps, byID := priorChain(1)
	target := planner.Step{ID: "2", Role: "coder", Goal: "g", DependsOn: []string{"1"}}
	prior := buildPriorBlock(priorContext(target, steps, byID), "")
	p := stepPrompt("task", target, prior)
	for _, want := range []string{"Completed steps", "goal 1", "sum 1"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestStepPrompt_EmptyPriorUnchanged(t *testing.T) {
	step := planner.Step{ID: "1", Role: "coder", Goal: "g"}
	want := "You are one agent in an automated pipeline that executes a plan step by step.\n" +
		"Overall task:\ndo it\n" +
		"Your step:\n" +
		"- role: coder\n" +
		"- goal: g\n" +
		"\nDo exactly this step, working in the current directory. " +
		"When done, print one short paragraph: what you changed and where the artifact is.\n"
	if got := stepPrompt("do it", step, ""); got != want {
		t.Errorf("prompt with empty prior changed:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestBuildPriorBlock_EmbedsArtifactContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("ARTIFACT-BODY"), 0o600); err != nil {
		t.Fatal(err)
	}
	priors := []StepResult{{
		Step:  planner.Step{ID: "1", Role: "coder", Goal: "g", Artifact: p},
		Alias: "auto", OK: true,
	}}
	b := buildPriorBlock(priors, dir)
	for _, want := range []string{"--- begin artifact " + p, "ARTIFACT-BODY", "--- end artifact " + p} {
		if !strings.Contains(b, want) {
			t.Errorf("block missing %q:\n%s", want, b)
		}
	}
}

func TestBuildPriorBlock_ProseArtifactNoContent(t *testing.T) {
	priors := []StepResult{{
		Step:  planner.Step{ID: "1", Role: "reviewer", Goal: "g", Artifact: "findings report"},
		Alias: "auto", OK: true,
	}}
	b := buildPriorBlock(priors, t.TempDir())
	if strings.Contains(b, "begin artifact") {
		t.Errorf("prose must not embed content:\n%s", b)
	}
	if !strings.Contains(b, "NOT a file path") {
		t.Errorf("prose marker missing:\n%s", b)
	}
}

func TestRun_StepDurationRecorded(t *testing.T) {
	opts := baseOpts(t, "sleep 0.05; echo ok")
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	if res.Steps[0].Duration < 40*time.Millisecond {
		t.Errorf("step duration = %v, want >= 40ms", res.Steps[0].Duration)
	}
	if res.Duration < res.Steps[0].Duration {
		t.Errorf("total %v < step %v", res.Duration, res.Steps[0].Duration)
	}
}

func TestRun_TotalDurationRecorded(t *testing.T) {
	opts := baseOpts(t, "echo ok")
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	res := Run(context.Background(), plan, opts)
	if res.Failed || res.Duration <= 0 {
		t.Errorf("res = %+v, want a successful run with a positive total", res)
	}
}
