package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProfileConfig writes a config file whose "profiles" section lists the
// given names in the given order and returns the parsed FileConfig.
func writeProfileConfig(t *testing.T, body string) *FileConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := LoadConfigFile(path, t.TempDir())
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if fc == nil {
		t.Fatal("fc = nil, want parsed config")
	}
	return fc
}

func profileNames(profiles []Profile) []string {
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}

func TestLoadConfigFile_RecordsProfileOrder(t *testing.T) {
	// The file order is deliberately neither alphabetical nor reverse.
	fc := writeProfileConfig(t, `{"profiles":{
		"zeta": {"alias": "z"},
		"alpha": {"alias": "a"},
		"mid": {"alias": "m"}
	}}`)

	want := []string{"zeta", "alpha", "mid"}
	got := strings.Join(fc.ProfileOrder, ",")
	if got != strings.Join(want, ",") {
		t.Errorf("ProfileOrder = %v, want %v", fc.ProfileOrder, want)
	}
	if len(fc.Profiles) != 3 {
		t.Errorf("len(Profiles) = %d, want 3", len(fc.Profiles))
	}
}

func TestOrderedProfiles_KeepsFileOrder(t *testing.T) {
	fc := writeProfileConfig(t, `{
		"model": "~/GGUF/root.gguf",
		"profiles":{
			"coder": {"model": "~/GGUF/coder.gguf", "alias": "qwen3-coder"},
			"big":   {"model": "~/GGUF/big.gguf"},
			"tiny":  {"alias": "tiny-alias"}
		}
	}`)

	profiles := OrderedProfiles(fc)
	want := []string{"coder", "big", "tiny"}
	got := strings.Join(profileNames(profiles), ",")
	if got != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", profileNames(profiles), want)
	}

	if profiles[0].Alias != "qwen3-coder" || !profiles[0].AliasSet {
		t.Errorf("coder = %+v, want alias qwen3-coder (set)", profiles[0])
	}
	if profiles[0].Model != "~/GGUF/coder.gguf" {
		t.Errorf("coder model = %q", profiles[0].Model)
	}
	// A profile without alias reports AliasSet=false and an empty Alias: the
	// alias layer, not this one, decides what name it ends up with.
	if profiles[1].AliasSet || profiles[1].Alias != "" {
		t.Errorf("big = %+v, want no alias", profiles[1])
	}
	// A profile without model leaves it empty (it inherits the root model).
	if profiles[2].Model != "" {
		t.Errorf("tiny model = %q, want empty", profiles[2].Model)
	}
}

func TestOrderedProfiles_FallsBackToSortedNames(t *testing.T) {
	// A FileConfig assembled in code has no recorded file order; the result
	// must still be stable across runs rather than follow map iteration.
	one := OrderedProfiles(&FileConfig{Profiles: map[string]FileConfig{
		"zeta":  {},
		"alpha": {},
		"mid":   {},
	}})
	want := strings.Join([]string{"alpha", "mid", "zeta"}, ",")
	if got := strings.Join(profileNames(one), ","); got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
	// Map iteration is randomised per range, so repeat to catch a regression
	// that only shows up occasionally.
	for i := 0; i < 20; i++ {
		if got := strings.Join(profileNames(OrderedProfiles(&FileConfig{Profiles: map[string]FileConfig{
			"zeta": {}, "alpha": {}, "mid": {},
		}})), ","); got != want {
			t.Fatalf("iteration %d: order = %q, want %q", i, got, want)
		}
	}
}

