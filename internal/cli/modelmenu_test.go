package cli

import (
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestBuildModelMenu_LocalFillsAllFiveSlots(t *testing.T) {
	cfg := &config.Config{Alias: "qwen3-coder"}
	slots := BuildModelMenu(cfg)
	if len(slots) != 5 {
		t.Fatalf("slots = %d, want 5", len(slots))
	}
	want := []SlotKind{SlotOpus, SlotSonnet, SlotHaiku, SlotFable, SlotCustom}
	for i, k := range want {
		if slots[i].Kind != k {
			t.Errorf("slots[%d].Kind = %v, want %v", i, slots[i].Kind, k)
		}
		if slots[i].ModelValue != "qwen3-coder" {
			t.Errorf("slots[%d].ModelValue = %q, want qwen3-coder", i, slots[i].ModelValue)
		}
		if !slots[i].Filled {
			t.Errorf("slots[%d] not Filled", i)
		}
	}
	if slots[4].Name != "local model" {
		t.Errorf("custom.Name = %q", slots[4].Name)
	}
	if !strings.Contains(slots[4].Description, "qwen3-coder") {
		t.Errorf("custom.Description = %q", slots[4].Description)
	}
}

func TestBuildModelMenu_HybridExternalFirstCustomLocal(t *testing.T) {
	cfg := &config.Config{
		Alias:         "qwen3-coder",
		OpenAIBaseURL: "https://api.example/v1",
		OpenAIModel:   "gpt-4",
		OpenAIKey:     "k",
	}
	slots := BuildModelMenu(cfg)
	if len(slots) != 5 {
		t.Fatalf("slots = %d, want 5", len(slots))
	}
	for i := 0; i < 4; i++ {
		if slots[i].ModelValue != "gpt-4" {
			t.Errorf("slots[%d].ModelValue = %q, want gpt-4", i, slots[i].ModelValue)
		}
	}
	if slots[4].ModelValue != "qwen3-coder" {
		t.Errorf("custom.ModelValue = %q, want qwen3-coder", slots[4].ModelValue)
	}
	if !strings.Contains(slots[4].Description, "qwen3-coder") {
		t.Errorf("custom.Description = %q", slots[4].Description)
	}
}

func TestBuildModelMenu_NilConfig(t *testing.T) {
	if got := BuildModelMenu(nil); got != nil {
		t.Errorf("BuildModelMenu(nil) = %+v", got)
	}
}

func TestBuildModelMenu_EmptyAlias(t *testing.T) {
	if got := BuildModelMenu(&config.Config{}); got != nil {
		t.Errorf("BuildModelMenu(empty) = %+v, want nil", got)
	}
}
