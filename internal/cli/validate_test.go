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
