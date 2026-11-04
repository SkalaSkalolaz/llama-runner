package cli

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

func TestParseFlags_Defaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	cfg, err := ParseFlags([]string{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q", cfg.Host)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if !cfg.Jinja {
		t.Error("Jinja should default to true")
	}
	if cfg.ChatTemplate != "auto" {
		t.Errorf("ChatTemplate = %q", cfg.ChatTemplate)
	}
	if cfg.ChatTemplateSet {
		t.Error("ChatTemplateSet should be false")
	}
	if cfg.ProxyAddr != "127.0.0.1:8082" {
		t.Errorf("ProxyAddr = %q, want 127.0.0.1:8082", cfg.ProxyAddr)
	}
}

func TestParseFlags_ExplicitChatTemplate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	cfg, err := ParseFlags([]string{"-chat-template", "/tmp/t.jinja"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ChatTemplateSet {
		t.Error("ChatTemplateSet should be true")
	}
	if cfg.ChatTemplate != "/tmp/t.jinja" {
		t.Errorf("ChatTemplate = %q", cfg.ChatTemplate)
	}
}

func TestParseFlags_Help(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	_, err := ParseFlags([]string{"-h"}, &buf)
	if !errors.Is(err, flag.ErrHelp) {
		t.Errorf("err = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(buf.String(), "llama-runner") {
		t.Errorf("usage not printed: %q", buf.String())
	}
}

func TestParseFlags_UnknownFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	_, err := ParseFlags([]string{"-nosuchflag"}, &buf)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestValidateConfig_Table(t *testing.T) {
	base := func() *config.Config {
		return &config.Config{Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
			ReadyTimeout: time.Second, McpProxy: false}
	}
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"valid", func(c *config.Config) {}, false},
		{"empty host", func(c *config.Config) { c.Host = "" }, true},
		{"bad port", func(c *config.Config) { c.Port = "abc" }, true},
		{"port 0", func(c *config.Config) { c.Port = "0" }, true},
		{"port 70000", func(c *config.Config) { c.Port = "70000" }, true},
		{"zero timeout", func(c *config.Config) { c.ReadyTimeout = 0 }, true},
		{"bad reasoning", func(c *config.Config) { c.Reasoning = "maybe" }, true},
		{"good reasoning on", func(c *config.Config) { c.Reasoning = "on" }, false},
		{"bad reasoning-budget", func(c *config.Config) { c.ReasoningBudget = "-5" }, true},
		{"good reasoning-budget -1", func(c *config.Config) { c.ReasoningBudget = "-1" }, false},
		{"good reasoning-budget 0", func(c *config.Config) { c.ReasoningBudget = "0" }, false},
		{"mcp proxy bad addr", func(c *config.Config) { c.McpProxy = true; c.McpAddr = "noseparator" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			tt.mutate(c)
			err := ValidateConfig(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveChatTemplate(t *testing.T) {
	fallback := "~/GGUF/qwen-fixed-chat-template.jinja"
	tests := []struct {
		name  string
		raw   string
		model string
		fb    string
		want  string
	}{
		{"qwen lowercase", "auto", "/m/qwen3.gguf", fallback, fallback},
		{"Qwen mixed case", "auto", "/m/Qwen3.8-Flash.gguf", fallback, fallback},
		{"non-qwen", "auto", "/m/llama3.gguf", fallback, ""},
		{"explicit path", "/custom/t.jinja", "/m/qwen.gguf", fallback, "/custom/t.jinja"},
		{"explicit empty", "", "/m/qwen.gguf", fallback, ""},
		{"custom fallback", "auto", "/m/qwen3.gguf", "/custom/fallback.jinja", "/custom/fallback.jinja"},
		{"empty fallback", "auto", "/m/qwen3.gguf", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveChatTemplate(tt.raw, tt.model, tt.fb); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- naming provenance flags ---

func TestParseFlags_CLIModelSetsModelSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	cfg, err := ParseFlags([]string{"-model", "/m/gpt-oss-20b-Q5_K_M.gguf"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "/m/gpt-oss-20b-Q5_K_M.gguf" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if !cfg.ModelSet {
		t.Error("ModelSet = false — ResolveAlias would keep a stale file alias")
	}
	if cfg.AliasFromCLI {
		t.Error("AliasFromCLI = true, want false (-model is not -alias)")
	}
}

func TestParseFlags_CLIAliasSetsAliasFromCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	cfg, err := ParseFlags([]string{"-alias", "Z"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alias != "Z" {
		t.Errorf("Alias = %q, want Z", cfg.Alias)
	}
	if !cfg.AliasFromCLI {
		t.Error("AliasFromCLI = false — a CLI -alias must outrank naming derived from -model")
	}
	if !cfg.AliasSet {
		t.Error("AliasSet = false, want true (backward-compatible meaning)")
	}
	if cfg.ModelSet {
		t.Error("ModelSet = true, want false (-alias is not -model)")
	}
}

func TestParseFlags_ProfileModelDoesNotSetModelSet(t *testing.T) {
	// A profile is self-contained: it supplies its own model AND alias, so
	// ModelSet must stay false — otherwise ResolveAlias rule 2 would re-derive
	// the name from the path and throw the profile's alias away.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"profiles": {"coder": {` +
		`"model": "/m/Qwen3.8-Flash-Next-GSQ-RCO-IQ1_M-00001-of-00002.gguf",` +
		`"alias": "qwen3.8-coder"}}}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome([]string{"-config", cfgPath, "-p", "coder"}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.ModelSet {
		t.Error("ModelSet = true — a profile's model must not trigger name derivation")
	}
	if !cfg.AliasSet || !cfg.AliasFromProfile {
		t.Errorf("AliasSet = %v, AliasFromProfile = %v; want both true",
			cfg.AliasSet, cfg.AliasFromProfile)
	}
	if alias, src := ResolveAlias(cfg); alias != "qwen3.8-coder" || src != i18n.MsgAliasSrcProfile {
		t.Errorf("ResolveAlias = (%q, %q), want (qwen3.8-coder, profile)", alias, src)
	}
}
