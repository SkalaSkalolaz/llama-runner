package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// --- model naming provenance (see cli.ResolveAlias) ---

func TestParseFlags_CLIModelOverridesProfileAlias(t *testing.T) {
	// -p coder -model Y: the profile brings both a model and a name, but the
	// model chosen on the command line is the deliberate choice, so the visible
	// name follows it.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"profiles": {"coder": {` +
		`"model": "~/GGUF/Qwen3.8-Flash.gguf", "alias": "qwen3.8-coder"}}}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome(
		[]string{"-config", cfgPath, "-p", "coder",
			"-model", "~/GGUF/gpt-oss-20b-Q5_K_M.gguf"}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !cfg.ModelSet {
		t.Error("ModelSet = false, want true (-model given on the CLI)")
	}
	if cfg.Model != "~/GGUF/gpt-oss-20b-Q5_K_M.gguf" {
		t.Errorf("Model = %q, want the CLI value (CLI > profile)", cfg.Model)
	}
	alias, src := ResolveAlias(cfg)
	if alias != "gpt-oss-20b-Q5_K_M" {
		t.Errorf("effective alias = %q, want gpt-oss-20b-Q5_K_M", alias)
	}
	if src != i18n.MsgAliasSrcDerived {
		t.Errorf("origin = %q, want the derived-from-model label", src)
	}
}

func TestRegression_CLIModelBeatsStaleFileAlias(t *testing.T) {
	// The reported bug. config.json pins model=Qwen3.8-35B-A3B-Q4_K_M and
	// alias=qwen3.8-35b; the user picks gpt-oss on the command line. The GGUF
	// really was loaded, yet llama-server's --alias (hence its WebUI card)
	// and ANTHROPIC_DEFAULT_*_MODEL (hence Claude Code's /model) still named
	// the Qwen model. Both names must now describe the chosen file.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf", "alias": "qwen3.8-35b"}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome(
		[]string{"-config", cfgPath, "-model", "~/GGUF/gpt-oss-20b-Q5_K_M.gguf"},
		&buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Alias != "qwen3.8-35b" || !cfg.AliasSet {
		t.Errorf("parse layer: Alias = %q, AliasSet = %v; want qwen3.8-35b, true",
			cfg.Alias, cfg.AliasSet)
	}

	// What run() does before building the command line and the environment.
	cfg.Alias, _ = ResolveAlias(cfg)

	args := BuildServerArgs(cfg, ServerPaths{Model: "/home/u/GGUF/gpt-oss-20b-Q5_K_M.gguf"})
	if !strings.Contains(strings.Join(args, " "), "--alias gpt-oss-20b-Q5_K_M") {
		t.Errorf("llama-server args miss --alias gpt-oss-20b-Q5_K_M: %s",
			strings.Join(args, " "))
	}
	env := strings.Join(BuildClaudeEnv(cfg, "http://127.0.0.1:8080",
		[]string{"PATH=/usr/bin"}), "\n")
	for _, key := range []string{"SONNET", "HAIKU", "OPUS"} {
		want := "ANTHROPIC_DEFAULT_" + key + "_MODEL=gpt-oss-20b-Q5_K_M"
		if !strings.Contains(env, want) {
			t.Errorf("claude environment misses %q: %s", want, env)
		}
	}
	if strings.Contains(env, "qwen3.8-35b") {
		t.Errorf("stale alias qwen3.8-35b still published: %s", env)
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
	cfg, err := ParseFlagsWithHome([]string{"-config", cfgPath}, &buf, dir)
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
	cfg, err := ParseFlagsWithHome(
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
	cfg, err := ParseFlagsWithHome(
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
	if _, err := ParseFlagsWithHome(
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
	cfg, err := ParseFlagsWithHome([]string{"-config", ""}, &buf, dir)
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
	cfg, err := ParseFlagsWithHome([]string{}, &buf, dir)
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
	cfg, err := ParseFlagsWithHome(
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
	if _, err := ParseFlagsWithHome([]string{"-config", cfgPath}, &buf, dir); err == nil {
		t.Fatal("expected error for bad duration")
	} else if !strings.Contains(err.Error(), "readyTimeout") {
		t.Errorf("err = %v, want упоминание readyTimeout", err)
	}
}

func TestParseFlags_ProfileAliasNotClobbered(t *testing.T) {
	// Профиль задаёт alias — AliasSet должен стать true, иначе run()
	// заменит alias на "local".
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"profiles": {"coder": {"alias": "qwen3-coder"}}}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome(
		[]string{"-config", cfgPath, "-p", "coder"}, &buf, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.Alias != "qwen3-coder" {
		t.Errorf("Alias = %q, want qwen3-coder", cfg.Alias)
	}
	if !cfg.AliasSet {
		t.Error("AliasSet = false — run() заменит alias на local")
	}
}

func TestParseFlags_FileAliasNotClobbered(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"alias": "my-model"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome([]string{"-config", cfgPath}, &buf, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alias != "my-model" || !cfg.AliasSet {
		t.Errorf("Alias = %q, AliasSet = %v; want my-model, true", cfg.Alias, cfg.AliasSet)
	}
}

func TestParseFlags_ProfileChatTemplateSet(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"profiles": {"coder": {"chatTemplate": "/custom/t.jinja"}}}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg, err := ParseFlagsWithHome(
		[]string{"-config", cfgPath, "-p", "coder"}, &buf, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChatTemplate != "/custom/t.jinja" {
		t.Errorf("ChatTemplate = %q", cfg.ChatTemplate)
	}
	if !cfg.ChatTemplateSet {
		t.Error("ChatTemplateSet = false — ValidateTemplatePath будет молчать")
	}
}

func TestParseFlags_ExplicitConfigMissing(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8") // текст ошибки локализован
	dir := t.TempDir()
	var buf bytes.Buffer
	if _, err := ParseFlagsWithHome(
		[]string{"-config", filepath.Join(dir, "missing.json")}, &buf, dir); err == nil {
		t.Fatal("expected error for explicitly missing config")
	} else if !strings.Contains(err.Error(), "не найден") {
		t.Errorf("err = %v, want упоминание «не найден»", err)
	}
}

func TestParseFlags_ExplicitConfigMissingWithProfile(t *testing.T) {
	// Ошибка должна быть про конфиг, не про профиль: сначала файл,
	// потом профиль.
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir()
	var buf bytes.Buffer
	if _, err := ParseFlagsWithHome(
		[]string{"-config", filepath.Join(dir, "missing.json"), "-p", "coder"}, &buf, dir); err == nil {
		t.Fatal("expected error for missing explicit config")
	} else if !strings.Contains(err.Error(), "конфиг") {
		t.Errorf("err = %v, want упоминание конфига", err)
	}
}

func TestParseFlags_ProfileWithoutConfigFile(t *testing.T) {
	// -config "" + -p coder: конфиг отключён, профиль неоткуда взять.
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir()
	var buf bytes.Buffer
	if _, err := ParseFlagsWithHome(
		[]string{"-config", "", "-p", "coder"}, &buf, dir); err == nil {
		t.Fatal("expected error for profile without config")
	} else if !strings.Contains(err.Error(), "профиль") {
		t.Errorf("err = %v, want упоминание профиля", err)
	}
}

func TestParseFlags_ProfileWithDefaultConfigMissing(t *testing.T) {
	// Дефолтный конфиг не существует, но пользователь просит профиль:
	// это не ошибка про пути, а ошибка про «нечего применять».
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir() // без .config/llama-runner/config.json
	var buf bytes.Buffer
	if _, err := ParseFlagsWithHome(
		[]string{"-p", "coder"}, &buf, dir); err == nil {
		t.Fatal("expected error for profile when default config missing")
	} else if !strings.Contains(err.Error(), "профиль") {
		t.Errorf("err = %v, want упоминание профиля", err)
	}
}
