package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want string
		ok   bool
	}{
		{"none", []string{}, "config", "", false},
		{"-config value", []string{"-config", "/x"}, "config", "/x", true},
		{"--config value", []string{"--config", "/x"}, "config", "/x", true},
		{"-config=value", []string{"-config=/x"}, "config", "/x", true},
		{"--config=value", []string{"--config=/x"}, "config", "/x", true},
		{"-config empty", []string{"-config="}, "config", "", true},
		{"-config at end", []string{"-config"}, "config", "", true},
		{"-other then -config", []string{"-other", "v", "-config", "/x"}, "config", "/x", true},
		{"-- terminator", []string{"--", "-config", "/x"}, "config", "", false},
		{"positional stops", []string{"pos", "-config", "/x"}, "config", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findFlag(tt.args, tt.flag)
			if ok != tt.ok || got != tt.want {
				t.Errorf("findFlag(%v, %q) = (%q, %v), want (%q, %v)",
					tt.args, tt.flag, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLoadConfigFile_NotExist(t *testing.T) {
	fc, err := loadConfigFile(filepath.Join(t.TempDir(), "missing.json"), t.TempDir())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if fc != nil {
		t.Errorf("fc = %v, want nil", fc)
	}
}

func TestLoadConfigFile_EmptyPath(t *testing.T) {
	fc, err := loadConfigFile("", t.TempDir())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if fc != nil {
		t.Errorf("fc = %v, want nil", fc)
	}
}

func TestLoadConfigFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"host": "0.0.0.0", "port": "9090", "jinja": false}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := loadConfigFile(path, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if fc == nil {
		t.Fatal("fc = nil")
	}
	if fc.Host == nil || *fc.Host != "0.0.0.0" {
		t.Errorf("Host = %v", fc.Host)
	}
	if fc.Port == nil || *fc.Port != "9090" {
		t.Errorf("Port = %v", fc.Port)
	}
	if fc.Jinja == nil || *fc.Jinja != false {
		t.Errorf("Jinja = %v", fc.Jinja)
	}
}

func TestLoadConfigFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFile(path, dir); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFlags_ConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"host": "0.0.0.0", "port": "9090", "readyTimeout": "5m", "jinja": false}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := parseFlagsWithHome([]string{"-config", cfgPath}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.ReadyTimeout != 5*time.Minute {
		t.Errorf("ReadyTimeout = %v, want 5m", cfg.ReadyTimeout)
	}
	if cfg.Jinja {
		t.Error("Jinja = true, want false (из конфига)")
	}
	// Встроенные дефолты для незаданных полей сохраняются.
	if cfg.ServerBin != "~/.local/bin/llama-server" {
		t.Errorf("ServerBin = %q, want встроенный дефолт", cfg.ServerBin)
	}
}

func TestParseFlags_CLIOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"host": "0.0.0.0", "port": "9090"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := parseFlagsWithHome(
		[]string{"-config", cfgPath, "-host", "example.com"}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Host != "example.com" {
		t.Errorf("Host = %q, want example.com (CLI > файл)", cfg.Host)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090 (из файла)", cfg.Port)
	}
}

func TestParseFlags_Profile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"host": "127.0.0.1",
		"port": "8080",
		"profiles": {
			"coder": {"host": "0.0.0.0", "port": "9090", "alias": "coder"}
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := parseFlagsWithHome(
		[]string{"-config", cfgPath, "-p", "coder"}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q (профиль должен переопределить корень)", cfg.Host)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.Alias != "coder" {
		t.Errorf("Alias = %q", cfg.Alias)
	}
	if cfg.Profile != "coder" {
		t.Errorf("cfg.Profile = %q", cfg.Profile)
	}
}

func TestParseFlags_ProfileNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := parseFlagsWithHome(
		[]string{"-config", cfgPath, "-p", "missing"}, &buf, dir); err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestParseFlags_EmptyConfigDisables(t *testing.T) {
	dir := t.TempDir()
	// Создаём дефолтный конфиг с нестандартным host.
	if err := os.MkdirAll(filepath.Join(dir, ".config", "llama-runner"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, ".config", "llama-runner", "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"host": "0.0.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := parseFlagsWithHome([]string{"-config", ""}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1 (-config \"\" отключает файл)", cfg.Host)
	}
}

func TestParseFlags_DefaultConfigPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".config", "llama-runner"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, ".config", "llama-runner", "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"host": "0.0.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := parseFlagsWithHome([]string{}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0 (дефолтный путь конфига)", cfg.Host)
	}
}

func TestParseFlags_ConfigWithProfilesAndDefaults(t *testing.T) {
	// Профиль, у которого задано только одно поле: остальные берутся из корня.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"host": "127.0.0.1",
		"port": "8080",
		"alias": "root",
		"profiles": {
			"fast": {"port": "7070"}
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := parseFlagsWithHome(
		[]string{"-config", cfgPath, "-p", "fast"}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Port != "7070" {
		t.Errorf("Port = %q, want 7070 (из профиля)", cfg.Port)
	}
	if cfg.Alias != "root" {
		t.Errorf("Alias = %q, want root (из корня)", cfg.Alias)
	}
}

func TestParseFlags_BadReadyTimeoutInConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"readyTimeout": "not-a-duration"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := parseFlagsWithHome([]string{"-config", cfgPath}, &buf, dir); err == nil {
		t.Fatal("expected error for bad duration")
	} else if !strings.Contains(err.Error(), "readyTimeout") {
		t.Errorf("err = %v, want упоминание readyTimeout", err)
	}
}