package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// doctorCfg returns a config that passes doctor's file checks in an empty
// temp home, so a test about the key line is not decided by a missing model.
func doctorCfg(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	model := filepath.Join(home, "model.gguf")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "llama-server")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Model = model
	cfg.ServerBin = bin
	cfg.ServerOnly = true
	cfg.ChatTemplate = ""
	cfg.Mmproj = ""
	cfg.OpenAIBaseURL = "https://oai.example.com/v1"
	cfg.OpenAIModel = "Qwen3.6-27B"
	return cfg, home
}

func TestRunDoctor_KeylessProviderSaysNotRequired(t *testing.T) {
	// "-doctor: ключ не задан" must not be how a deliberate keyless provider
	// reads — it would look like a misconfiguration.
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	cfg, home := doctorCfg(t)
	cfg.OpenAIKeyless = true

	var stdout, stderr bytes.Buffer
	if code := RunDoctor(cfg, home, fakeRunner{}, "9.9.9", &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, want 0\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "не требуется (ключ не передаётся)") {
		t.Errorf("keyless label missing in: %s", out)
	}
}

func TestRunDoctor_RealKeyIsNotReportedAsKeyless(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	t.Setenv("LLAMA_RUNNER_OPENAI_KEY", "")
	cfg, home := doctorCfg(t)
	cfg.OpenAIKey = "sk-secret"

	var stdout, stderr bytes.Buffer
	if code := RunDoctor(cfg, home, fakeRunner{}, "9.9.9", &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, want 0\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "не требуется") {
		t.Errorf("a configured key must not read as keyless: %s", out)
	}
	if !strings.Contains(out, "OpenAI-ключ:    задан") {
		t.Errorf("expected the key to be reported as set: %s", out)
	}
}
