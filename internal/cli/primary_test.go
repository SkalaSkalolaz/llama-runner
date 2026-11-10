package cli

import (
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestPrimaryModel_Local(t *testing.T) {
	cfg := &config.Config{Alias: "qwen3.5-9B-fast"}
	if got := PrimaryModel(cfg); got != "qwen3.5-9B-fast" {
		t.Errorf("got %q", got)
	}
}

func TestPrimaryModel_Hybrid(t *testing.T) {
	cfg := &config.Config{
		Alias:         "local",
		OpenAIBaseURL: "https://x/v1",
		OpenAIModel:   "gpt-4",
		OpenAIKey:     "k",
	}
	if got := PrimaryModel(cfg); got != "gpt-4" {
		t.Errorf("got %q, want gpt-4", got)
	}
}

func TestPrimaryModel_HybridKeyless(t *testing.T) {
	cfg := &config.Config{
		Alias:         "local",
		OpenAIBaseURL: "https://x/v1",
		OpenAIModel:   "Qwen3.6-27B",
		OpenAIKeyless: true,
	}
	if got := PrimaryModel(cfg); got != "Qwen3.6-27B" {
		t.Errorf("got %q", got)
	}
}

func TestPrimaryModel_Nil(t *testing.T) {
	if got := PrimaryModel(nil); got != "" {
		t.Errorf("got %q", got)
	}
}
