package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestBuildServerArgs_Minimal(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: "8080", Tools: "all",
		CtxSize: "16384", BatchSize: "512", UbatchSize: "128",
		FlashAttn: "auto", Parallel: "1", Jinja: true}
	args := BuildServerArgs(cfg, ServerPaths{Model: "/m.gguf"})

	want := []string{"-m", "/m.gguf", "--host", "127.0.0.1", "--port", "8080",
		"--tools", "all", "-c", "16384", "-b", "512", "-ub", "128",
		"-fa", "auto", "--parallel", "1", "--jinja"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v\nwant %v", args, want)
	}
}

func TestBuildServerArgs_FullOptional(t *testing.T) {
	cfg := &config.Config{Host: "0.0.0.0", Port: "9000", Tools: "read",
		CtxSize: "1", BatchSize: "2", UbatchSize: "3", FlashAttn: "on",
		Parallel: "4", Jinja: false, McpProxy: true, LazyMmap: true,
		Alias: "local", NGL: "999", Temp: "0.7", TopP: "0.9",
		CacheTypeK: "q8_0", CacheTypeV: "q8_0", FitMode: "off", LogVerbosity: "4",
		SpecType: "draft-mtp", SpecDraftNMax: "2", SpecDraftNGL: "0"}
	paths := ServerPaths{Model: "/m", Mmproj: "/mm", Draft: "/d", Template: "/t"}
	args := BuildServerArgs(cfg, paths)

	required := [][]string{
		{"-m", "/m"}, {"--host", "0.0.0.0"}, {"--port", "9000"},
		{"--webui-mcp-proxy"}, {"-lm", "mmap", "--lazy-mode", "on"},
		{"--alias", "local"}, {"-ngl", "999"}, {"--temp", "0.7"},
		{"--top-p", "0.9"}, {"--cache-type-k", "q8_0"}, {"--cache-type-v", "q8_0"},
		{"--fit", "off"}, {"-lv", "4"}, {"--mmproj", "/mm"},
		{"--spec-type", "draft-mtp"}, {"--spec-draft-n-max", "2"},
		{"--spec-draft-ngl", "0"}, {"--spec-draft-model", "/d"},
		{"--chat-template-file", "/t"},
	}
	joined := strings.Join(args, " ")
	for _, pair := range required {
		if !strings.Contains(joined, strings.Join(pair, " ")) {
			t.Errorf("args missing %v\nfull: %s", pair, joined)
		}
	}
	if strings.Contains(joined, "--jinja") {
		t.Error("--jinja should not be present when Jinja=false")
	}
}

func TestBuildClaudeEnv(t *testing.T) {
	cfg := &config.Config{Alias: "local"}
	env := BuildClaudeEnv(cfg, "http://127.0.0.1:8080", []string{"PATH=/usr/bin"})
	m := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		m[parts[0]] = parts[1]
	}
	checks := map[string]string{
		"PATH":                           "/usr/bin",
		"ANTHROPIC_BASE_URL":             "http://127.0.0.1:8080",
		"ANTHROPIC_AUTH_TOKEN":           "local-llama",
		"ANTHROPIC_API_KEY":              "local-llama",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "local",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "local",
		// Local mode: every position carries the single local alias.
		"ANTHROPIC_DEFAULT_OPUS_MODEL":       "local",
		"ANTHROPIC_DEFAULT_FABLE_MODEL":      "local",
		"ANTHROPIC_CUSTOM_MODEL_OPTION":      "local",
		"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME": "local model",
	}
	for k, v := range checks {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
}

func TestBuildClaudeEnv_NoAlias(t *testing.T) {
	cfg := &config.Config{}
	env := BuildClaudeEnv(cfg, "http://h:1", nil)
	m := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		m[parts[0]] = parts[1]
	}
	// Without a local alias there is nothing to publish: BuildModelMenu
	// returns nil and no menu variable appears at all.
	for _, k := range []string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_FABLE_MODEL",
		"ANTHROPIC_CUSTOM_MODEL_OPTION",
	} {
		if _, ok := m[k]; ok {
			t.Errorf("unexpected %s=%q", k, m[k])
		}
	}
}

