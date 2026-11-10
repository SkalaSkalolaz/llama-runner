package cli

import (
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestBuildModelMenu_Dynamic_LocalOnly(t *testing.T) {
	cfg := &config.Config{
		Alias: "glm-4.7-flash",
		Profiles: map[string]*config.Config{
			"glm-4.7-flash":   {Alias: "glm-4.7-flash"},
			"qwen3.5-9B-fast": {Alias: "qwen3.5-9B-fast"},
			"qwen3.8-distil":  {Alias: "qwen3.8-distil"},
			"qwen3.8-flash":   {Alias: "qwen3.8-flash"},
		},
	}
	slots := BuildModelMenu(cfg)
	if len(slots) != 5 {
		t.Fatalf("len = %d, want 5", len(slots))
	}

	// Baseline for 1.9.0: dynamic menu uses switch-to: in standard slots
	// (Opus=current, Sonnet/Fable=other profiles, Haiku=bare primary).
	// Cleaner bare-primary layout is planned for 1.9.1.
	if got, want := slots[0].ModelValue, config.SwitchPrefix+"glm-4.7-flash"; got != want {
		t.Errorf("slots[0] (Opus) = %q, want %q", got, want)
	}
	if got, want := slots[1].ModelValue, config.SwitchPrefix+"qwen3.5-9B-fast"; got != want {
		t.Errorf("slots[1] (Sonnet) = %q, want %q", got, want)
	}
	if got, want := slots[2].ModelValue, "glm-4.7-flash"; got != want {
		t.Errorf("slots[2] (Haiku) = %q, want bare primary %q", got, want)
	}
	if got, want := slots[3].ModelValue, config.SwitchPrefix+"qwen3.8-distil"; got != want {
		t.Errorf("slots[3] (Fable) = %q, want %q", got, want)
	}

	// Custom 1 = switch-to:<third other profile>.
	if got, want := slots[4].ModelValue, config.SwitchPrefix+"qwen3.8-flash"; got != want {
		t.Errorf("slots[4] (Custom1) = %q, want %q", got, want)
	}
	if got, want := slots[4].Name, "qwen3.8-flash"; got != want {
		t.Errorf("slots[4].Name = %q, want %q", got, want)
	}
}

func TestBuildModelMenu_Dynamic_Hybrid(t *testing.T) {
	cfg := &config.Config{
		Alias:         "glm-4.7-flash",
		OpenAIBaseURL: "https://api.example.com/v1",
		OpenAIModel:   "gpt-4",
		OpenAIKey:     "k",
		Profiles: map[string]*config.Config{
			"glm-4.7-flash": {Alias: "glm-4.7-flash"},
			"qwen3.8-flash": {Alias: "qwen3.8-flash"},
		},
	}
	slots := BuildModelMenu(cfg)
	// Only one "other" profile → no Custom slot, Fable is empty.
	// Baseline for 1.9.0 (dynamic layout with switch-to: in standard slots);
	// cleaner bare-primary layout is planned for 1.9.1.
	if len(slots) != 4 {
		t.Fatalf("len = %d, want 4", len(slots))
	}

	if got, want := slots[0].ModelValue, config.SwitchPrefix+"glm-4.7-flash"; got != want {
		t.Errorf("slots[0] (Opus) = %q, want %q", got, want)
	}
	if got, want := slots[1].ModelValue, config.SwitchPrefix+"qwen3.8-flash"; got != want {
		t.Errorf("slots[1] (Sonnet) = %q, want %q", got, want)
	}
	if got, want := slots[2].ModelValue, "gpt-4"; got != want {
		t.Errorf("slots[2] (Haiku) = %q, want bare primary %q", got, want)
	}
	if slots[3].ModelValue != "" || slots[3].Filled {
		t.Errorf("slots[3] (Fable) = %q Filled=%v, want empty and unfilled",
			slots[3].ModelValue, slots[3].Filled)
	}
}

func TestBuildModelMenu_NonDynamic_Unchanged(t *testing.T) {
	// No Profiles → static path; matches old test expectations.
	cfg := &config.Config{Alias: "x"}
	slots := BuildModelMenu(cfg)
	if len(slots) != 5 || slots[4].Name != "local model" {
		t.Errorf("static menu changed: %+v", slots)
	}
}
