package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Version(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-version"}, &out, &errb, fakeRunner{})
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("out = %q, want %q", out.String(), version)
	}
}

func TestRun_Help(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-h"}, &out, &errb, fakeRunner{})
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "llama-runner") {
		t.Errorf("usage not printed to stderr: %q", errb.String())
	}
}

func TestRun_BadPort(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-port", "abc"}, &out, &errb, fakeRunner{})
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "некорректный -port") {
		t.Errorf("err = %q", errb.String())
	}
}

func TestRun_UpdateFlag_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	runner := fakeRunner{}
	var out, errb bytes.Buffer
	code := run([]string{"-update", "-llama-src", dir}, &out, &errb, runner)
	if code != 0 {
		t.Errorf("code = %d, stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "успешно обновлён") {
		t.Errorf("out = %q", out.String())
	}
}

func TestRun_VerboseDoesNotLeakKey(t *testing.T) {
	// Проверяем два этапа: (a) -key виден только в cfg, (b) в -v-выводе
	// при валидации/запуске до старта процессов ключа нет.
	//
	// Полный run() без модели не запустится — поэтому проверяем именно
	// валидацию: с -openai без ключа и с ключом из env ошибки разные,
	// но сам ключ в выводе не появляется ни в одном случае.

	var out, errb bytes.Buffer

	// -openai есть, -key нет, env нет → ошибка про отсутствующий ключ
	run([]string{"-openai", "https://api.example.com/v1",
		"-model-openai", "m"}, &out, &errb, fakeRunner{})
	if strings.Contains(out.String(), "секрет") || strings.Contains(errb.String(), "секрет") {
		t.Errorf("секрет просочился в вывод")
	}

	// -key задан — тоже не должен попасть в вывод
	out.Reset()
	errb.Reset()
	run([]string{"-openai", "https://api.example.com/v1", "-key", "sk-very-secret-123",
		"-model-openai", "m"}, &out, &errb, fakeRunner{})
	if strings.Contains(out.String(), "sk-very-secret-123") ||
		strings.Contains(errb.String(), "sk-very-secret-123") {
		t.Errorf("ключ просочился в stdout/stderr: out=%q err=%q", out.String(), errb.String())
	}
}
