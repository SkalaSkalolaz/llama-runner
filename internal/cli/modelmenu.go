package cli

import (
	"fmt"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// This module owns one responsibility: deciding WHAT the Claude Code /model
// menu contains — which model fills which slot, and what a slot says when
// nothing is behind it. It deliberately does not know how a slot is
// serialised: BuildClaudeEnv turns a []ModelSlot into environment variables
// (see modelmenuEnv naming in build.go), which is the only place that knows
// Claude Code reads its menu from the environment. Keeping the two apart is
// what lets a later change to model switching reuse this list as-is: at that
// point the menu stops being a description of one launch and becomes state
// the user moves through, and only the renderer has to follow.

// SlotKind names a position in the /model menu.
type SlotKind int

const (
	// SlotOpus is the Opus position. It is reserved for models reached
	// through the OpenAI-compatible API — the openaiSlotPlaceholder in local
	// mode, the configured external model itself in hybrid mode — and is
	// never filled from a local profile.
	SlotOpus SlotKind = iota
	// SlotSonnet is the Sonnet position, filled from the first profile.
	SlotSonnet
	// SlotHaiku is the Haiku position, filled from the second profile.
	SlotHaiku
	// SlotCustom is an extra position Claude Code shows below the standard
	// ones. The first carries the model that is actually loaded (CustomIndex
	// 1); further ones carry the remaining profiles.
	SlotCustom
)

// ModelSlot is one entry of the /model menu.
type ModelSlot struct {
	// Kind is the menu position this slot occupies.
	Kind SlotKind
	// CustomIndex numbers the SlotCustom entries, 1-based: 1 is the model
	// llama-server really serves, 2..N are the extra profiles. Unused for the
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
	// Filled reports whether a real model stands behind the slot, as opposed
	// to a placeholder telling the user the position is not usable.
	Filled bool
	// Profile is the config-file profile behind this slot, "" when the slot
	// comes from somewhere else (the root section, or nothing at all). It is
	// what a future model switch has to re-select.
	Profile string
}

// Placeholder texts for menu positions with no profile behind them. They are
// English on purpose: they are rendered by Claude Code, whose interface this
// launcher does not localise (unlike its own messages, which go through i18n),
// and the user asked for an English marker in the empty places.
const (
	// unusedStandardSlotValue fills a Sonnet/Haiku position that has no
	// profile. Those positions show the bare identifier — there is no
	// name/description variable for them — so the identifier itself has to
	// carry the message.
	unusedStandardSlotValue = "unused (no profile)"
	// unusedCustomSlotValue formats the identifier of an unused extra slot;
	// the number keeps two such slots distinct, and Claude Code sees a
	// readable label in UnusedCustomSlotNameFormat instead of this token.
	unusedCustomSlotValue = "unused-slot-%d"
	// unusedCustomSlotName formats the label of an unused extra slot.
	unusedCustomSlotName = "unused - slot %d (no profile)"
	// unusedCustomSlotDescription explains how to make the slot usable.
	unusedCustomSlotDescription = "No model behind this slot: add a profile to ~/.config/llama-runner/config.json"

	// defaultSlotName labels the first extra slot, which is the model the
	// server actually loaded — picking it needs no restart when nothing has
	// switched since, and switches back to the as-launched model otherwise.
	defaultSlotName = "local default (running)"
	// defaultSlotDescription names the loaded model behind that slot. The
	// profile list can name models the server did not load, so this is the
	// entry the user checks to see what is really answering.
	defaultSlotDescription = "Model llama-server loaded for this session: %s"
	// profileSlotName labels an extra slot filled from a config profile, and
	// profileSlotDescription tells what picking it does: the proxy restarts
	// the local llama-server with this profile before the next message goes
	// through, so the first reply after the pick is slow (see ModelSwitch).
	profileSlotName         = "profile %s - %s"
	profileSlotDescription  = "Config profile %q (model %s). Switches the local model on the next message (takes up to the ready timeout)."
	profileSlotModelUnknown = "unknown"
)

// The hybrid extra entry that keeps the local server reachable while the
// external gateway is the default target. The identifier is the token the
// proxy recognises and routes to llama-server; it must stay in sync with
// proxy.localModelName.
const (
	localRouteSlotValue       = "llama-server"
	localRouteSlotName        = "llama-server (local)"
	localRouteSlotDescription = "Route to local llama-server"
)

// BuildModelMenu computes the /model menu: which model fills which position
// and what an empty position says. It is mode-aware — local and hybrid share
// one layout so the positions never shift between launch modes — and the
// mode decides only what stands behind a position.
//
// Layout, top to bottom:
//
//	Opus     reserved for OpenAI-compatible models: the openaiSlotPlaceholder
//	         in local mode (unchanged from before profiles existed), the
//	         configured external model in hybrid mode — the entry through
//	         which the gateway itself is picked.
//	Sonnet   alias of the first profile in the config file.
//	Haiku    alias of the second profile.
//	custom 1 local: alias of the model that is actually loaded (cfg.Alias,
//	         already resolved by the caller — see ResolveAlias), labelled as
//	         the running one. Hybrid: the proxy's local-route token
//	         ("llama-server"), the entry that reaches llama-server while the
//	         gateway is the default target.
//	custom 2..N aliases of the remaining profiles, in file order.
//
// Two deviations from that shape, both deliberate:
//
//   - No profiles at all keeps the historical menu of each mode: in local
//     mode Sonnet and Haiku show the effective alias and there is no extra
//     slot; in hybrid mode all three standard slots carry the external
//     model. A single-model setup is the common case and has nothing to
//     choose between, so leaving it exactly as it was is friendlier than
//     "unused" markers.
//   - Fewer than two profiles leaves the remaining standard slot showing the
//     unused marker, so the user sees the position exists and what would
//     fill it.
//
// Picking a profile entry switches the local llama-server to that profile on
// the next message (see ModelSwitch): in both modes the proxy recognises the
// profile display names and routes them to the local switcher. In hybrid
// mode the gateway keeps working too: selecting the Opus slot sends the
// external model's name, which the proxy forwards upstream, and the
// "llama-server" token reaches whatever local model is loaded right now.
func BuildModelMenu(cfg *config.Config) []ModelSlot {
	if cfg == nil {
		return nil
	}
	if cfg.OpenAIProviderConfigured() {
		return hybridModelMenu(cfg)
	}
	// The alias is resolved once, early in run(), so here it is simply the
	// name of whatever the server loaded.
	loaded := cfg.Alias

	if len(cfg.Profiles) == 0 {
		// Historical single-model menu. Without a name to show there is
		// nothing to publish: the empty alias case is left untouched, exactly
		// as before, so Claude Code keeps its own default label.
		slots := []ModelSlot{{Kind: SlotOpus, ModelValue: openaiSlotPlaceholder}}
		if loaded != "" {
			slots = append(slots,
				ModelSlot{Kind: SlotSonnet, ModelValue: loaded, Filled: true},
				ModelSlot{Kind: SlotHaiku, ModelValue: loaded, Filled: true},
			)
		}
		return slots
	}

	slots := []ModelSlot{{Kind: SlotOpus, ModelValue: openaiSlotPlaceholder}}
	slots = append(slots, profileSlot(SlotSonnet, cfg.Profiles, 0))
	slots = append(slots, profileSlot(SlotHaiku, cfg.Profiles, 1))
	slots = append(slots, loadedModelSlot(loaded))
	// Extra profiles continue the custom numbering where loadedModelSlot left
	// it, so the published variables form a gapless sequence.
	for i, index := 2, 2; i < len(cfg.Profiles); i++ {
		slots = append(slots, extraProfileSlot(index, cfg.Profiles[i]))
		index++
	}
	return slots
}

// hybridModelMenu lays out the menu for hybrid mode (-openai with a key or a
// keyless provider): the same positions as local mode, with the external
// model in the Opus slot it is reserved for and the proxy's local-route
// token in the first extra slot, so both the gateway and the local server
// stay selectable. Profile entries follow the file order exactly as in local
// mode; see BuildModelMenu for what choosing one currently does.
func hybridModelMenu(cfg *config.Config) []ModelSlot {
	slots := []ModelSlot{{Kind: SlotOpus, ModelValue: cfg.OpenAIModel, Filled: true}}
	if len(cfg.Profiles) == 0 {
		// Historical hybrid menu: with nothing to lay out, every standard
		// slot keeps pointing at the gateway, exactly as before profiles.
		slots = append(slots,
			ModelSlot{Kind: SlotSonnet, ModelValue: cfg.OpenAIModel, Filled: true},
			ModelSlot{Kind: SlotHaiku, ModelValue: cfg.OpenAIModel, Filled: true},
		)
	} else {
		slots = append(slots, profileSlot(SlotSonnet, cfg.Profiles, 0))
		slots = append(slots, profileSlot(SlotHaiku, cfg.Profiles, 1))
	}
	slots = append(slots, localRouteSlot())
	for i, index := 2, 2; i < len(cfg.Profiles); i++ {
		slots = append(slots, extraProfileSlot(index, cfg.Profiles[i]))
		index++
	}
	return slots
}

// localRouteSlot is the first extra entry in hybrid mode: it carries the
// token the proxy routes to the local llama-server (see proxy.localModelName
// — the two literals must stay in sync).
func localRouteSlot() ModelSlot {
	return ModelSlot{
		Kind:        SlotCustom,
		CustomIndex: 1,
		ModelValue:  localRouteSlotValue,
		Name:        localRouteSlotName,
		Description: localRouteSlotDescription,
		Filled:      true,
	}
}

// profileSlot fills one of the standard positions from the profile at index i
// of the file-ordered list, or marks the position unused when the list is too
// short.
func profileSlot(kind SlotKind, profiles []config.Profile, i int) ModelSlot {
	if i >= len(profiles) {
		return ModelSlot{Kind: kind, ModelValue: unusedStandardSlotValue}
	}
	p := profiles[i]
	return ModelSlot{
		Kind:       kind,
		ModelValue: ProfileDisplayName(p),
		Filled:     true,
		Profile:    p.Name,
	}
}

// loadedModelSlot builds the first extra slot: the model the server actually
// holds. Its identifier is cfg.Alias, which the switcher recognises as the
// as-launched model (switch back to it reloads the original GGUF). An
// unresolved alias leaves the slot unused rather than publishing an empty
// model name.
func loadedModelSlot(alias string) ModelSlot {
	if alias == "" {
		return unusedCustomSlot(1)
	}
	return ModelSlot{
		Kind:        SlotCustom,
		CustomIndex: 1,
		ModelValue:  alias,
		Name:        defaultSlotName,
		Description: fmt.Sprintf(defaultSlotDescription, alias),
		Filled:      true,
	}
}

// unusedCustomSlot builds placeholder slot number index.
func unusedCustomSlot(index int) ModelSlot {
	return ModelSlot{
		Kind:        SlotCustom,
		CustomIndex: index,
		ModelValue:  fmt.Sprintf(unusedCustomSlotValue, index),
		Name:        fmt.Sprintf(unusedCustomSlotName, index),
		Description: unusedCustomSlotDescription,
	}
}

// extraProfileSlot builds custom position number index (1-based, continuing
// after the slot that carries the loaded model) for one profile.
func extraProfileSlot(index int, p config.Profile) ModelSlot {
	alias := ProfileDisplayName(p)
	return ModelSlot{
		Kind:        SlotCustom,
		CustomIndex: index,
		ModelValue:  alias,
		Name:        fmt.Sprintf(profileSlotName, p.Name, alias),
		Description: fmt.Sprintf(profileSlotDescription, p.Name, profileModelLabel(p)),
		Filled:      true,
		Profile:     p.Name,
	}
}

// profileModelLabel describes which model file a profile points at, falling
// back to a neutral word when it names none and therefore inherits the root
// model (whose name the menu does not know at this layer).
func profileModelLabel(p config.Profile) string {
	if p.Model != "" {
		return p.Model
	}
	return profileSlotModelUnknown
}
