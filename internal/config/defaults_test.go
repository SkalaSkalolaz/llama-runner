package config

import (
	"testing"
	"time"
)

// TestDefaultConfig pins the built-in defaults: they are the bottom layer of
// the precedence chain and must keep matching the historical hardcoded values.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Host != "127.0.0.1" || cfg.Port != "8080" {
		t.Errorf("host:port = %s:%s", cfg.Host, cfg.Port)
	}
	if cfg.CtxSize != "65536" || cfg.BatchSize != "2048" || cfg.UbatchSize != "1024" {
		t.Errorf("sizes = %s/%s/%s", cfg.CtxSize, cfg.BatchSize, cfg.UbatchSize)
	}
	if !cfg.Jinja {
		t.Error("Jinja = false, want true")
	}
	if cfg.ChatTemplate != "auto" || cfg.ChatTemplateFallback == "" {
		t.Errorf("chatTemplate = %q, fallback = %q", cfg.ChatTemplate, cfg.ChatTemplateFallback)
	}
	if cfg.ReadyTimeout != 180*time.Second {
		t.Errorf("ReadyTimeout = %v, want 180s", cfg.ReadyTimeout)
	}
	if cfg.LogMaxMB != 50 || cfg.LogKeep != 5 {
		t.Errorf("log rotation = %d/%d, want 50/5", cfg.LogMaxMB, cfg.LogKeep)
	}
	// Paths keep "~"; expansion happens later, at use time.
	if cfg.Model[0] != '~' || cfg.ServerBin[0] != '~' {
		t.Errorf("Model = %q, ServerBin = %q: want unexpanded ~ paths", cfg.Model, cfg.ServerBin)
	}
}

// TestApplyFileConfig_OverlaysOnlySetFields checks the precedence mechanism:
// non-nil FileConfig fields overwrite cfg, nil fields leave it untouched.
func TestApplyFileConfig_OverlaysOnlySetFields(t *testing.T) {
	cfg := DefaultConfig()
	fc := FileConfig{
		Host:     str("0.0.0.0"),
		Jinja:    bl(false),
		LogMaxMB: i(7),
	}
	if err := ApplyFileConfig(cfg, fc); err != nil {
		t.Fatalf("ApplyFileConfig: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Jinja {
		t.Error("Jinja = true, want false")
	}
	if cfg.LogMaxMB != 7 {
		t.Errorf("LogMaxMB = %d, want 7", cfg.LogMaxMB)
	}
	if cfg.Port != "8080" || cfg.Model != "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf" {
		t.Errorf("nil fields were clobbered: port=%q model=%q", cfg.Port, cfg.Model)
	}
}

// TestApplyFileConfig_BadReadyTimeout: a malformed duration must be an error,
// never a silently kept previous value.
func TestApplyFileConfig_BadReadyTimeout(t *testing.T) {
	cfg := DefaultConfig()
	before := cfg.ReadyTimeout
	if err := ApplyFileConfig(cfg, FileConfig{ReadyTimeout: str("not-a-duration")}); err == nil {
		t.Fatal("expected error for malformed readyTimeout")
	}
	if cfg.ReadyTimeout != before {
		t.Errorf("ReadyTimeout changed to %v", cfg.ReadyTimeout)
	}
	if err := ApplyFileConfig(cfg, FileConfig{ReadyTimeout: str("90s")}); err != nil {
		t.Fatalf("ApplyFileConfig: %v", err)
	}
	if cfg.ReadyTimeout != 90*time.Second {
		t.Errorf("ReadyTimeout = %v, want 90s", cfg.ReadyTimeout)
	}
}

func str(s string) *string { return &s }
func bl(b bool) *bool      { return &b }
func i(n int) *int         { return &n }