func TestOrderedProfiles_NoProfiles(t *testing.T) {
	tests := []struct {
		name string
		fc   *FileConfig
	}{
		{"nil file", nil},
		{"no field", &FileConfig{}},
		{"empty map", &FileConfig{Profiles: map[string]FileConfig{}}},
		{"order without profiles", &FileConfig{ProfileOrder: []string{"ghost"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OrderedProfiles(tt.fc); got != nil {
				t.Errorf("OrderedProfiles = %+v, want nil", got)
			}
		})
	}
}

func TestLoadConfigFile_NullProfiles(t *testing.T) {
	fc := writeProfileConfig(t, `{"host": "127.0.0.1", "profiles": null}`)
	if got := OrderedProfiles(fc); got != nil {
		t.Errorf("OrderedProfiles = %+v, want nil", got)
	}
}

func TestOrderedProfiles_SkipsStaleNameInOrder(t *testing.T) {
	// ProfileOrder normally mirrors the map exactly; a hand-built config that
	// disagrees must not invent a profile that cannot be applied.
	fc := &FileConfig{
		ProfileOrder: []string{"coder", "ghost", "big"},
		Profiles:     map[string]FileConfig{"coder": {}, "big": {}},
	}
	got := strings.Join(profileNames(OrderedProfiles(fc)), ",")
	if want := "coder,big"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
}

// launchedConfig replays what ParseFlagsWithHome assembles for a launch with
// -p profile: defaults → root file → profile overlay, plus the derived travel
// fields (Profiles, ProfileOverlays, RootSection).
func launchedConfig(t *testing.T, body, profile string) *Config {
	t.Helper()
	fc := writeProfileConfig(t, body)
	cfg := DefaultConfig()
	if err := ApplyFileConfig(cfg, *fc); err != nil {
		t.Fatalf("ApplyFileConfig(root): %v", err)
	}
	if fc.Alias != nil {
		cfg.AliasSet = true
	}
	p, ok := fc.Profiles[profile]
	if !ok {
		t.Fatalf("profile %q not in test config", profile)
	}
	if err := ApplyFileConfig(cfg, p); err != nil {
		t.Fatalf("ApplyFileConfig(%s): %v", profile, err)
	}
	if p.Alias != nil {
		cfg.AliasSet = true
		cfg.AliasFromProfile = true
	}
	cfg.Profiles = OrderedProfiles(fc)
	cfg.ProfileOverlays = fc.Profiles
	root := *fc
	cfg.RootSection = &root
	cfg.Profile = profile
	return cfg
}

func TestApplyRootOverlay_ReappliesRootFields(t *testing.T) {
	cfg := launchedConfig(t, `{
		"model": "~/GGUF/root.gguf", "alias": "root-alias", "c": "111",
		"profiles": {"fast": {"model": "~/GGUF/fast.gguf", "alias": "fast-alias", "c": "222"}}
	}`, "fast")

	root, err := ApplyRootOverlay(cfg)
	if err != nil {
		t.Fatalf("ApplyRootOverlay: %v", err)
	}
	if root.Model != "~/GGUF/root.gguf" {
		t.Errorf("Model = %q, want the root model", root.Model)
	}
	if root.CtxSize != "111" {
		t.Errorf("CtxSize = %q, want the root c (profile value must not survive)", root.CtxSize)
	}
	if root.Alias != "root-alias" || !root.AliasSet {
		t.Errorf("Alias = %q (set %v), want the root alias marked explicit", root.Alias, root.AliasSet)
	}
	if root.AliasFromProfile {
		t.Error("AliasFromProfile = true, want false for a root-sourced alias")
	}
	if root.Profile != "" {
		t.Errorf("Profile = %q, want cleared", root.Profile)
	}

	// The receiver describes the launched profile and must stay untouched.
	if cfg.Model != "~/GGUF/fast.gguf" || cfg.CtxSize != "222" || cfg.Profile != "fast" {
		t.Errorf("receiver mutated: model=%q c=%q profile=%q", cfg.Model, cfg.CtxSize, cfg.Profile)
	}
}

func TestApplyRootOverlay_RootWithoutAliasClearsProfileName(t *testing.T) {
	// A root that names no alias must not keep the switched-from profile's
	// name — the caller resolves a fresh one from the root model.
	cfg := launchedConfig(t, `{
		"model": "~/GGUF/root.gguf",
		"profiles": {"fast": {"model": "~/GGUF/fast.gguf", "alias": "fast-alias"}}
	}`, "fast")

	root, err := ApplyRootOverlay(cfg)
	if err != nil {
		t.Fatalf("ApplyRootOverlay: %v", err)
	}
	if root.Alias != "" {
		t.Errorf("Alias = %q, want cleared for a root without its own alias", root.Alias)
	}
	if root.AliasFromProfile {
		t.Error("AliasFromProfile = true, want false after leaving the profile")
	}
}

func TestApplyRootOverlay_NoRootSectionIsAnError(t *testing.T) {
	// A config assembled in code (no file) has no root section to re-apply.
	if _, err := ApplyRootOverlay(DefaultConfig()); err == nil {
		t.Fatal("err = nil, want an error when no config file was read")
	}
}

func TestUnmarshalJSON_ProfileShapeErrorsStillReported(t *testing.T) {
	// The custom unmarshal must not swallow ordinary decoding errors: a
	// "profiles" section that is not an object stays a config error.
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles": [1,2]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFile(path, t.TempDir()); err == nil {
		t.Fatal("err = nil, want a JSON error for a non-object profiles section")
	}
}
