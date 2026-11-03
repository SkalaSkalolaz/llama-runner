package cli

import (
	"bytes"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// baseCfg fills the fields ValidateConfig checks so a test can isolate one
// rule instead of tripping over unrelated defaults.
func baseCfg() *config.Config {
	return &config.Config{
		Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
		ReadyTimeout: 1e9,
	}
}

func TestValidateConfig_KeylessProviderIsValid(t *testing.T) {
	cfg := baseCfg()
	cfg.OpenAIBaseURL = "https://oai.example.com/v1"
	cfg.OpenAIModel = "Qwen3.6-27B"
	cfg.OpenAIKeyless = true // resolved from -key none by run()
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("err = %v, want nil for a provider configured without a key", err)
	}
}

func TestValidateConfig_MissingKeyStillErrorsWhenNotKeyless(t *testing.T) {
	cfg := baseCfg()
	cfg.OpenAIBaseURL = "https://oai.example.com/v1"
	cfg.OpenAIModel = "Qwen3.6-27B"
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("err = nil, want the missing-key error: an empty key is not the same as no key wanted")
	}
}

func TestValidateConfig_KeylessStillNeedsURLAndModel(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		model string
	}{
		{"no url", "", "Qwen3.6-27B"},
		{"no model", "https://oai.example.com/v1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseCfg()
			cfg.OpenAIBaseURL, cfg.OpenAIModel, cfg.OpenAIKeyless = tt.url, tt.model, true
			if err := ValidateConfig(cfg); err == nil {
				t.Error("err = nil, want an error: keyless removes the key, not the other settings")
			}
		})
	}
}

func TestParseFlags_KeyFlagPresenceIsRecorded(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"-key none", []string{"-key", "none"}, true},
		{`-key ""`, []string{"-key", ""}, true},
		{"-key value", []string{"-key", "sk-1"}, true},
		{"no -key", []string{"-openai", "https://x/v1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var buf bytes.Buffer
			cfg, err := ParseFlagsWithHome(tt.args, &buf, dir)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if cfg.OpenAIKeyFromCLI != tt.want {
				t.Errorf("OpenAIKeyFromCLI = %v, want %v", cfg.OpenAIKeyFromCLI, tt.want)
			}
		})
	}
}

// TestKeylessFlow walks the same order run() does — parse, then resolve the
// key — so the flag-presence plumbing and the sentinel are covered together.
func TestKeylessFlow(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		envKey      string
		wantKey     string
		wantKeyless bool
	}{
		{
			name:        "-key none ignores an exported key",
			args:        []string{"-openai", "https://oai.example.com/v1", "-model-openai", "Qwen3.6-27B", "-key", "none"},
			envKey:      "sk-from-env",
			wantKey:     "",
			wantKeyless: true,
		},
		{
			name:        `-key "" ignores an exported key`,
			args:        []string{"-openai", "https://oai.example.com/v1", "-model-openai", "Qwen3.6-27B", "-key", ""},
			envKey:      "sk-from-env",
			wantKey:     "",
			wantKeyless: true,
		},
		{
			name:    "omitting -key still uses the environment",
			args:    []string{"-openai", "https://oai.example.com/v1", "-model-openai", "Qwen3.6-27B"},
			envKey:  "sk-from-env",
			wantKey: "sk-from-env",
		},
		{
			name:    "an explicit key is used",
			args:    []string{"-openai", "https://oai.example.com/v1", "-model-openai", "Qwen3.6-27B", "-key", "sk-cli"},
			envKey:  "sk-from-env",
			wantKey: "sk-cli",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg, err := ParseFlagsWithHome(tt.args, &buf, t.TempDir())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			env := func(k string) string {
				if k == "LLAMA_RUNNER_OPENAI_KEY" {
					return tt.envKey
				}
				return ""
			}
			var stderr bytes.Buffer
			key, keyless, err := ResolveOpenAIKey(cfg.OpenAIKey, cfg.OpenAIKeyFromCLI, env, t.TempDir(), &stderr)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			cfg.OpenAIKey, cfg.OpenAIKeyless = key, keyless

			if cfg.OpenAIKey != tt.wantKey || cfg.OpenAIKeyless != tt.wantKeyless {
				t.Fatalf("resolved (%q, keyless=%v), want (%q, keyless=%v)",
					cfg.OpenAIKey, cfg.OpenAIKeyless, tt.wantKey, tt.wantKeyless)
			}
			if err := ValidateConfig(cfg); err != nil {
				t.Errorf("ValidateConfig = %v, want nil", err)
			}
			if want := tt.wantKey != "" || tt.wantKeyless; cfg.OpenAIProviderConfigured() != want {
				t.Errorf("OpenAIProviderConfigured = %v, want %v", cfg.OpenAIProviderConfigured(), want)
			}
		})
	}
}

func TestBuildClaudeEnv_KeylessProviderSharesHybridMenu(t *testing.T) {
	// Keyless changes only whether the proxy authenticates upstream; the
	// menu is laid out exactly like the keyed hybrid one: the external model
	// fills the four standard positions, the custom entry carries the local
	// alias.
	cfg := &config.Config{
		Alias:         "local",
		OpenAIBaseURL: "https://oai.example.com/v1",
		OpenAIModel:   "Qwen3.6-27B",
		OpenAIKeyless: true,
	}
	m := envMap(t, BuildClaudeEnv(cfg, "http://proxy:8082", nil))

	for _, k := range []string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_FABLE_MODEL",
	} {
		if m[k] != "Qwen3.6-27B" {
			t.Errorf("%s = %q, want the external model", k, m[k])
		}
	}
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "local" {
		t.Errorf("custom = %q, want the local alias", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
}
