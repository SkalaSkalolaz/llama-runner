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

func TestBuildClaudeEnv_LocalAllFiveSlots(t *testing.T) {
	cfg := &config.Config{Alias: "qwen3-coder"}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))
	for _, k := range []string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_FABLE_MODEL",
		"ANTHROPIC_CUSTOM_MODEL_OPTION",
	} {
		if m[k] != "qwen3-coder" {
			t.Errorf("%s = %q, want qwen3-coder", k, m[k])
		}
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"] != "local model" {
		t.Errorf("custom name = %q", m["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"])
	}
	for k := range m {
		if strings.Contains(k, "ANTHROPIC_CUSTOM_MODEL_OPTION_2") {
			t.Errorf("unexpected numbered slot %q", k)
		}
	}
}

func TestBuildClaudeEnv_HybridFourExternalOneCustom(t *testing.T) {
	cfg := &config.Config{
		Alias:         "qwen3-coder",
		OpenAIBaseURL: "https://api.example/v1",
		OpenAIModel:   "gpt-4",
		OpenAIKey:     "k",
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://h:1", nil))
	for _, k := range []string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_FABLE_MODEL",
	} {
		if m[k] != "gpt-4" {
			t.Errorf("%s = %q, want gpt-4", k, m[k])
		}
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "qwen3-coder" {
		t.Errorf("custom = %q, want qwen3-coder", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	if !strings.Contains(m["ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION"], "qwen3-coder") {
		t.Errorf("custom description = %q", m["ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION"])
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
	cfg := &config.Config{Alias: "qwen3.8-distil"}
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
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "qwen3.8-distil" {
		t.Errorf("haiku = %q, want the local alias to be published",
			m["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "qwen3.8-distil" {
		t.Errorf("custom = %q, want the loaded model", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
}
