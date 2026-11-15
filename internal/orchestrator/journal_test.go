package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

func journalPlan() planner.Plan {
	return planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
		{ID: "2", Role: "coder", Goal: "g", Alias: "auto", DependsOn: []string{"1"}},
	}}
}

func TestJournal_SaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-run.json")
	j := NewJournal("do it", "/work", journalPlan())
	j.Sync(map[string]StepResult{
		"1": {Step: j.Plan.Steps[0], Alias: "auto", OK: true, Output: "done", Attempts: 2, Duration: time.Second},
	})
	if err := j.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJournal(path)
	if err != nil || loaded == nil {
		t.Fatalf("Load = %v, %v", loaded, err)
	}
	if loaded.Task != "do it" || loaded.WorkDir != "/work" {
		t.Errorf("header lost: %+v", loaded)
	}
	if len(loaded.Plan.Steps) != 2 {
		t.Errorf("plan lost: %+v", loaded.Plan)
	}
	s, ok := loaded.Steps["1"]
	if !ok || !s.OK || s.Attempts != 2 || s.Output != "done" {
		t.Errorf("step lost: %+v", loaded.Steps)
	}
}

func TestLoadJournal_MissingIsNil(t *testing.T) {
	j, err := LoadJournal(filepath.Join(t.TempDir(), "nope.json"))
	if j != nil || err != nil {
		t.Errorf("want (nil, nil), got (%v, %v)", j, err)
	}
}

func TestLoadJournal_CorruptIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-run.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadJournal(path); err == nil {
		t.Error("corrupt journal must be an error")
	}
}

func TestJournal_SeedResults_SkipsFailedAndUnknown(t *testing.T) {
	j := NewJournal("t", "/w", journalPlan())
	j.Steps["1"] = journalStep{OK: true, Alias: "auto", Output: "one"}
	j.Steps["2"] = journalStep{OK: false, Err: "boom"}
	j.Steps["99"] = journalStep{OK: true, Alias: "auto"}
	seeded := j.SeedResults(j.Plan.Steps)
	if len(seeded) != 1 {
		t.Fatalf("seeded = %d, want 1 (failed and unknown skipped)", len(seeded))
	}
	if r := seeded["1"]; !r.OK || r.Output != "one" {
		t.Errorf("seeded[1] = %+v", r)
	}
}

func TestRun_ResumesFromJournal(t *testing.T) {
	dir := t.TempDir()
	script := `case "$*" in
*S2*) echo two; touch f2 ;;
*S1*) echo one; touch f1 ;;
esac`
	opts := baseOpts(t, script)
	opts.Cfg.ClaudeDir = dir
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "S1 write f1", Alias: "auto"},
		{ID: "2", Role: "coder", Goal: "S2 write f2", Alias: "auto", DependsOn: []string{"1"}},
	}}
	j := NewJournal("do it", dir, plan)
	j.Steps["1"] = journalStep{OK: true, Alias: "auto", Output: "one", Attempts: 1}
	opts.Journal = j
	opts.JournalPath = filepath.Join(dir, "last-run.json")
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	if _, err := os.Stat(filepath.Join(dir, "f1")); err == nil {
		t.Error("step 1 must not re-run on resume")
	}
	if _, err := os.Stat(filepath.Join(dir, "f2")); err != nil {
		t.Errorf("step 2 did not run: %v", err)
	}
	if _, err := os.Stat(opts.JournalPath); err != nil {
		t.Errorf("journal not saved: %v", err)
	}
}
