package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestPrintClaudeDiagnostics_ShowsEnvironmentModels(t *testing.T) {
	out := new(bytes.Buffer)
	cfg := &config.Config{ProxyToken: ""}
	env := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_DEFAULT_SONNET_MODEL=external-model",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=external-model",
		"ANTHROPIC_CUSTOM_MODEL_OPTION=local-alias",
	}

	PrintClaudeDiagnostics(out, cfg, "/home/u/bin/claude", "/home/u/project", "http://127.0.0.1:8082", env)

	text := out.String()
	for _, want := range []string{
		"/home/u/bin/claude",
		"/home/u/project",
		"http://127.0.0.1:8082",
		"ANTHROPIC_DEFAULT_SONNET_MODEL=external-model",
		"ANTHROPIC_CUSTOM_MODEL_OPTION=local-alias",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "ANTHROPIC_DEFAULT_OPUS_MODEL") {
		t.Errorf("absent variable echoed anyway:\n%s", text)
	}
}

func TestPrintClaudeDiagnostics_HidesProxyToken(t *testing.T) {
	out := new(bytes.Buffer)
	cfg := &config.Config{ProxyToken: "super-secret-token"}
	env := []string{"ANTHROPIC_AUTH_TOKEN=super-secret-token"}

	PrintClaudeDiagnostics(out, cfg, "/bin/claude", "/tmp", "http://127.0.0.1:8082", env)

	text := out.String()
	if strings.Contains(text, "super-secret-token") {
		t.Errorf("proxy token leaked into -v output:\n%s", text)
	}
	if text == "" {
		t.Error("no diagnostics printed at all")
	}
}
