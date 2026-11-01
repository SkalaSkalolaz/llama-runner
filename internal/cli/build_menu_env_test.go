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

func TestBuildClaudeEnv_OneProfileMarksHaikuUnused(t *testing.T) {
	cfg := &config.Config{Alias: "root-alias", Profiles: []config.Profile{coderProfile()}}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "qwen3-coder" {
		t.Errorf("sonnet = %q, want qwen3-coder", m["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != unusedStandardSlotValue {
		t.Errorf("haiku = %q, want the unused marker %q",
			m["ANTHROPIC_DEFAULT_HAIKU_MODEL"], unusedStandardSlotValue)
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "root-alias" {
		t.Errorf("custom = %q, want root-alias", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
}

func TestBuildClaudeEnv_ThirdProfileAddsNumberedCustomSlot(t *testing.T) {
	cfg := &config.Config{
		Alias:    "root-alias",
		Profiles: []config.Profile{coderProfile(), bigProfile(), tinyProfile()},
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))

	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "root-alias" {
		t.Errorf("first custom slot = %q, want the loaded model root-alias",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION_2"] != "tiny-alias" {
		t.Errorf("slot 2 = %q, want tiny-alias", m["ANTHROPIC_CUSTOM_MODEL_OPTION_2"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_NAME"] != "profile tiny - tiny-alias" {
		t.Errorf("slot 2 name = %q", m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_NAME"])
	}
	if !strings.Contains(m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_DESCRIPTION"], "tiny") {
		t.Errorf("slot 2 description = %q, want it to name the profile",
			m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_DESCRIPTION"])
	}
	// The description must not promise a working switch: only the loaded
	// model can answer requests today.
	if !strings.Contains(m["ANTHROPIC_CUSTOM_MODEL_OPTION_2_DESCRIPTION"], "not implemented") {
		t.Errorf("slot 2 description should say switching is not implemented: %q",
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
	if _, ok := m["ANTHROPIC_CUSTOM_MODEL_OPTION"]; ok {
		t.Error("no custom slot expected when there is nothing to choose between")
	}
	for k, v := range m {
		if strings.Contains(v, "unused") {
			t.Errorf("%s = %q carries an unused marker in a single-model setup", k, v)
		}
	}
}

func TestBuildClaudeEnv_HybridModeIsUnchangedByProfiles(t *testing.T) {
	// The proxy forwards every unknown model name to the external provider,
	// so local profile entries must not appear in hybrid mode at all.
	cfg := &config.Config{
		Alias:         "local",
		Profiles:      []config.Profile{coderProfile(), bigProfile(), tinyProfile()},
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
			t.Errorf("%s = %q, want ext/model-x in hybrid mode", k, m[k])
		}
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "llama-server" {
		t.Errorf("custom = %q, want llama-server", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	for k, v := range m {
		if strings.Contains(v, "qwen") || strings.Contains(v, "unused") {
			t.Errorf("%s = %q leaks a local profile or an unused marker into hybrid mode", k, v)
		}
	}
}
