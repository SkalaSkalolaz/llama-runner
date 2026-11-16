package cli

import (
	"bytes"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestParseFlags_Resume(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	cfg, err := ParseFlags([]string{"-resume"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Resume {
		t.Error("Resume should be true with -resume")
	}
}

func TestValidateConfig_Resume(t *testing.T) {
	base := func() *config.Config {
		return &config.Config{Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
			ReadyTimeout: 1e9, Mode: "auto", Task: "t", Resume: true}
	}
	if err := ValidateConfig(base()); err != nil {
		t.Errorf("valid resume rejected: %v", err)
	}
	c := base()
	c.Mode = ""
	if err := ValidateConfig(c); err == nil {
		t.Error("resume without auto mode must fail")
	}
	c = base()
	c.PlanOnly = true
	if err := ValidateConfig(c); err == nil {
		t.Error("resume with -plan-only must fail")
	}
}
