package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOpenAIKey_FlagWins(t *testing.T) {
	var stderr bytes.Buffer
	got, err := ResolveOpenAIKey("from-flag", func(string) string {
		return "from-env"
	}, t.TempDir(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-flag" {
		t.Errorf("got %q, want from-flag", got)
	}
}

func TestResolveOpenAIKey_EnvPrecedence(t *testing.T) {
	var stderr bytes.Buffer
	env := func(k string) string {
		switch k {
		case "LLAMA_RUNNER_OPENAI_KEY":
			return "runner-key"
		case "OPENAI_API_KEY":
			return "generic-key"
		}
		return ""
	}
	got, _ := ResolveOpenAIKey("", env, t.TempDir(), &stderr)
	if got != "runner-key" {
		t.Errorf("got %q, want runner-key (LLAMA_RUNNER_OPENAI_KEY должен быть выше OPENAI_API_KEY)", got)
	}
}

func TestResolveOpenAIKey_GenericEnv(t *testing.T) {
	var stderr bytes.Buffer
	env := func(k string) string {
		if k == "OPENAI_API_KEY" {
			return "generic-key"
		}
		return ""
	}
	got, _ := ResolveOpenAIKey("", env, t.TempDir(), &stderr)
	if got != "generic-key" {
		t.Errorf("got %q", got)
	}
}

func TestResolveOpenAIKey_FromFile(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "llama-runner")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials")
	content := "# comment\nOPENAI_KEY=\"file-key\"\nother=ignored\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	got, err := ResolveOpenAIKey("", func(string) string { return "" }, home, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-key" {
		t.Errorf("got %q, want file-key; stderr=%q", got, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected warning: %q", stderr.String())
	}
}

func TestResolveOpenAIKey_FileWarnsAboutPerms(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "llama-runner")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials")
	if err := os.WriteFile(path, []byte("OPENAI_KEY=k\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	got, _ := ResolveOpenAIKey("", func(string) string { return "" }, home, &stderr)
	if got != "k" {
		t.Errorf("got %q, want k", got)
	}
	if !strings.Contains(stderr.String(), "0600") {
		t.Errorf("ожидалось предупреждение про 0600, stderr=%q", stderr.String())
	}
}

func TestResolveOpenAIKey_NoSources(t *testing.T) {
	var stderr bytes.Buffer
	got, err := ResolveOpenAIKey("", func(string) string { return "" }, t.TempDir(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