func TestBuildClaudeEnv_OpenAIWithoutModelStaysLocalMenu(t *testing.T) {
	// -openai alone (no -model-openai) is not a configured provider: the menu
	// stays the local one — every position carries the local alias.
	cfg := &config.Config{Alias: "local", OpenAIBaseURL: "https://api.example.com/v1"}
	env := BuildClaudeEnv(cfg, "http://h:1", nil)
	m := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		m[parts[0]] = parts[1]
	}
	for _, k := range []string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_FABLE_MODEL",
		"ANTHROPIC_CUSTOM_MODEL_OPTION",
	} {
		if m[k] != "local" {
			t.Errorf("%s = %q, want local", k, m[k])
		}
	}
}

func TestBuildClaudeArgs_Empty(t *testing.T) {
	args := BuildClaudeArgs(&config.Config{}, "")
	if len(args) != 0 {
		t.Errorf("got %v, want empty", args)
	}
}

func TestBuildClaudeArgs_WithMCP(t *testing.T) {
	args := BuildClaudeArgs(&config.Config{}, "/tmp/mcp.json")
	want := []string{"--mcp-config", "/tmp/mcp.json"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_Strict(t *testing.T) {
	args := BuildClaudeArgs(&config.Config{McpStrict: true}, "/tmp/mcp.json")
	want := []string{"--mcp-config", "/tmp/mcp.json", "--strict-mcp-config"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_LocalPinsLoadedModel(t *testing.T) {
	// The pin is the whole point: Claude Code must start naming the model
	// the server actually loaded, not whatever a previous session persisted.
	args := BuildClaudeArgs(&config.Config{Alias: "qwen3.8-flash"}, "")
	want := []string{"--model", "qwen3.8-flash"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_ModelPinPrecedesMCP(t *testing.T) {
	args := BuildClaudeArgs(&config.Config{Alias: "m", McpStrict: true}, "/tmp/mcp.json")
	want := []string{"--model", "m", "--mcp-config", "/tmp/mcp.json", "--strict-mcp-config"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_HybridPinsExternalModel(t *testing.T) {
	// Hybrid sessions must pin the external model as the starting point:
	// it fills the four standard /model slots, so it is exactly what Claude
	// Code labels "Default (recommended)". Without the pin Claude Code
	// resumes the persisted model from a prior session, and when that name
	// no longer matches any menu entry it appears as a phantom "Custom
	// model" line without a label.
	cfg := &config.Config{Alias: "qwen3-coder", OpenAIBaseURL: "https://gw/v1",
		OpenAIModel: "gpt-5.1", OpenAIKey: "k"}
	args := BuildClaudeArgs(cfg, "")
	want := []string{"--model", "gpt-5.1"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_HybridKeylessAlsoPinsExternalModel(t *testing.T) {
	// The keyless provider shares the hybrid menu, so it also pins the
	// external model as the starting point.
	cfg := &config.Config{Alias: "qwen3-coder", OpenAIBaseURL: "https://oai.example.com/v1",
		OpenAIModel: "Qwen3.6-27B", OpenAIKeyless: true}
	args := BuildClaudeArgs(cfg, "")
	want := []string{"--model", "Qwen3.6-27B"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_HalfConfiguredHybridFallsBackToLocal(t *testing.T) {
	// A provider missing its model (URL only) is not configured:
	// OpenAIProviderConfigured() is false, so the local alias is pinned
	// instead — the same as before the change.
	cfg := &config.Config{Alias: "local-x", OpenAIBaseURL: "https://gw/v1"}
	args := BuildClaudeArgs(cfg, "")
	want := []string{"--model", "local-x"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeEnv_ScrubsInheritedModelPin(t *testing.T) {
	// A stale ANTHROPIC_MODEL from an outer session must not survive into
	// the child: it would contradict (or override) this launcher's pin.
	env := BuildClaudeEnv(&config.Config{Alias: "fresh"}, "http://h:1",
		[]string{"ANTHROPIC_MODEL=stale-model", "PATH=/usr/bin"})
	for _, kv := range env {
		if strings.HasPrefix(kv, "ANTHROPIC_MODEL=") {
			t.Errorf("stale pin survived scrub: %q", kv)
		}
	}
}

func TestBuildServerArgs_Reasoning(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: "8080", Tools: "all",
		CtxSize: "16384", BatchSize: "512", UbatchSize: "128",
		FlashAttn: "auto", Parallel: "1", Jinja: true,
		Reasoning: "on", ReasoningBudget: "2048"}
	args := BuildServerArgs(cfg, ServerPaths{Model: "/m.gguf"})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--reasoning on") {
		t.Errorf("missing --reasoning on in %s", joined)
	}
	if !strings.Contains(joined, "--reasoning-budget 2048") {
		t.Errorf("missing --reasoning-budget 2048 in %s", joined)
	}
}

func TestBuildServerArgs_ReasoningOmitted(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: "8080", Tools: "all",
		CtxSize: "16384", BatchSize: "512", UbatchSize: "128",
		FlashAttn: "auto", Parallel: "1", Jinja: true}
	args := BuildServerArgs(cfg, ServerPaths{Model: "/m.gguf"})
	for _, a := range args {
		if a == "--reasoning" || a == "--reasoning-budget" {
			t.Errorf("flag %q should not be present by default", a)
		}
	}
}

func TestBuildClaudeEnv_ProxyToken(t *testing.T) {
	cfg := &config.Config{Alias: "local", ProxyToken: "secret"}
	env := BuildClaudeEnv(cfg, "http://proxy", nil)
	m := map[string]string{}
	for _, kv := range env {
		p := strings.SplitN(kv, "=", 2)
		m[p[0]] = p[1]
	}
	if m["ANTHROPIC_AUTH_TOKEN"] != "secret" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want secret", m["ANTHROPIC_AUTH_TOKEN"])
	}
	if m["ANTHROPIC_API_KEY"] != "secret" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want secret", m["ANTHROPIC_API_KEY"])
	}
}

func TestBuildClaudeEnv_NoProxyToken_KeepsDefault(t *testing.T) {
	cfg := &config.Config{Alias: "local"}
	env := BuildClaudeEnv(cfg, "http://proxy", nil)
	m := map[string]string{}
	for _, kv := range env {
		p := strings.SplitN(kv, "=", 2)
		m[p[0]] = p[1]
	}
	if m["ANTHROPIC_AUTH_TOKEN"] != "local-llama" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want local-llama (backward compat)", m["ANTHROPIC_AUTH_TOKEN"])
	}
	if m["ANTHROPIC_API_KEY"] != "local-llama" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want local-llama", m["ANTHROPIC_API_KEY"])
	}
}
func TestBuildClaudeEnv_WithOpenAI(t *testing.T) {
	// A fully configured provider: run() has already resolved a key by this
	// point (hybrid mode is keyed on OpenAIProviderConfigured, which needs URL
	// + model + key-or-keyless), so the test states the key explicitly.
	cfg := &config.Config{
		Alias:         "local",
		OpenAIBaseURL: "https://api.example.com/v1",
		OpenAIModel:   "gpt-4",
		OpenAIKey:     "sk-test",
	}
	env := BuildClaudeEnv(cfg, "http://proxy:8082", nil)
	m := map[string]string{}
	for _, kv := range env {
		p := strings.SplitN(kv, "=", 2)
		m[p[0]] = p[1]
	}
	// Все стандартные алиасы теперь ведут на внешнюю модель.
	if m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "gpt-4" {
		t.Errorf("sonnet = %q, want gpt-4", m["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "gpt-4" {
		t.Errorf("opus = %q, want gpt-4", m["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "gpt-4" {
		t.Errorf("haiku = %q, want gpt-4", m["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	// Кастомная запись несёт алиас локальной модели — прокси узнаёт его и
	// направляет запрос в llama-server.
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "local" {
		t.Errorf("custom option = %q, want local", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
	if m["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "gpt-4" {
		t.Errorf("fable = %q, want gpt-4", m["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
}

func TestContextTokensPerSlot(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want int
	}{
		{"single slot", &config.Config{CtxSize: "262144", Parallel: "1"}, 262144},
		{"two slots", &config.Config{CtxSize: "262144", Parallel: "2"}, 131072},
		{"agent profile (c=73728, parallel=3)", &config.Config{CtxSize: "73728", Parallel: "3"}, 24576},
		{"parallel unset → 1", &config.Config{CtxSize: "147456"}, 147456},
		{"auto → unknown", &config.Config{CtxSize: "auto", Parallel: "1"}, 0},
		{"negative → unknown", &config.Config{CtxSize: "-1", Parallel: "1"}, 0},
		{"garbage parallel → unknown", &config.Config{CtxSize: "262144", Parallel: "two"}, 0},
		{"nil cfg", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContextTokensPerSlot(tt.cfg); got != tt.want {
				t.Errorf("ContextTokensPerSlot(%+v) = %d, want %d", tt.cfg, got, tt.want)
			}
		})
	}
}
