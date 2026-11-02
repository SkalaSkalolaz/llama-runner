package cli

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// profiles builds a file-ordered profile list the way OrderedProfiles would.
func profiles(items ...config.Profile) []config.Profile { return items }

func menuShape(slots []ModelSlot) string {
	parts := make([]string, 0, len(slots))
	for _, s := range slots {
		label := s.ModelValue
		switch s.Kind {
		case SlotOpus:
			label = "opus:" + s.ModelValue
		case SlotSonnet:
			label = "sonnet:" + s.ModelValue
		case SlotHaiku:
			label = "haiku:" + s.ModelValue
		case SlotCustom:
			label = "custom" + strconv.Itoa(s.CustomIndex) + ":" + s.ModelValue
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "|")
}

func TestBuildModelMenu_NoProfilesKeepsHistoricalShape(t *testing.T) {
	slots := BuildModelMenu(&config.Config{Alias: "qwen3.8-distil"})
	want := "opus:openai|sonnet:qwen3.8-distil|haiku:qwen3.8-distil"
	if got := menuShape(slots); got != want {
		t.Errorf("menu = %s, want %s", got, want)
	}
	for _, s := range slots {
		if s.Kind == SlotCustom {
			t.Errorf("unexpected custom slot %+v with no profiles", s)
		}
	}
}

func TestBuildModelMenu_NoProfilesNoAliasPublishesNothingLocal(t *testing.T) {
	// No alias means nothing to name the local model by; only the reserved
	// Opus placeholder is published (as before profiles existed).
	slots := BuildModelMenu(&config.Config{})
	want := "opus:openai"
	if got := menuShape(slots); got != want {
		t.Errorf("menu = %s, want %s", got, want)
	}
}

func TestBuildModelMenu_TwoProfiles(t *testing.T) {
	cfg := &config.Config{
		Alias: "qwen3.8-distil",
		Profiles: profiles(
			config.Profile{Name: "coder", Model: "~/G/coder.gguf", Alias: "qwen3-coder", AliasSet: true},
			config.Profile{Name: "big", Model: "~/G/big.gguf", Alias: "qwen3.8-flash", AliasSet: true},
		),
	}
	want := "opus:openai|sonnet:qwen3-coder|haiku:qwen3.8-flash|custom1:qwen3.8-distil"
	if got := menuShape(BuildModelMenu(cfg)); got != want {
		t.Errorf("menu = %s, want %s", got, want)
	}

	slots := BuildModelMenu(cfg)
	if slots[1].Profile != "coder" || slots[2].Profile != "big" {
		t.Errorf("profile back-references = %q/%q, want coder/big", slots[1].Profile, slots[2].Profile)
	}
	if slots[0].Profile != "" || slots[3].Profile != "" {
		t.Errorf("opus/loaded slots should not reference a profile, got %q/%q",
			slots[0].Profile, slots[3].Profile)
	}
	if !slots[3].Filled || slots[3].Name != defaultSlotName {
		t.Errorf("loaded slot = %+v, want filled and labelled %q", slots[3], defaultSlotName)
	}
	if !strings.Contains(slots[3].Description, "qwen3.8-distil") {
		t.Errorf("loaded slot description does not name the loaded model: %q", slots[3].Description)
	}
}

func TestBuildModelMenu_OneProfilesMarksHaikuUnused(t *testing.T) {
	cfg := &config.Config{
		Alias:    "root-alias",
		Profiles: profiles(config.Profile{Name: "coder", Alias: "qwen3-coder", AliasSet: true}),
	}
	slots := BuildModelMenu(cfg)
	want := "opus:openai|sonnet:qwen3-coder|haiku:" + unusedStandardSlotValue + "|custom1:root-alias"
	if got := menuShape(slots); got != want {
		t.Errorf("menu = %s, want %s", got, want)
	}
	for _, s := range slots {
		if s.Kind == SlotHaiku && s.Filled {
			t.Error("the unused Haiku slot must not report Filled")
		}
	}
}

func TestBuildModelMenu_MoreThanTwoProfilesAddCustomSlots(t *testing.T) {
	cfg := &config.Config{
		Alias: "root-alias",
		Profiles: profiles(
			config.Profile{Name: "one", Alias: "a1", AliasSet: true},
			config.Profile{Name: "two", Alias: "a2", AliasSet: true},
			config.Profile{Name: "three", Alias: "a3", AliasSet: true},
			config.Profile{Name: "four", Model: "~/G/four.gguf"},
		),
	}
	slots := BuildModelMenu(cfg)
	// The loaded model takes custom position 1 and the extra profiles follow
	// it in file order with no gaps in the numbering.
	want := "opus:openai|sonnet:a1|haiku:a2|custom1:root-alias|custom2:a3|custom3:four"
	if got := menuShape(slots); got != want {
		t.Errorf("menu = %s, want %s", got, want)
	}
	fourth := slots[5]
	if fourth.Name != "profile four - four" {
		t.Errorf("slot for an alias-less profile = %q, want its derived name", fourth.Name)
	}
	if !strings.Contains(fourth.Description, "Switches the local model") {
		t.Errorf("extra slot should say picking it switches the local model: %q", fourth.Description)
	}
}

func TestBuildModelMenu_ProfileNamesFallBack(t *testing.T) {
	tests := []struct {
		name    string
		profile config.Profile
		want    string
	}{
		{"alias wins", config.Profile{Name: "coder", Model: "~/G/x.gguf", Alias: "qwen3-coder", AliasSet: true}, "qwen3-coder"},
		{"derived from model", config.Profile{Name: "coder", Model: "~/G/Qwen3-Coder-Q4_K_S.gguf"}, "Qwen3-Coder-Q4_K_S"},
		{"profile key last", config.Profile{Name: "coder"}, "coder"},
		{"empty alias falls back", config.Profile{Name: "coder", Model: "~/G/m.gguf", Alias: "", AliasSet: true}, "m"},
		{"no extension in name", config.Profile{Name: "coder", Model: "~/G/plain-model"}, "plain-model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfileDisplayName(tt.profile); got != tt.want {
				t.Errorf("ProfileDisplayName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildModelMenu_NilConfig(t *testing.T) {
	if got := BuildModelMenu(nil); got != nil {
		t.Errorf("BuildModelMenu(nil) = %+v, want nil", got)
	}
}

func TestBuildModelMenu_ProfilesWithUnresolvedAliasMarkCustomUnused(t *testing.T) {
	// Profiles exist but the running model has no name to publish (run()
	// always resolves one; this is the defensive path): the loaded-model slot
	// shows the unused marker instead of an empty model name.
	cfg := &config.Config{
		Profiles: profiles(
			config.Profile{Name: "coder", Alias: "qwen3-coder", AliasSet: true},
			config.Profile{Name: "big", Alias: "qwen3.8-flash", AliasSet: true},
		),
	}
	slots := BuildModelMenu(cfg)
	want := "opus:openai|sonnet:qwen3-coder|haiku:qwen3.8-flash|custom1:" +
		fmt.Sprintf(unusedCustomSlotValue, 1)
	if got := menuShape(slots); got != want {
		t.Errorf("menu = %s, want %s", got, want)
	}
	if slots[3].Filled {
		t.Error("the unused custom slot must not report Filled")
	}
	if !strings.Contains(slots[3].Name, "unused") {
		t.Errorf("custom slot name = %q, want an unused marker", slots[3].Name)
	}
	if !strings.Contains(slots[3].Description, "config.json") {
		t.Errorf("custom slot should say how to fill it: %q", slots[3].Description)
	}
}

func TestBuildModelMenu_IsDeterministic(t *testing.T) {
	cfg := &config.Config{
		Alias: "root",
		Profiles: profiles(
			config.Profile{Name: "zeta", Alias: "z", AliasSet: true},
			config.Profile{Name: "alpha", Alias: "a", AliasSet: true},
			config.Profile{Name: "mid", Alias: "m", AliasSet: true},
		),
	}
	want := menuShape(BuildModelMenu(cfg))
	for i := 0; i < 20; i++ {
		if got := menuShape(BuildModelMenu(cfg)); got != want {
			t.Fatalf("iteration %d: menu = %s, want %s", i, got, want)
		}
	}
}
