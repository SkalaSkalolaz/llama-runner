package cli

import (
	"sort"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// This module owns one responsibility: deciding WHAT the Claude Code /model
// menu contains — which model fills which slot. There is exactly one local
// model (the one llama-server loaded) and, optionally, one external
// OpenAI-compatible model, so the menu has no other choices to describe.
// It deliberately does not know how a slot is serialised: BuildClaudeEnv
// turns a []ModelSlot into environment variables (see modelmenuEnv naming in
// build.go), which is the only place that knows Claude Code reads its menu
// from the environment.

// SlotKind names a position in the /model menu.
type SlotKind int

// localModelRoute is the model ID that the proxy always routes to the local
// llama-server without a swap. It mirrors proxy.localModelName; the duplicate
// exists because cli cannot import proxy (cycle: proxy → cli → proxy).
// Currently unused: standard /model slots with no real profile to fill are
// left empty (Claude Code shows its builtin entry there) instead of being
// backfilled. Kept for a possible future re-introduction of backfill.
const localModelRoute = "llama-server"

const (
	// SlotOpus is the Opus position.
	SlotOpus SlotKind = iota
	// SlotSonnet is the Sonnet position.
	SlotSonnet
	// SlotHaiku is the Haiku position.
	SlotHaiku
	// SlotFable is the Fable position — the built-in top-tier entry Claude
	// Code shows between Opus and Sonnet.
	SlotFable
	// SlotCustom is the extra position Claude Code shows below the standard
	// ones. It carries the local model alias — the entry that routes the
	// request to llama-server in hybrid mode (the proxy recognises the alias,
	// see handlers.go).
	SlotCustom
)

// ModelSlot is one entry of the /model menu.
type ModelSlot struct {
	// Kind is the menu position this slot occupies.
	Kind SlotKind
	// CustomIndex numbers the SlotCustom entries, 1-based. Unused for the
	// standard positions.
	CustomIndex int
	// ModelValue is the model identifier Claude Code will put into its
	// requests for this slot.
	ModelValue string
	// Name is the human-readable label shown instead of ModelValue. Only
	// SlotCustom supports one; it is empty for the standard positions, which
	// display the bare identifier.
	Name string
	// Description is the explanatory line shown under Name (SlotCustom only).
	Description string
	// Filled reports whether a real model stands behind the slot.
	Filled bool
}

// BuildModelMenu computes the /model menu for Claude Code. There is exactly
// one local model (the one llama-server loaded) and, optionally, one external
// OpenAI-compatible model. All five standard positions (Opus/Sonnet/Haiku/
// Fable) plus the Custom entry are filled:

//   - local mode (no external provider): every position carries the local
//     model alias — the model llama-server actually serves;
//   - hybrid mode (external provider configured): the four standard positions
//     carry the external model, the Custom entry carries the local alias, so
//     selecting it routes the request to llama-server (the proxy recognises
//     the alias — see handlers.go).
//
// The first return value may be nil when cfg is nil or the local alias is
// empty (nothing to publish).
func BuildModelMenu(cfg *config.Config) []ModelSlot {
	if cfg == nil {
		return nil
	}
	if len(cfg.Profiles) > 0 {
		return buildDynamicMenu(cfg)
	}
	return buildStaticMenu(cfg)
}

// buildStaticMenu is the pre-profiles behaviour, preserved verbatim so
// configurations without a "profiles" section see byte-identical output.
func buildStaticMenu(cfg *config.Config) []ModelSlot {
	alias := cfg.Alias
	if alias == "" {
		return nil
	}
	custom := ModelSlot{
		Kind: SlotCustom, CustomIndex: 1,
		ModelValue: alias, Name: "local model", Filled: true,
	}
	if cfg.OpenAIProviderConfigured() {
		custom.Description = "Local llama-server model: " + alias
		ext := cfg.OpenAIModel
		return []ModelSlot{
			{Kind: SlotOpus, ModelValue: ext, Filled: true},
			{Kind: SlotSonnet, ModelValue: ext, Filled: true},
			{Kind: SlotHaiku, ModelValue: ext, Filled: true},
			{Kind: SlotFable, ModelValue: ext, Filled: true},
			custom,
		}
	}
	custom.Description = "Model llama-server loaded: " + alias
	return []ModelSlot{
		{Kind: SlotOpus, ModelValue: alias, Filled: true},
		{Kind: SlotSonnet, ModelValue: alias, Filled: true},
		{Kind: SlotHaiku, ModelValue: alias, Filled: true},
		{Kind: SlotFable, ModelValue: alias, Filled: true},
		custom,
	}
}

// buildDynamicMenu lays out the /model menu when a profile catalog is
// present:
//
//   - Standard slots (Opus/Sonnet/Haiku/Fable) always carry the BARE
//     primary model (PrimaryModel): Claude Code sends these values for
//     background tasks (session titles, compaction, subagents) and the
//     proxy routes such requests to whichever server is currently running
//     — never restarting llama-server.
//
//   - Custom 1 = switch-to:<startup local alias>, the profile the launcher
//     started with (root, -p <name>, or -model <path>).
//
//   - Custom 2+ = switch-to:<other profile>, one per declared profile.
//
// Selecting a custom slot is the ONLY way to trigger a llama-server
// restart; the prefix prevents background traffic from being mistaken for
// an explicit choice.
func buildDynamicMenu(cfg *config.Config) []ModelSlot {
	current := cfg.Alias
	if current == "" {
		return nil
	}
	primary := PrimaryModel(cfg)

	var others []string
	for a := range cfg.Profiles {
		if !strings.EqualFold(a, current) {
			others = append(others, a)
		}
	}
	sort.Strings(others) // mini, coder, agent → sorted by alias

	// Slots (fixed order in /model: Opus, Sonnet, Haiku, Fable):
	//   Opus    = switch-to:<root>     → Default resolves here; "recommended" = primary
	//   Sonnet  = switch-to:<other[0]> → first profile
	//   Haiku   = bare primary         → --model pins here; background never swaps
	//   Fable   = switch-to:<other[1]> → second profile
	// Custom 1  = switch-to:<other[2]> → third profile
	op := config.SwitchPrefix + current
	so, fa, cu := "", "", ""
	if len(others) > 0 {
		so = config.SwitchPrefix + others[0]
	}
	if len(others) > 1 {
		fa = config.SwitchPrefix + others[1]
	}
	if len(others) > 2 {
		cu = config.SwitchPrefix + others[2]
	}

	slots := []ModelSlot{
		{Kind: SlotOpus, ModelValue: op, Filled: true}, // Default → primary
		{Kind: SlotSonnet, ModelValue: so, Filled: so != ""},
		{Kind: SlotHaiku, ModelValue: primary, Filled: true}, // bare (safety)
		{Kind: SlotFable, ModelValue: fa, Filled: fa != ""},
	}
	if cu != "" {
		slots = append(slots, ModelSlot{
			Kind: SlotCustom, CustomIndex: 1,
			ModelValue:  cu,
			Name:        strings.TrimPrefix(cu, config.SwitchPrefix),
			Description: "Profile: " + strings.TrimPrefix(cu, config.SwitchPrefix),
			Filled:      true,
		})
	}
	return slots
}
