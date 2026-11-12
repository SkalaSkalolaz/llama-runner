package cli

import (
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// The assertions below match flag names (-task, -plan-only, -server, -update)
// and the schema token "mode": all of them are spelled identically in the
// Russian and English catalogs, so these tests are locale-independent by
// construction and need no LC_ALL/LANG pinning.
func TestValidateConfig_TaskAndPlanOnly(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantSub string
	}{
		{"ok", func(c *config.Config) { c.Mode, c.Task, c.PlanOnly = "auto", "t", true }, ""},
		{"task without auto", func(c *config.Config) { c.Task, c.PlanOnly = "t", true }, "mode"},
		{"plan-only without auto", func(c *config.Config) { c.PlanOnly = true }, "mode"},
		{"task without plan-only", func(c *config.Config) { c.Mode, c.Task = "auto", "t" }, "-plan-only"},
		{"plan-only without task", func(c *config.Config) { c.Mode, c.PlanOnly = "auto", true }, "-task"},
		{"blank task", func(c *config.Config) { c.Mode, c.Task, c.PlanOnly = "auto", "   ", true }, "-task"},
		{"with server", func(c *config.Config) {
			c.Mode, c.Task, c.PlanOnly, c.ServerOnly = "auto", "t", true, true
		}, "-server"},
		{"with update", func(c *config.Config) {
			c.Mode, c.Task, c.PlanOnly, c.UpdateFlag = "auto", "t", true, true
		}, "-update"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := baseCfg()
			tt.mutate(c)
			err := ValidateConfig(c)
			if tt.wantSub == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestParseFlags_TaskAndPlanOnly(t *testing.T) {
	cfg, err := parseAuto(t, "-p", "auto", "-task", "refactor", "-plan-only")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Task != "refactor" || !cfg.PlanOnly {
		t.Errorf("task = %q, planOnly = %v", cfg.Task, cfg.PlanOnly)
	}
}
