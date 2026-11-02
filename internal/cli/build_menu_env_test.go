package cli

import (
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// envMap collects KEY=VALUE pairs, failing on a repeated key: the menu slots
// are appended one after another, so a duplicated name would silently shadow
// an earlier value depending on which one Claude Code reads.
func envMap(t *testing.T, env []string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			t.Fatalf("env entry without '=': %q", kv)
		}
		if _, dup := m[k]; dup {
			t.Errorf("duplicate env key %q (%q and %q)", k, m[k], v)
		}
		m[k] = v
	}
	return m
}

func coderProfile() config.Profile {
	return config.Profile{Name: "coder", Model: "~/GGUF/Qwen3-Coder-30B-A3B-Instruct-Q4_K_S.gguf",
		Alias: "qwen3-coder", AliasSet: true}
}

func bigProfile() config.Profile {
	return config.Profile{Name: "big", Model: "~/GGUF/Qwen3.8-Flash-Next-UD-Q4_K_XL-merged.gguf",
		Alias: "qwen3.8-flash", AliasSet: true}
}

func tinyProfile() config.Profile {
	return config.Profile{Name: "tiny", Model: "~/GGUF/tiny.gguf", Alias: "tiny-alias", AliasSet: true}
}

func TestBuildClaudeEnv_TwoProfilesFillMenu(t *testing.T) {
	cfg := &config.Config{
		Alias:    "qwen3.8-distil",
		Profiles: []config.Profile{coderProfile(), bigProfile()},
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://127.0.0.1:8082", []string{"PATH=/usr/bin"}))

	want := map[string]string{
		"PATH":                 "/usr/bin",
		"ANTHROPIC_BASE_URL":   "http://127.0.0.1:8082",
		"ANTHROPIC_AUTH_TOKEN": "local-llama",
		"ANTHROPIC_API_KEY":    "local-llama",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":             "openai",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":           "qwen3-coder",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":            "qwen3.8-flash",
		"ANTHROPIC_DEFAULT_FABLE_MODEL":            unusedStandardSlotValue,
		"ANTHROPIC_CUSTOM_MODEL_OPTION":            "qwen3.8-distil",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME":       defaultSlotName,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	if !strings.Contains(m["ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION"], "qwen3.8-distil") {
		t.Errorf("custom description should name the loaded model, got %q",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION"])
	}
	for k := range m {
		if strings.Contains(k, "ANTHROPIC_CUSTOM_MODEL_OPTION_2") {
			t.Errorf("unexpected numbered custom slot %q with only two profiles", k)
		}
	}
}

func TestBuildClaudeEnv_OneProfileMarksHaikuAndFableUnused(t *testing.T) {
	cfg := &config.Config{Alias: "root-alias", Profiles: []config.Profile{coderProfile()}}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "qwen3-coder" {
		t.Errorf("sonnet = %q, want qwen3-coder", m["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != unusedStandardSlotValue {
		t.Errorf("haiku = %q, want the unused marker %q",
			m["ANTHROPIC_DEFAULT_HAIKU_MODEL"], unusedStandardSlotValue)
	}
	if m["ANTHROPIC_DEFAULT_FABLE_MODEL"] != unusedStandardSlotValue {
		t.Errorf("fable = %q, want the unused marker %q",
			m["ANTHROPIC_DEFAULT_FABLE_MODEL"], unusedStandardSlotValue)
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "root-alias" {
		t.Errorf("custom = %q, want root-alias", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
}

func TestBuildClaudeEnv_ThirdProfileFillsFableSlot(t *testing.T) {
	// The third profile occupies the built-in Fable channel, so no custom
	// slot is needed and the numbered family stays closed.
	cfg := &config.Config{
		Alias:    "root-alias",
		Profiles: []config.Profile{coderProfile(), bigProfile(), tinyProfile()},
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "tiny-alias" {
		t.Errorf("fable = %q, want tiny-alias", m["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "root-alias" {
		t.Errorf("first custom slot = %q, want the loaded model root-alias",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	for k := range m {
		if strings.Contains(k, "ANTHROPIC_CUSTOM_MODEL_OPTION_2") {
			t.Errorf("unexpected numbered custom slot %q with exactly three profiles", k)
		}
	}
}

func TestBuildClaudeEnv_FourthProfileAddsNumberedCustomSlot(t *testing.T) {
	// Profiles beyond the third return to the numbered custom family (the
	// first three ride on Sonnet/Haiku/Fable), with the switching hint in
	// the description.
	cfg := &config.Config{
		Alias:    "root-alias",
		Profiles: []config.Profile{coderProfile(), bigProfile(), tinyProfile(), coderProfile()},
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "root-alias" {
		t.Errorf("first custom slot = %q, want the loaded model root-alias",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION_2"] != "qwen3-coder" {
		t.Errorf("slot 2 = %q, want qwen3-coder", m["ANTHROPIC_CUSTOM_MODEL_OPTION_2"])
	}
	if !strings.Contains(m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_DESCRIPTION"], "Switches the local model") {
		t.Errorf("slot 2 description should say picking it switches the local model: %q",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_DESCRIPTION"])
	}
}

func TestBuildClaudeEnv_NoProfilesKeepsHistoricalEnv(t *testing.T) {
	cfg := &config.Config{Alias: "local"}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "local" || m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "local" {
		t.Errorf("sonnet/haiku = %q/%q, want local/local without profiles",
			m["ANTHROPIC_DEFAULT_SONNET_MODEL"], m["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "openai" {
		t.Errorf("opus = %q, want openai", m["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if _, ok := m["ANTHROPIC_DEFAULT_FABLE_MODEL"]; ok {
		t.Error("fable must stay untouched (built-in) when there are no profiles")
	}
	if _, ok := m["ANTHROPIC_CUSTOM_MODEL_OPTION"]; ok {
		t.Error("no custom slot expected when there is nothing to choose between")
	}
	for k, v := range m {
		if strings.Contains(v, "unused") {
			t.Errorf("%s = %q carries an unused marker in a single-model setup", k, v)
		}
	}
}

func TestBuildClaudeEnv_HybridModeKeepsProfileLayout(t *testing.T) {
	// Hybrid mode shares the local layout, so the positions never shift:
	// the Opus slot carries the external model (the entry through which the
	// gateway is picked — the proxy forwards every unknown name upstream),
	// the first extra slot is the proxy's "llama-server" local-route token,
	// and Sonnet/Haiku/Fable/extras fill from the profiles like in local
	// mode.
	cfg := &config.Config{
		Alias:         "local",
		Profiles:      []config.Profile{coderProfile(), bigProfile(), tinyProfile()},
		OpenAIBaseURL: "https://api.example.com/v1",
		OpenAIKey:     "k",
		OpenAIModel:   "ext/model-x",
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	want := map[string]string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL":              "ext/model-x",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":            "qwen3-coder",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":             "qwen3.8-flash",
		"ANTHROPIC_DEFAULT_FABLE_MODEL":             "tiny-alias",
		"ANTHROPIC_CUSTOM_MODEL_OPTION":             localRouteSlotValue,
		"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME":        localRouteSlotName,
		"ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION": localRouteSlotDescription,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	// The loaded local alias must not shadow the gateway or the local route:
	// in hybrid mode no standard slot and no first custom entry carries it.
	for _, k := range []string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_FABLE_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_CUSTOM_MODEL_OPTION",
	} {
		if m[k] == cfg.Alias {
			t.Errorf("%s = %q, want the hybrid value, not the loaded alias", k, m[k])
		}
	}
}

func TestBuildClaudeEnv_HybridModeMarksMissingProfilesUnused(t *testing.T) {
	cfg := &config.Config{
		Alias:         "local",
		Profiles:      []config.Profile{coderProfile()},
		OpenAIBaseURL: "https://api.example.com/v1",
		OpenAIKey:     "k",
		OpenAIModel:   "ext/model-x",
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "qwen3-coder" {
		t.Errorf("sonnet = %q, want qwen3-coder", m["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != unusedStandardSlotValue {
		t.Errorf("haiku = %q, want the unused marker %q",
			m["ANTHROPIC_DEFAULT_HAIKU_MODEL"], unusedStandardSlotValue)
	}
	if m["ANTHROPIC_DEFAULT_FABLE_MODEL"] != unusedStandardSlotValue {
		t.Errorf("fable = %q, want the unused marker %q",
			m["ANTHROPIC_DEFAULT_FABLE_MODEL"], unusedStandardSlotValue)
	}
	if m["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "ext/model-x" {
		t.Errorf("opus = %q, want the external model", m["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != localRouteSlotValue {
		t.Errorf("custom = %q, want the local route token %q",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION"], localRouteSlotValue)
	}
}

func TestBuildClaudeEnv_HybridModeWithoutProfilesKeepsGatewayOnAllSlots(t *testing.T) {
	// Historical hybrid menu: with no profiles there is nothing to lay out,
	// so every standard slot keeps pointing at the gateway and no unused
	// marker appears.
	cfg := &config.Config{
		Alias:         "local",
		OpenAIBaseURL: "https://api.example.com/v1",
		OpenAIKey:     "k",
		OpenAIModel:   "ext/model-x",
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	for _, k := range []string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
	} {
		if m[k] != "ext/model-x" {
			t.Errorf("%s = %q, want ext/model-x in a profile-less hybrid menu", k, m[k])
		}
	}
	if _, ok := m["ANTHROPIC_DEFAULT_FABLE_MODEL"]; ok {
		t.Error("fable must stay untouched (built-in) in a profile-less hybrid menu")
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != localRouteSlotValue {
		t.Errorf("custom = %q, want the local route token", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	for k, v := range m {
		if strings.Contains(v, "unused") {
			t.Errorf("%s = %q carries an unused marker in a single-model setup", k, v)
		}
	}
}

func TestBuildClaudeEnv_ScrubsInheritedMenuVars(t *testing.T) {
	// A nested llama-runner must not leak the outer session's menu into the
	// child: every model-menu variable is dropped from the base environment
	// before the launcher publishes its own, and no key ends up duplicated.
	base := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=ghost-haiku",
		"ANTHROPIC_DEFAULT_FABLE_MODEL=ghost-fable",
		"ANTHROPIC_CUSTOM_MODEL_OPTION=ghost-custom",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_2=ghost-profile",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_2_NAME=Ghost profile",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_3_DESCRIPTION=ghost description",
	}
	cfg := &config.Config{
		Alias:    "qwen3.8-distil",
		Profiles: []config.Profile{coderProfile(), bigProfile()},
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://127.0.0.1:8082", base))

	for k, v := range m {
		if strings.Contains(v, "ghost") {
			t.Errorf("%s = %q leaks an inherited menu value", k, v)
		}
	}
	if m["PATH"] != "/usr/bin" {
		t.Errorf("PATH = %q, want the base environment preserved", m["PATH"])
	}
	// The launcher's own menu still lands on top of the scrubbed base.
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "qwen3.8-flash" {
		t.Errorf("haiku = %q, want the profile alias to be published",
			m["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "qwen3.8-distil" {
		t.Errorf("custom = %q, want the loaded model", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
}
