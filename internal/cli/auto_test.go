package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// autoConfigJSON declares the ROOT as a planner on purpose: it is the
// fixture that proves mode is not inherited by the worker profiles.
const autoConfigJSON = `{
  "mode": "auto",
  "alias": "root-planner",
  "parallel": "2",
  "c": "65536",
  "profiles": {
    "auto": {"mode": "auto", "alias": "auto", "model": "~/GGUF/auto.gguf", "c": "32768", "parallel": "1"},
    "big":  {"alias": "big", "model": "~/GGUF/big.gguf", "c": "131072", "parallel": "1"},
    "wave": {"alias": "wave", "model": "~/GGUF/wave.gguf", "c": "262144", "parallel": "2"}
  }
}`

func writeAutoConfig(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "config.json")
	if err := os.WriteFile(path, []byte(autoConfigJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, path
}

func parseAuto(t *testing.T, args ...string) (*config.Config, error) {
	t.Helper()
	home, path := writeAutoConfig(t)
	var buf bytes.Buffer
	return ParseFlagsWithHome(append([]string{"-config", path}, args...), &buf, home)
}

func TestAutoMode_TriggerIsTheFieldNotTheName(t *testing.T) {
	cfg, err := parseAuto(t, "-p", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoMode() {
		t.Error(`-p auto with "mode":"auto" must enable auto mode`)
	}
	if cfg.Alias != "auto" {
		t.Errorf("Alias = %q, want auto", cfg.Alias)
	}
}

func TestAutoMode_NotInheritedByProfiles(t *testing.T) {
	cfg, err := parseAuto(t, "-p", "big")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoMode() {
		t.Error("root mode leaked into -p big")
	}
	for alias, want := range map[string]bool{
		"auto":         true,  // declares its own mode
		"big":          false, // must NOT inherit the root's
		"wave":         false,
		"root-planner": true, // the root entry keeps its own mode
	} {
		pc, ok := cfg.Profiles[alias]
		if !ok {
			t.Fatalf("catalog is missing %q", alias)
		}
		if pc.AutoMode() != want {
			t.Errorf("catalog[%q].AutoMode() = %v, want %v", alias, pc.AutoMode(), want)
		}
	}
}

func TestAutoMode_RootPlannerWithoutProfileFlag(t *testing.T) {
	cfg, err := parseAuto(t)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoMode() {
		t.Error("a root declaring mode:auto must enable auto mode without -p")
	}
}

// The ban list. Each of these breaks a contract the orchestrator owns.
func TestAutoMode_BannedFlags(t *testing.T) {
	for _, flag := range [][]string{
		{"-model", "~/GGUF/other.gguf"},
		{"-download", "~/GGUF/https://example.com/m.gguf"},
		{"-alias", "renamed"},
		{"-parallel", "4"},
		{"-c", "4096"},
	} {
		t.Run(flag[0], func(t *testing.T) {
			_, err := parseAuto(t, append([]string{"-p", "auto"}, flag...)...)
			if err == nil {
				t.Fatalf("%s must be rejected in auto mode", flag[0])
			}
			if !strings.Contains(err.Error(), flag[0]) {
				t.Errorf("error must name the flag: %v", err)
			}
		})
	}
}

func TestAutoMode_AllBannedFlagsReportedAtOnce(t *testing.T) {
	_, err := parseAuto(t, "-p", "auto", "-model", "~/GGUF/x.gguf", "-c", "4096")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"-model", "-c"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must list %s: %v", want, err)
		}
	}
	// -download sets ModelSet too; it must not be reported as -model.
	_, err = parseAuto(t, "-p", "auto", "-download", "~/GGUF/https://example.com/m.gguf")
	if err == nil || strings.Contains(err.Error(), "-model") {
		t.Errorf("-download alone must not be reported as -model: %v", err)
	}
}

func TestAutoMode_TuningFlagsStayAllowed(t *testing.T) {
	cfg, err := parseAuto(t, "-p", "auto",
		"-temp", "0.1", "-ngl", "999", "-fit", "on", "-server", "-v")
	if err != nil {
		t.Fatalf("tuning flags must stay allowed: %v", err)
	}
	if !cfg.AutoMode() || !cfg.ServerOnly {
		t.Error("auto + server must parse as a plain planner launch")
	}
}

// Regression guard: the ban must not touch ordinary launches.
func TestAutoMode_BanDoesNotAffectOtherProfiles(t *testing.T) {
	cfg, err := parseAuto(t, "-p", "big",
		"-model", "~/GGUF/other.gguf", "-alias", "x", "-parallel", "4", "-c", "4096")
	if err != nil {
		t.Fatalf("non-auto profile must accept overrides: %v", err)
	}
	if cfg.AutoMode() {
		t.Error("big is not a planner")
	}
	if cfg.Parallel != "4" || cfg.CtxSize != "4096" {
		t.Errorf("overrides lost: parallel=%q c=%q", cfg.Parallel, cfg.CtxSize)
	}
}

