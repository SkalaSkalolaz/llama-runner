package cli

import (
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
	// Profile is the config-file profile behind this slot, "" when the slot
	// comes from somewhere else.
	Profile string
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
	alias := cfg.Alias
	if alias == "" {
		return nil
	}

	custom := ModelSlot{
		Kind:        SlotCustom,
		CustomIndex: 1,
		ModelValue:  alias,
		Name:        "local model",
		Filled:      true,
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
