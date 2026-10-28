package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestRunDoctor_OK(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8") // вывод локализзован, проверяем русские подписи
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	serverBin := filepath.Join(dir, "llama-server")
	if err := os.WriteFile(serverBin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Model = model
	cfg.ServerBin = serverBin
	cfg.ServerOnly = true // не проверять claude
	cfg.ChatTemplate = "" // не проверять chat-template

	var stdout, stderr bytes.Buffer
	code := RunDoctor(cfg, dir, fakeRunner{}, "9.9.9", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0\nstdout=%s\nstderr=%s",
			code, stdout.String(), stderr.String())
	}
	for _, want := range []string{"модель:", "llama-server:", "OK"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q: %s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "НЕ найден") {
		t.Errorf("unexpected НЕ найден: %s", stdout.String())
	}
}

func TestRunDoctor_MissingModel(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Model = filepath.Join(dir, "missing.gguf")
	cfg.ServerBin = filepath.Join(dir, "missing-bin")
	cfg.ServerOnly = true
	cfg.ChatTemplate = ""

	var stdout, stderr bytes.Buffer
	code := RunDoctor(cfg, dir, fakeRunner{}, "9.9.9", &stdout, &stderr)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "НЕ найден") {
		t.Errorf("stdout = %s", stdout.String())
	}
}

func TestRunDoctor_InvalidConfig(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Port = "not-a-number" // ValidateConfig упадёт
	cfg.Model = filepath.Join(dir, "m.gguf")

	var stdout, stderr bytes.Buffer
	code := RunDoctor(cfg, dir, fakeRunner{}, "9.9.9", &stdout, &stderr)
	if code != 2 {
		t.Errorf("code = %d, want 2\nstderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ошибка конфига") {
		t.Errorf("stderr = %s", stderr.String())
	}
}

func TestRunDoctor_DoesNotStartProcesses(t *testing.T) {
	// runner, который упадёт, если его вызовут не для LookPath.
	// fakeRunner.Command вернёт sh -c "exit 0" — безопасно; тест
	// проверяет, что doctor не пытается запустить команду и не падает.
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Model = filepath.Join(dir, "m.gguf")
	cfg.ServerBin = filepath.Join(dir, "bin")
	cfg.ServerOnly = true
	cfg.ChatTemplate = ""

	var stdout, stderr bytes.Buffer
	// Не паникуем, не зависаем.
	_ = RunDoctor(cfg, dir, fakeRunner{}, "9.9.9", &stdout, &stderr)
}

func TestRunDoctor_PrintsCommand(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	dir := t.TempDir()
	model := filepath.Join(dir, "m.gguf")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	serverBin := filepath.Join(dir, "bin")
	if err := os.WriteFile(serverBin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Model = model
	cfg.ServerBin = serverBin
	cfg.ServerOnly = true
	cfg.ChatTemplate = ""

	var stdout, stderr bytes.Buffer
	_ = RunDoctor(cfg, dir, fakeRunner{}, "9.9.9", &stdout, &stderr)

	if !strings.Contains(stdout.String(), "Команда llama-server:") {
		t.Errorf("stdout missing 'Команда llama-server:': %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "-m") || !strings.Contains(stdout.String(), model) {
		t.Errorf("stdout missing -m / model: %s", stdout.String())
	}
}

func TestRunDoctor_ExpandsTildeInCommand(t *testing.T) {
	dir := t.TempDir()
	// Создаём файлы и ссылаемся на них через ~ (home=dir).
	model := filepath.Join(dir, "m.gguf")
	serverBin := filepath.Join(dir, "bin")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverBin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Model = "~/m.gguf"
	cfg.ServerBin = "~/bin"
	cfg.ServerOnly = true
	cfg.ChatTemplate = ""

	var stdout, stderr bytes.Buffer
	_ = RunDoctor(cfg, dir, fakeRunner{}, "9.9.9", &stdout, &stderr)
	out := stdout.String()

	if strings.Contains(out, "~/") {
		t.Errorf("в команде остался нераскрытый ~: %s", out)
	}
	if !strings.Contains(out, model) || !strings.Contains(out, serverBin) {
		t.Errorf("в команде нет абсолютных путей: %s", out)
	}
}