func TestAutoMode_NameWithoutFieldIsOrdinaryProfile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.json")
	body := `{"profiles":{"auto":{"alias":"auto","model":"~/GGUF/a.gguf"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome(
		[]string{"-config", path, "-p", "auto", "-parallel", "3"}, &buf, home)
	if err != nil {
		t.Fatalf("a profile merely NAMED auto must not trigger the ban: %v", err)
	}
	if cfg.AutoMode() {
		t.Error("the name alone must not enable auto mode")
	}
}

func TestValidateConfig_Mode(t *testing.T) {
	for _, ok := range []string{"", "auto"} {
		c := baseCfg()
		c.Mode = ok
		if err := ValidateConfig(c); err != nil {
			t.Errorf("mode %q: %v", ok, err)
		}
	}
	c := baseCfg()
	c.Mode = "autom"
	if err := ValidateConfig(c); err == nil {
		t.Error("a typo in mode must be an error, not a silent no-op")
	}
}

func TestAutoContextBudget(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want int
	}{
		{"nil", nil, 0},
		{"no catalog", &config.Config{CtxSize: "32768", Parallel: "1"}, 0},
		{
			"max over single-slot profiles",
			&config.Config{Profiles: map[string]*config.Config{
				"a": {CtxSize: "32768", Parallel: "1"},
				"b": {CtxSize: "131072", Parallel: "1"},
				"c": {CtxSize: "262144", Parallel: "2"}, // excluded
				"d": {CtxSize: "auto", Parallel: "1"},   // unknown → skipped
				"e": {CtxSize: "65536"},                 // parallel unset → 1
				"f": nil,
			}},
			131072,
		},
		{
			"all multi-slot → no candidate",
			&config.Config{Profiles: map[string]*config.Config{
				"a": {CtxSize: "262144", Parallel: "2"},
				"b": {CtxSize: "131072", Parallel: "3"},
			}},
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AutoContextBudget(tt.cfg); got != tt.want {
				t.Errorf("AutoContextBudget = %d, want %d", got, tt.want)
			}
		})
	}
}

func autoEnvCfg() *config.Config {
	return &config.Config{
		Alias: "auto", Mode: "auto", CtxSize: "32768", Parallel: "1",
		Profiles: map[string]*config.Config{
			"auto": {Alias: "auto", CtxSize: "32768", Parallel: "1"},
			"big":  {Alias: "big", CtxSize: "131072", Parallel: "1"},
			"wave": {Alias: "wave", CtxSize: "262144", Parallel: "2"},
		},
	}
}

func TestBuildClaudeEnv_AutoPublishesCatalogCeiling(t *testing.T) {
	m := envMap(t, BuildClaudeEnv(autoEnvCfg(), "http://h:1", nil))
	for _, k := range []string{
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS",
		"CLAUDE_CODE_AUTO_COMPACT_WINDOW",
	} {
		if m[k] != "131072" {
			t.Errorf("%s = %q, want 131072 (one rule for both)", k, m[k])
		}
	}
}

func TestBuildClaudeEnv_NonAutoKeepsPerSlot(t *testing.T) {
	cfg := autoEnvCfg()
	cfg.Mode = ""
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))
	if m["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] != "32768" {
		t.Errorf("non-auto behaviour changed: %q", m["CLAUDE_CODE_MAX_CONTEXT_TOKENS"])
	}
}

func TestBuildClaudeEnv_AutoFallsBackWhenNoSingleSlotProfile(t *testing.T) {
	cfg := autoEnvCfg()
	cfg.Profiles = map[string]*config.Config{
		"wave": {CtxSize: "262144", Parallel: "2"},
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))
	if m["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] != "32768" {
		t.Errorf("fallback = %q, want the planner's own 32768", m["CLAUDE_CODE_MAX_CONTEXT_TOKENS"])
	}
}

func TestBuildClaudeEnv_AutoUserOverrideStillWins(t *testing.T) {
	base := []string{"CLAUDE_CODE_MAX_CONTEXT_TOKENS=999999"}
	m := envMap(t, BuildClaudeEnv(autoEnvCfg(), "http://h:1", base))
	if m["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] != "999999" {
		t.Errorf("user value lost: %q", m["CLAUDE_CODE_MAX_CONTEXT_TOKENS"])
	}
	if m["CLAUDE_CODE_AUTO_COMPACT_WINDOW"] != "131072" {
		t.Errorf("compact window = %q, want the computed ceiling", m["CLAUDE_CODE_AUTO_COMPACT_WINDOW"])
	}
}

func TestBuildModelMenu_AutoHasNoSwitchEntries(t *testing.T) {
	slots := BuildModelMenu(autoEnvCfg())
	if len(slots) != 5 {
		t.Fatalf("len = %d, want the static 5-slot layout", len(slots))
	}
	for i, s := range slots {
		if strings.HasPrefix(s.ModelValue, config.SwitchPrefix) {
			t.Errorf("slot %d offers a no-op switch: %q", i, s.ModelValue)
		}
		if s.ModelValue != "auto" {
			t.Errorf("slot %d = %q, want the bare planner alias", i, s.ModelValue)
		}
	}
}
