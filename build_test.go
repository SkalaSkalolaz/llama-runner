package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildServerArgs_Minimal(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: "8080", Tools: "all",
		CtxSize: "16384", BatchSize: "512", UbatchSize: "128",
		FlashAttn: "auto", Parallel: "1", Jinja: true}
	args := buildServerArgs(cfg, ServerPaths{Model: "/m.gguf"})

	want := []string{"-m", "/m.gguf", "--host", "127.0.0.1", "--port", "8080",
		"--tools", "all", "-c", "16384", "-b", "512", "-ub", "128",
		"-fa", "auto", "--parallel", "1", "--jinja"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v\nwant %v", args, want)
	}
}

func TestBuildServerArgs_FullOptional(t *testing.T) {
	cfg := &Config{Host: "0.0.0.0", Port: "9000", Tools: "read",
		CtxSize: "1", BatchSize: "2", UbatchSize: "3", FlashAttn: "on",
		Parallel: "4", Jinja: false, McpProxy: true, LazyMmap: true,
		Alias: "local", NGL: "999", Temp: "0.7", TopP: "0.9",
		CacheTypeK: "q8_0", CacheTypeV: "q8_0", FitMode: "off", LogVerbosity: "4",
		SpecType: "draft-mtp", SpecDraftNMax: "2", SpecDraftNGL: "0"}
	paths := ServerPaths{Model: "/m", Mmproj: "/mm", Draft: "/d", Template: "/t"}
	args := buildServerArgs(cfg, paths)

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
	cfg := &Config{Alias: "local"}
	env := buildClaudeEnv(cfg, "http://127.0.0.1:8080", []string{"PATH=/usr/bin"})
	m := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		m[parts[0]] = parts[1]
	}
	checks := map[string]string{
		"PATH":                            "/usr/bin",
		"ANTHROPIC_BASE_URL":              "http://127.0.0.1:8080",
		"ANTHROPIC_AUTH_TOKEN":            "local-llama",
		"ANTHROPIC_API_KEY":               "local-llama",
		"ANTHROPIC_DEFAULT_SONNET_MODEL":  "local",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":   "local",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":    "local",
	}
	for k, v := range checks {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
}

func TestBuildClaudeEnv_NoAlias(t *testing.T) {
	cfg := &Config{}
	env := buildClaudeEnv(cfg, "http://h:1", nil)
	for _, kv := range env {
		if strings.HasPrefix(kv, "ANTHROPIC_DEFAULT_") {
			t.Errorf("unexpected %q", kv)
		}
	}
}

func TestBuildClaudeArgs_Empty(t *testing.T) {
	args := buildClaudeArgs(&Config{}, "")
	if len(args) != 0 {
		t.Errorf("got %v, want empty", args)
	}
}

func TestBuildClaudeArgs_WithMCP(t *testing.T) {
	args := buildClaudeArgs(&Config{}, "/tmp/mcp.json")
	want := []string{"--mcp-config", "/tmp/mcp.json"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildClaudeArgs_Strict(t *testing.T) {
	args := buildClaudeArgs(&Config{McpStrict: true}, "/tmp/mcp.json")
	want := []string{"--mcp-config", "/tmp/mcp.json", "--strict-mcp-config"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("got %v, want %v", args, want)
	}
}

func TestBuildServerArgs_Reasoning(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: "8080", Tools: "all",
		CtxSize: "16384", BatchSize: "512", UbatchSize: "128",
		FlashAttn: "auto", Parallel: "1", Jinja: true,
		Reasoning: "on", ReasoningBudget: "2048"}
	args := buildServerArgs(cfg, ServerPaths{Model: "/m.gguf"})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--reasoning on") {
		t.Errorf("missing --reasoning on in %s", joined)
	}
	if !strings.Contains(joined, "--reasoning-budget 2048") {
		t.Errorf("missing --reasoning-budget 2048 in %s", joined)
	}
}

func TestBuildServerArgs_ReasoningOmitted(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: "8080", Tools: "all",
		CtxSize: "16384", BatchSize: "512", UbatchSize: "128",
		FlashAttn: "auto", Parallel: "1", Jinja: true}
	args := buildServerArgs(cfg, ServerPaths{Model: "/m.gguf"})
	for _, a := range args {
		if a == "--reasoning" || a == "--reasoning-budget" {
			t.Errorf("flag %q should not be present by default", a)
		}
	}
}