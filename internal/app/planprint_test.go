package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

func TestPrintPlan_Full(t *testing.T) {
	plan := planner.Plan{
		Summary: "Refactor orchestrator",
		Steps: []planner.Step{
			{ID: "1", Role: "coder", Goal: "Split orchestrator.go", Alias: "coder",
				Artifact: "internal/orchestrator/orchestrator.go",
				Verify:   "go build ./internal/orchestrator/"},
			{ID: "2", Role: "coder", Goal: "Move wave scheduler", Alias: "coder",
				Verify: "go test -race ./internal/orchestrator/", DependsOn: []string{"1"}},
		},
	}
	want := "summary:  Refactor orchestrator\n" +
		"models:   coder × 2\n" +
		"\n" +
		"1/2 [1] coder · coder\n" +
		"goal:     Split orchestrator.go\n" +
		"artifact: internal/orchestrator/orchestrator.go\n" +
		"verify:   go build ./internal/orchestrator/\n" +
		"\n" +
		"2/2 [2] coder · coder\n" +
		"goal:     Move wave scheduler\n" +
		"verify:   go test -race ./internal/orchestrator/\n" +
		"depends:  1\n"
	var buf bytes.Buffer
	printPlan(&buf, plan)
	if got := buf.String(); got != want {
		t.Errorf("listing mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintPlan_EmptySummary(t *testing.T) {
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	var buf bytes.Buffer
	printPlan(&buf, plan)
	got := buf.String()
	if strings.Contains(got, "summary:") {
		t.Errorf("empty summary printed: %q", got)
	}
	if !strings.HasPrefix(got, "models:") {
		t.Errorf("listing must start with models line: %q", got)
	}
}

func TestPrintPlan_MultipleModels(t *testing.T) {
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "b"},
		{ID: "2", Role: "coder", Goal: "g", Alias: "a"},
		{ID: "3", Role: "coder", Goal: "g", Alias: "b"},
	}}
	var buf bytes.Buffer
	printPlan(&buf, plan)
	if !strings.Contains(buf.String(), "models:   a × 1, b × 2") {
		t.Errorf("aggregate wrong (alphabetical, counts): %q", buf.String())
	}
}

func TestPrintPlan_OptionalFields(t *testing.T) {
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto"},
	}}
	var buf bytes.Buffer
	printPlan(&buf, plan)
	got := buf.String()
	for _, absent := range []string{"artifact:", "verify:", "depends:"} {
		if strings.Contains(got, absent) {
			t.Errorf("unset field %q printed: %q", absent, got)
		}
	}
	if !strings.Contains(got, "goal:") {
		t.Errorf("goal missing: %q", got)
	}
}

func TestPrintPlan_DependenciesJoined(t *testing.T) {
	plan := planner.Plan{Steps: []planner.Step{
		{ID: "3", Role: "tester", Goal: "g", Alias: "auto", DependsOn: []string{"1", "2"}},
	}}
	var buf bytes.Buffer
	printPlan(&buf, plan)
	if !strings.Contains(buf.String(), "depends:  1, 2") {
		t.Errorf("deps not joined: %q", buf.String())
	}
}
