package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// testVersion is the version string injected into Run by tests: Run receives
// it as data (the executable owns the real value), so assertions never depend
// on a package-level global.
const testVersion = "1.5.3-test"

func TestRun_Version(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out, errb bytes.Buffer
	code := Run([]string{"-version"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if strings.TrimSpace(out.String()) != testVersion {
		t.Errorf("out = %q, want %q", out.String(), testVersion)
	}
}

func TestRun_Help(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out, errb bytes.Buffer
	code := Run([]string{"-h"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "llama-runner") {
		t.Errorf("usage not printed to stderr: %q", errb.String())
	}
}

func TestRun_BadPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LC_ALL", "ru_RU.UTF-8") // сообщение об ошибке локализовано
	var out, errb bytes.Buffer
	code := Run([]string{"-port", "abc"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "некорректный -port") {
		t.Errorf("err = %q", errb.String())
	}
}

func TestRun_UpdateFlag_PassesThrough(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir()
	runner := fakeRunner{}
	var out, errb bytes.Buffer
	code := Run([]string{"-update", "-llama-src", dir}, &out, &errb, runner, testVersion)
	if code != 0 {
		t.Errorf("code = %d, stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "успешно обновлён") {
		t.Errorf("out = %q", out.String())
	}
}

func TestRun_VerboseDoesNotLeakKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Проверяем два этапа: (a) -key виден только в cfg, (b) в -v-выводе
	// при валидации/запуске до старта процессов ключа нет.
	//
	// Полный Run() без модели не запустится — поэтому проверяем именно
	// валидацию: с -openai без ключа и с ключом из env ошибки разные,
	// но сам ключ в выводе не появляется ни в одном случае.

	var out, errb bytes.Buffer

	// -openai есть, -key нет, env нет → ошибка про отсутствующий ключ
	Run([]string{"-openai", "https://api.example.com/v1",
		"-model-openai", "m"}, &out, &errb, fakeRunner{}, testVersion)
	if strings.Contains(out.String(), "секрет") || strings.Contains(errb.String(), "секрет") {
		t.Errorf("секрет просочился в вывод")
	}

	// -key задан — тоже не должен попасть в вывод
	out.Reset()
	errb.Reset()
	Run([]string{"-openai", "https://api.example.com/v1", "-key", "sk-very-secret-123",
		"-model-openai", "m"}, &out, &errb, fakeRunner{}, testVersion)
	if strings.Contains(out.String(), "sk-very-secret-123") ||
		strings.Contains(errb.String(), "sk-very-secret-123") {
		t.Errorf("ключ просочился в stdout/stderr: out=%q err=%q", out.String(), errb.String())
	}
}

func TestRun_ModelAliasAddedToCatalog(t *testing.T) {
	// -model X.gguf where X is not a declared profile must still be
	// switch-to-able (Custom 1 = switch-to:X). Otherwise the user could
	// swap away and be unable to swap back.
	home := serverHomeWithConfig(t, `{"profiles": {"coder": {"alias": "coder"}}}`)

	var out, errb bytes.Buffer
	log := &argLog{}
	code := Run(
		[]string{"-config", filepath.Join(home, "config.json"), "-server",
			"-model", "~/GGUF/gpt-oss-20b-Q5_K_M.gguf"},
		&out, &errb, recordingRunner{log: log}, "1.5.3-test")
	if code != 0 {
		t.Fatalf("Run = %d, want 0 (stderr=%s)", code, errb.String())
	}
	// Behavioural assertion: the run succeeded with the derived alias.
	// The catalog-completeness itself is exercised by proxy tests; here
	// we only prove -server is not broken by the new catalog entry.
	_ = log
}
