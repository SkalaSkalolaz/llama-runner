package cli

import (
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestValidateConfig_OpenAIMissingKey(t *testing.T) {
	cfg := &config.Config{
		Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
		ReadyTimeout:  1e9,
		OpenAIBaseURL: "https://api.example.com/v1",
		// OpenAIModel задана, но нет ключа
		OpenAIModel: "gpt-4",
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error about missing -key")
	}
}

func TestValidateConfig_MaxRetries(t *testing.T) {
	base := &config.Config{
		Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
		ReadyTimeout: 1e9,
	}
	type testCase struct {
		name string
		cfg  *config.Config
		want bool // true = expect error
	}
	tests := []testCase{
		{"-1", copyCfg(base, -1), true},
		{"0 ok", copyCfg(base, 0), false},
		{"5 ok", copyCfg(base, 5), false},
		{"6 error", copyCfg(base, 6), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if tt.want && err == nil {
				t.Fatal("expected error")
			}
			if !tt.want && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func copyCfg(base *config.Config, maxRetries int) *config.Config {
	c := *base
	c.MaxRetries = maxRetries
	return &c
}
