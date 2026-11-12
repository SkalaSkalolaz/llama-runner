package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func planOnlyCfg() *config.Config {
	return &config.Config{
		Alias: "auto", Mode: "auto", Task: "do it", PlanOnly: true,
		CtxSize: "32768", Parallel: "1",
		Profiles: map[string]*config.Config{
			"auto":  {Alias: "auto", CtxSize: "32768", Parallel: "1"},
			"coder": {Alias: "coder", CtxSize: "262144", Parallel: "2"},
		},
		ProfileDescriptions: map[string]string{"coder": "code"},
	}
}

func TestCatalogModels_PerSlotContextAndOrder(t *testing.T) {
	got := catalogModels(planOnlyCfg())
	if len(got) != 2 || got[0].Alias != "auto" || got[1].Alias != "coder" {
		t.Fatalf("models = %+v, want sorted [auto coder]", got)
	}
	if got[1].Context != 131072 || got[1].Parallel != 2 {
		t.Errorf("coder = %+v, want per-slot 131072 and 2 slots", got[1])
	}
	if got[1].Description != "code" {
		t.Errorf("description lost: %+v", got[1])
	}
}

func TestCatalogModels_NoProfilesUsesPlannerItself(t *testing.T) {
	cfg := planOnlyCfg()
	cfg.Profiles = nil
	got := catalogModels(cfg)
	if len(got) != 1 || got[0].Alias != "auto" || got[0].Context != 32768 {
		t.Errorf("models = %+v, want the planner alone", got)
	}
}

func planUpstream(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunPlanOnly_PrintsJSONPlan(t *testing.T) {
	srv := planUpstream(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"coder"}]}`)
	var out, errb bytes.Buffer
	if code := runPlanOnly(context.Background(), planOnlyCfg(), srv.URL, &out, &errb); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errb.String())
	}
	body := out.String()
	i := strings.Index(body, "{")
	if i < 0 {
		t.Fatalf("no JSON in output: %s", body)
	}
	var plan struct {
		Steps []struct{ ID, Alias string } `json:"steps"`
	}
	if err := json.Unmarshal([]byte(body[i:]), &plan); err != nil {
		t.Fatalf("stdout is not a clean plan JSON: %v\n%s", err, body)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Alias != "coder" {
		t.Errorf("plan = %+v", plan)
	}
	// MsgPlanBuilding goes to stderr by design. It MUST be there
	// (progress is diagnostics) and it MUST NOT be in stdout (stdout is
	// the machine-readable channel for -plan-only > plan.json).
	if !strings.Contains(errb.String(), "Planner") &&
		!strings.Contains(errb.String(), "Планировщик") {
		t.Errorf("expected the building-progress line on stderr: %q", errb.String())
	}
	if strings.Contains(out.String(), "Planner") || strings.Contains(out.String(), "Планировщик") {
		t.Errorf("progress leaked into stdout: %q", out.String())
	}
}

func TestRunPlanOnly_FallbackWarnsOnStderr(t *testing.T) {
	srv := planUpstream(t, "no json at all")
	var out, errb bytes.Buffer
	if code := runPlanOnly(context.Background(), planOnlyCfg(), srv.URL, &out, &errb); code != 0 {
		t.Fatalf("a fallback is a warning, code = %d", code)
	}
	if !strings.Contains(errb.String(), "warning") {
		t.Errorf("fallback not announced: %s", errb.String())
	}
	if strings.Contains(out.String(), "warning") {
		t.Error("the warning contaminated the JSON stdout")
	}
}

func TestRunPlanOnly_DeadServerIsExitOne(t *testing.T) {
	var out, errb bytes.Buffer
	code := runPlanOnly(context.Background(), planOnlyCfg(), "http://127.0.0.1:1", &out, &errb)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Error("no diagnostic on stderr")
	}
}
